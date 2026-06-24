// Command plato-sync imports a directory of cross-linked Markdown files into a
// Plato wiki, operating directly on the SQLite index and wiki directory.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/plato/plato/internal/store"
	"github.com/plato/plato/internal/sync"
	"github.com/plato/plato/internal/wiki"
)

func main() {
	dir := flag.String("dir", "", "Markdown directory to import (required)")
	wikiSlug := flag.String("wiki", "", "target wiki slug (required)")
	dbPath := flag.String("db", "./plato.db", "SQLite DB path")
	wikiDir := flag.String("wiki-dir", "./data", "Plato wiki root")
	withDelete := flag.Bool("delete", false, "delete pages whose source files vanished")
	create := flag.Bool("create", false, "create the wiki if it does not exist")
	flag.Parse()

	if *dir == "" || *wikiSlug == "" {
		fmt.Fprintln(os.Stderr, "plato-sync: --dir and --wiki are required")
		flag.Usage()
		os.Exit(2)
	}

	db, err := store.Open(*dbPath)
	if err != nil {
		fatal("open db: %v", err)
	}
	defer db.Close()

	svc := wiki.New(db, *wikiDir)

	w, err := db.WikiBySlug(*wikiSlug)
	if err != nil {
		fatal("lookup wiki: %v", err)
	}
	if w == nil {
		if !*create {
			fatal("wiki %q does not exist (use --create)", *wikiSlug)
		}
		w, err = svc.CreateWiki(*wikiSlug, *wikiSlug)
		if err != nil {
			fatal("create wiki: %v", err)
		}
		_ = db.Audit("plato-sync", "wiki.create", w.ID, 0, "")
	}

	rep, err := sync.Run(svc, w, *dir, *withDelete, "plato-sync")
	if err != nil {
		fatal("sync: %v", err)
	}

	fmt.Printf("synced wiki %q from %s\n", *wikiSlug, *dir)
	fmt.Printf("  created:   %d\n", len(rep.Created))
	fmt.Printf("  updated:   %d\n", len(rep.Updated))
	fmt.Printf("  unchanged: %d\n", len(rep.Unchanged))
	fmt.Printf("  deleted:   %d\n", len(rep.Deleted))
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "plato-sync: "+format+"\n", args...)
	os.Exit(1)
}
