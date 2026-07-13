// Command cella is a self-hostable, single-binary tool for Cardano
// Constitutional Committee (CC) governance.
//
// This is the standalone, WordPress-free build: ingest on-chain governance
// actions from Koios into a local SQLite database and serve them for review.
// Build once, run anywhere — no database server, no runtime dependencies.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Awen-online/cella/internal/cardano"
	"github.com/Awen-online/cella/internal/config"
	"github.com/Awen-online/cella/internal/constitution"
	"github.com/Awen-online/cella/internal/koios"
	"github.com/Awen-online/cella/internal/llm"
	"github.com/Awen-online/cella/internal/server"
	"github.com/Awen-online/cella/internal/store"
)

const version = "0.0.1"

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
  CELLA_ROSTER     path to the delegate roster JSON (default: placeholder roster)
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

	// For each action, pull its on-chain votes and keep the CC ones, plus the
	// stake-weighted tally across every voter role. A committee weighing
	// constitutionality should know how the DReps and SPOs are voting — not to
	// follow them, but to know what it is agreeing or disagreeing with.
	votesTotal, summaries := 0, 0
	for _, a := range actions {
		votes, err := kc.ProposalVotes(ctx, a.ProposalID)
		if err != nil {
			log.Printf("  warn: votes for %s: %v", a.ProposalID, err)
			continue
		}
		nv, err := db.UpsertVotes(a.ProposalID, votes)
		if err != nil {
			return err
		}
		votesTotal += nv

		sum, ok, err := kc.ProposalVotingSummary(ctx, a.ProposalID)
		if err != nil {
			log.Printf("  warn: voting summary for %s: %v", a.ProposalID, err)
			continue
		}
		if ok {
			if err := db.SaveVotingSummary(a.ProposalID, sum); err != nil {
				return err
			}
			summaries++
		}
	}

	log.Printf("ingested %d governance actions (%d written), %d CC votes and %d voting summaries into %s",
		len(actions), na, votesTotal, summaries, cfg.DBPath)
	return nil
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

	body, err := server.LoadBody(cfg.RosterPath)
	if err != nil {
		return err
	}
	if cfg.RosterPath == "" {
		log.Printf("warning: CELLA_ROSTER is not set — using the placeholder roster; no wallet can sign in")
	} else {
		log.Printf("roster: %s (%d delegates)", body.Name, len(body.Members))
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
