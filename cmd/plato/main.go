// Command plato runs the Plato wiki server: a REST API over a SQLite-indexed,
// filesystem-backed Markdown wiki, plus the embedded web UI.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/plato/plato/internal/api"
	"github.com/plato/plato/internal/auth"
	"github.com/plato/plato/internal/config"
	"github.com/plato/plato/internal/store"
	"github.com/plato/plato/internal/wiki"
)

func main() {
	def := config.Defaults()
	port := flag.Int("port", def.Port, "HTTP port")
	dbPath := flag.String("db", def.DBPath, "SQLite database path")
	wikiDir := flag.String("wiki-dir", def.WikiDir, "root directory for Markdown files")
	flag.Parse()

	if err := os.MkdirAll(*wikiDir, 0o755); err != nil {
		fatal("create wiki-dir: %v", err)
	}

	db, err := store.Open(*dbPath)
	if err != nil {
		fatal("open db: %v", err)
	}
	defer db.Close()

	if err := maybeBootstrap(db); err != nil {
		fatal("bootstrap: %v", err)
	}

	svc := wiki.New(db, *wikiDir)
	srv := api.New(db, svc, webFS())

	httpSrv := &http.Server{
		Addr:    ":" + strconv.Itoa(*port),
		Handler: srv,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		fmt.Printf("Plato listening on http://localhost:%d  (wiki-dir=%s db=%s)\n", *port, *wikiDir, *dbPath)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fatal("serve: %v", err)
		}
	}()

	<-ctx.Done()
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutCtx)
	fmt.Println("Plato stopped")
}

// maybeBootstrap mints a one-time full-access token if none exist.
func maybeBootstrap(db *store.DB) error {
	n, err := db.CountTokens()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	raw, hash := auth.Generate()
	if _, err := db.CreateToken("bootstrap", hash, auth.NormalizeScopes([]string{auth.ScopeWrite})); err != nil {
		return err
	}
	_ = db.Audit("system", "token.create", 0, 0, `{"name":"bootstrap"}`)
	fmt.Println("Plato bootstrap token:")
	fmt.Println(raw)
	fmt.Println("(store this now — it will not be shown again)")
	return nil
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "plato: "+format+"\n", args...)
	os.Exit(1)
}
