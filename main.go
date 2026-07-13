// Command cella is a self-hostable, single-binary tool for Cardano
// Constitutional Committee (CC) governance.
//
// This is the standalone, WordPress-free build: ingest on-chain governance
// actions from Koios into a local SQLite database and serve them for review.
// Build once, run anywhere — no database server, no runtime dependencies.
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/Awen-online/cella/internal/cardano"
	"github.com/Awen-online/cella/internal/config"
	"github.com/Awen-online/cella/internal/constitution"
	"github.com/Awen-online/cella/internal/koios"
	"github.com/Awen-online/cella/internal/llm"
	"github.com/Awen-online/cella/internal/server"
	"github.com/Awen-online/cella/internal/store"
)

const version = "0.0.1"

// The body Cella ships with, compiled in so a fresh clone runs with no
// configuration at all. It is the same body/curia.json an operator would edit —
// one source of truth, readable on disk and embedded in the binary, rather than
// a Go struct that drifts away from the JSON beside it.
//
//go:embed body/curia.json body/curia-logo.svg
var builtinBody embed.FS

// loadBody resolves who this instance belongs to: the file CELLA_BODY names, or
// the built-in Curia body when it names none.
func loadBody(path string) (server.Body, string, error) {
	if path != "" {
		b, err := server.LoadBody(path)
		return b, path, err
	}

	data, err := builtinBody.ReadFile("body/curia.json")
	if err != nil {
		return server.Body{}, "", err
	}
	b, err := server.ParseBody(data, "the built-in body")
	if err != nil {
		return server.Body{}, "", err
	}
	logo, err := builtinBody.ReadFile("body/curia-logo.svg")
	if err != nil {
		return server.Body{}, "", err
	}
	b.SetLogo(logo, "image/svg+xml")
	return b, "built-in", nil
}

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	cfg := config.Load()
	cmd, args := os.Args[1], os.Args[2:]

	var err error
	switch cmd {
	case "ingest":
		err = runIngest(cfg, args)
	case "review":
		err = runReview(cfg, args)
	case "serve":
		err = runServe(cfg, args)
	case "version", "-v", "--version":
		fmt.Printf("cella %s\n", version)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatalf("%s: %v", cmd, err)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `cella — self-hostable Cardano Constitutional Committee governance

usage:
  cella ingest    pull governance actions + CC votes from Koios into the database
  cella review    assess stored actions against the Constitution (bring your own LLM)
  cella serve     start the web UI
  cella version   print the version

configuration (environment):
  CELLA_DB         path to the SQLite database   (default ./cella.db)
  CELLA_ADDR       web server listen address     (default :8080)
  CELLA_SECRET     signs session cookies         (default: random key per start)
  CELLA_DEMO       enable roster sign-in — NO AUTH; never on a reachable instance
  CELLA_BODY       path to the body's JSON (who this instance belongs to; logo alongside)
  CELLA_HOT_NFT_ADDR  hot NFT script address — its datum sets the voting group + quorum
  KOIOS_URL        Koios API base URL            (default https://api.koios.rest/api/v1)
  KOIOS_TOKEN      optional Koios bearer token
  CELLA_LLM_URL    OpenAI-compatible endpoint    (e.g. http://localhost:11434/v1 for Ollama)
  CELLA_LLM_MODEL  model name                    (e.g. gpt-4o-mini, llama3.1)
  CELLA_LLM_KEY    optional API key (local models need none)
`)
}

func runIngest(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	limit := fs.Int("limit", 100, "max governance actions to fetch")
	if err := fs.Parse(args); err != nil {
		return err
	}

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()
	kc := koios.New(cfg.KoiosURL, cfg.KoiosToken)

	// The chain states an action's expiration as an epoch number. Capture the
	// network's genesis parameters so the web UI can turn that into a real
	// deadline without needing the network itself.
	if gp, err := kc.Genesis(ctx); err != nil {
		log.Printf("  warn: genesis parameters: %v (deadlines will show the raw epoch)", err)
	} else if err := db.SaveNetwork(gp); err != nil {
		return err
	}

	// Who sits on the Constitutional Committee, and what it takes for them to
	// ratify. Members resign and terms expire; a hardcoded roster is quietly
	// wrong the first time either happens.
	if ci, err := kc.Committee(ctx); err != nil {
		log.Printf("  warn: committee: %v", err)
	} else if err := db.SaveCommittee(ci); err != nil {
		return err
	} else {
		log.Printf("committee: %d seats (%d authorized), quorum %s — %d Yes needed to ratify",
			len(ci.Members), len(ci.Authorized()), ci.Quorum(), ci.YesNeeded())
	}

	// Who may sign the committee's vote, and therefore what quorum is. The chain
	// is the authority; a warning is enough if we cannot reach it, because
	// everything else in Cella still works without it.
	if cfg.HotNFTAddr != "" {
		if err := syncVotingGroup(ctx, kc, db, cfg.HotNFTAddr); err != nil {
			log.Printf("  warn: hot NFT voting group: %v", err)
		}
	}

	actions, err := kc.GovernanceActions(ctx, *limit)
	if err != nil {
		return err
	}
	na, err := db.UpsertActions(actions)
	if err != nil {
		return err
	}
	log.Printf("%d governance actions (%d written). Fetching votes and voting summaries for each…",
		len(actions), na)

	// For each action, pull its on-chain votes and keep the CC ones, plus the
	// stake-weighted tally across every voter role. A committee weighing
	// constitutionality should know how the DReps and SPOs are voting — not to
	// follow them, but to know what it is agreeing or disagreeing with.
	//
	// That is two Koios calls per action, and a hundred actions run serially is
	// several silent minutes — long enough that an operator reasonably concludes
	// it has hung. So: fetch concurrently, and say what is happening while it
	// happens. Writes stay on this goroutine, because SQLite would rather they
	// did.
	type result struct {
		id      string
		title   string
		votes   []koios.Vote
		summary koios.VotingSummary
		hasSum  bool
		err     error
	}

	const workers = 6 // enough to be quick, few enough to be a good Koios citizen
	jobs := make(chan koios.GovernanceAction)
	out := make(chan result)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for a := range jobs {
				r := result{id: a.ProposalID, title: a.Title()}
				if r.votes, r.err = kc.ProposalVotes(ctx, a.ProposalID); r.err == nil {
					r.summary, r.hasSum, r.err = kc.ProposalVotingSummary(ctx, a.ProposalID)
				}
				out <- r
			}
		}()
	}
	go func() {
		for _, a := range actions {
			jobs <- a
		}
		close(jobs)
		wg.Wait()
		close(out)
	}()

	votesTotal, summaries, done := 0, 0, 0
	for r := range out {
		done++
		if r.err != nil {
			log.Printf("  [%d/%d] %s — %v", done, len(actions), short(r.title, r.id), r.err)
			continue
		}
		nv, err := db.UpsertVotes(r.id, r.votes)
		if err != nil {
			return err
		}
		votesTotal += nv

		if r.hasSum {
			if err := db.SaveVotingSummary(r.id, r.summary); err != nil {
				return err
			}
			summaries++
		}
		log.Printf("  [%d/%d] %s — %d CC votes%s",
			done, len(actions), short(r.title, r.id), nv, map[bool]string{true: ", voting summary"}[r.hasSum])
	}

	log.Printf("done: %d governance actions (%d written), %d CC votes and %d voting summaries in %s",
		len(actions), na, votesTotal, summaries, cfg.DBPath)
	return nil
}

// short names an action for a progress line: its title, or its id when the
// anchor carried no title.
func short(title, id string) string {
	if title == "" {
		if len(id) > 24 {
			return id[:24] + "…"
		}
		return id
	}
	r := []rune(title)
	if len(r) > 48 {
		return string(r[:48]) + "…"
	}
	return title
}

// syncVotingGroup reads the hot NFT's inline datum and records who the chain
// will accept vote signatures from.
func syncVotingGroup(ctx context.Context, kc *koios.Client, db *store.DB, addr string) error {
	datum, err := kc.HotNFTDatum(ctx, addr)
	if err != nil {
		return err
	}
	group, err := cardano.DecodeHotDatum(datum)
	if err != nil {
		return err
	}
	if err := db.SaveVotingGroup(group); err != nil {
		return err
	}
	log.Printf("hot NFT voting group: %d delegates, quorum %d", len(group.Distinct()), group.Quorum())
	return nil
}

// browseURL turns a listen address into one you can actually click. A bare
// ":8080" listens on every interface, so localhost is the sensible thing to
// offer; an address that already names a host should be left alone rather than
// having "localhost" glued to the front of it.
func browseURL(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "http://localhost" + addr
	}
	return "http://" + addr
}

func runServe(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", cfg.Addr, "listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	body, source, err := loadBody(cfg.BodyPath)
	if err != nil {
		return err
	}
	log.Printf("body: %s — %d member(s), from %s", body.Name, len(body.Members), source)

	// A member with no registered wallet address cannot sign in. Say so at
	// startup rather than leaving them to discover it at the door.
	var unregistered int
	for _, m := range body.Members {
		if m.Address == "" {
			unregistered++
		}
	}
	if unregistered > 0 {
		log.Printf("warning: %d of %d member(s) have no wallet address in %s and cannot sign in",
			unregistered, len(body.Members), source)
	}

	if cfg.Secret == "" {
		log.Printf("warning: CELLA_SECRET is not set — using a random session key; sessions will not survive a restart")
	}
	if cfg.Demo {
		log.Printf("WARNING: CELLA_DEMO is set — anyone who can reach this instance may enter as any")
		log.Printf("WARNING: delegate, cast votes in their name, and author the committee's rationale.")
		log.Printf("WARNING: Do not expose this instance to a network.")
	}

	log.Printf("cella %s serving on %s", version, browseURL(*addr))
	return server.New(db, server.Options{
		Secret: cfg.Secret,
		Demo:   cfg.Demo,
		Body:   body,
	}).ListenAndServe(*addr)
}

func runReview(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("review", flag.ExitOnError)
	limit := fs.Int("limit", 20, "max actions to review")
	force := fs.Bool("force", false, "re-review actions that already have a review")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if cfg.LLMURL == "" || cfg.LLMModel == "" {
		return fmt.Errorf("no model configured: set CELLA_LLM_URL and CELLA_LLM_MODEL " +
			"(e.g. a local Ollama at http://localhost:11434/v1 with model llama3.1)")
	}

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	actions, err := db.Actions(*limit)
	if err != nil {
		return err
	}
	ids := make([]string, len(actions))
	for i, a := range actions {
		ids[i] = a.ProposalID
	}
	existing, err := db.ReviewsFor(ids)
	if err != nil {
		return err
	}

	constText, constVer, err := constitution.Text("")
	if err != nil {
		return fmt.Errorf("load constitution: %w", err)
	}
	log.Printf("grounding review against the Cardano Constitution %s", constVer.Label)

	prov := llm.NewOpenAICompatible(cfg.LLMURL, cfg.LLMModel, cfg.LLMKey, constText)
	ctx := context.Background()
	reviewed := 0
	for _, a := range actions {
		if _, done := existing[a.ProposalID]; done && !*force {
			continue
		}
		as, err := prov.Assess(ctx, llm.ActionInput{Type: a.Type, Title: a.Title, Abstract: a.Abstract})
		if err != nil {
			log.Printf("  warn: review %s: %v", a.ProposalID, err)
			continue
		}
		if err := db.UpsertReview(a.ProposalID, as.Verdict, as.Summary, as.Model); err != nil {
			return err
		}
		reviewed++
		log.Printf("  %s -> %s", a.ProposalID, as.Verdict)
	}
	log.Printf("reviewed %d actions with %s", reviewed, cfg.LLMModel)
	return nil
}
