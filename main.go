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
	"github.com/Awen-online/cella/internal/koios"
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
  cella ingest    pull governance actions from Koios into the local database
  cella serve     start the web UI
  cella version   print the version

configuration (environment, all optional):
  CELLA_DB      path to the SQLite database   (default ./cella.db)
  CELLA_ADDR    web server listen address     (default :8080)
  KOIOS_URL     Koios API base URL            (default https://api.koios.rest/api/v1)
  KOIOS_TOKEN   optional Koios bearer token
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

	actions, err := koios.New(cfg.KoiosURL, cfg.KoiosToken).GovernanceActions(context.Background(), *limit)
	if err != nil {
		return err
	}
	n, err := db.UpsertActions(actions)
	if err != nil {
		return err
	}
	log.Printf("ingested %d governance actions (%d new/updated) into %s", len(actions), n, cfg.DBPath)
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

	log.Printf("cella %s serving on http://localhost%s", version, *addr)
	return server.New(db).ListenAndServe(*addr)
}
