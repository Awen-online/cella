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

	actions, err := kc.GovernanceActions(ctx, *limit)
	if err != nil {
		return err
	}
	na, err := db.UpsertActions(actions)
	if err != nil {
		return err
	}

	// For each action, pull its on-chain votes and keep the CC ones.
	votesTotal := 0
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
	}

	log.Printf("ingested %d governance actions (%d written) and %d CC votes into %s", len(actions), na, votesTotal, cfg.DBPath)
	return nil
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

	if cfg.Secret == "" {
		log.Printf("warning: CELLA_SECRET is not set — using a random session key; sessions will not survive a restart")
	}
	if cfg.Demo {
		log.Printf("WARNING: CELLA_DEMO is set — anyone who can reach this instance may enter as any")
		log.Printf("WARNING: delegate, cast votes in their name, and author the committee's rationale.")
		log.Printf("WARNING: Do not expose this instance to a network.")
	}

	log.Printf("cella %s serving on http://localhost%s", version, *addr)
	return server.New(db, server.Options{Secret: cfg.Secret, Demo: cfg.Demo}).ListenAndServe(*addr)
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
