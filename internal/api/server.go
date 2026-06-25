// Package api exposes Plato's HTTP REST API and serves the embedded web UI.
package api

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"

	"github.com/plato/plato/internal/store"
	"github.com/plato/plato/internal/wiki"
)

// Server wires the store, wiki service, and HTTP routes.
type Server struct {
	DB   *store.DB
	Svc  *wiki.Service
	Mux  *http.ServeMux
	webFS fs.FS // optional embedded SPA (may be nil)
}

// New builds a Server. webFS, if non-nil, is served as the SPA at "/".
func New(db *store.DB, svc *wiki.Service, webFS fs.FS) *Server {
	s := &Server{DB: db, Svc: svc, Mux: http.NewServeMux(), webFS: webFS}
	s.routes()
	return s
}

func (s *Server) routes() {
	// Health (no auth).
	s.Mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	// Tokens.
	s.Mux.HandleFunc("POST /api/v1/tokens", s.require(writeScope, s.handleCreateToken))
	s.Mux.HandleFunc("GET /api/v1/tokens", s.require(readScope, s.handleListTokens))
	s.Mux.HandleFunc("DELETE /api/v1/tokens/{id}", s.require(writeScope, s.handleRevokeToken))

	// Wikis.
	s.Mux.HandleFunc("GET /api/v1/wikis", s.require(readScope, s.handleListWikis))
	s.Mux.HandleFunc("POST /api/v1/wikis", s.require(writeScope, s.handleCreateWiki))
	s.Mux.HandleFunc("GET /api/v1/wikis/{wikiSlug}", s.require(readScope, s.handleGetWiki))

	// Pages.
	s.Mux.HandleFunc("GET /api/v1/wikis/{wikiSlug}/pages", s.require(readScope, s.handleListPages))
	s.Mux.HandleFunc("POST /api/v1/wikis/{wikiSlug}/pages", s.require(writeScope, s.handleCreatePage))
	s.Mux.HandleFunc("GET /api/v1/wikis/{wikiSlug}/pages/{pageSlug}", s.require(readScope, s.handleGetPage))
	s.Mux.HandleFunc("PUT /api/v1/wikis/{wikiSlug}/pages/{pageSlug}", s.require(writeScope, s.handleUpdatePage))
	s.Mux.HandleFunc("DELETE /api/v1/wikis/{wikiSlug}/pages/{pageSlug}", s.require(writeScope, s.handleDeletePage))

	// Links.
	s.Mux.HandleFunc("GET /api/v1/wikis/{wikiSlug}/pages/{pageSlug}/links", s.require(readScope, s.handlePageLinks))
	s.Mux.HandleFunc("POST /api/v1/wikis/{wikiSlug}/pages/{pageSlug}/links", s.require(writeScope, s.handleAddLink))
	s.Mux.HandleFunc("DELETE /api/v1/wikis/{wikiSlug}/pages/{pageSlug}/links", s.require(writeScope, s.handleRemoveLink))

	// Cross-link verification (project-wide integrity report).
	s.Mux.HandleFunc("GET /api/v1/wikis/{wikiSlug}/verify", s.require(readScope, s.handleVerify))

	// Link graph (nodes + edges) for external agent analysis.
	s.Mux.HandleFunc("GET /api/v1/wikis/{wikiSlug}/graph", s.require(readScope, s.handleGraph))

	// Sync.
	s.Mux.HandleFunc("POST /api/v1/wikis/{wikiSlug}/sync", s.require(writeScope, s.handleSync))

	// Git pull (git-backed projects only).
	s.Mux.HandleFunc("POST /api/v1/wikis/{wikiSlug}/git/pull", s.require(writeScope, s.handleGitPull))

	// SPA (catch-all). Registered last so /api and /healthz take precedence.
	if s.webFS != nil {
		s.Mux.Handle("/", s.spaHandler())
	}
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.Mux.ServeHTTP(w, r) }

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// lookupWiki resolves the {wikiSlug} path value to a wiki, writing 404 on miss.
func (s *Server) lookupWiki(w http.ResponseWriter, r *http.Request) *store.Wiki {
	slug := r.PathValue("wikiSlug")
	wk, err := s.DB.WikiBySlug(slug)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return nil
	}
	if wk == nil {
		writeErr(w, http.StatusNotFound, "wiki not found")
		return nil
	}
	return wk
}

// spaHandler serves the embedded SPA, falling back to index.html for client routes.
func (s *Server) spaHandler() http.Handler {
	fileServer := http.FileServer(http.FS(s.webFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(s.webFS, p); err != nil {
			// Unknown path -> serve index.html for SPA routing.
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			b, err := fs.ReadFile(s.webFS, "index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(b)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
