package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/plato/plato/internal/gitsrc"
	"github.com/plato/plato/internal/store"
	"github.com/plato/plato/internal/wiki"
)

// wikiJSON renders a project with source metadata and graph stats for the API.
func wikiJSON(wk *store.Wiki, st *store.WikiStats) map[string]any {
	return map[string]any{
		"id":              wk.ID,
		"slug":            wk.Slug,
		"title":           wk.Title,
		"created_at":      wk.CreatedAt,
		"source_type":     wk.SourceType,
		"source_url":      wk.SourceURL.String,
		"source_branch":   wk.SourceBranch.String,
		"source_subdir":   wk.SourceSubdir.String,
		"last_indexed_at": wk.LastIndexedAt.String,
		"stats":           st,
	}
}

func (s *Server) handleListWikis(w http.ResponseWriter, r *http.Request) {
	wikis, err := s.DB.ListWikis()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := []map[string]any{}
	for i := range wikis {
		st, err := s.DB.Stats(wikis[i].ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, wikiJSON(&wikis[i], st))
	}
	writeJSON(w, http.StatusOK, map[string]any{"wikis": out})
}

type createWikiReq struct {
	Slug         string `json:"slug"`
	Title        string `json:"title"`
	SourceType   string `json:"source_type"`   // empty | local | git (default empty)
	SourceURL    string `json:"source_url"`    // local path or git url
	SourceBranch string `json:"source_branch"` // git only
	SourceSubdir string `json:"source_subdir"` // git/local subdir (git only here)
}

func (s *Server) handleCreateWiki(w http.ResponseWriter, r *http.Request) {
	var req createWikiReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(req.Slug) == "" {
		writeErr(w, http.StatusBadRequest, "slug required")
		return
	}
	st := req.SourceType
	if st == "" {
		st = store.SourceEmpty
	}

	var wk *store.Wiki
	var err error
	switch st {
	case store.SourceEmpty:
		wk, err = s.Svc.CreateEmptyProject(req.Slug, req.Title)
	case store.SourceLocal:
		if req.SourceURL == "" {
			writeErr(w, http.StatusBadRequest, "source_url (local folder path) required")
			return
		}
		wk, err = s.Svc.CreateLocalProject(req.Slug, req.Title, req.SourceURL)
	case store.SourceGit:
		if req.SourceURL == "" {
			writeErr(w, http.StatusBadRequest, "source_url (git url) required")
			return
		}
		wk, err = s.Svc.CreateGitProject(req.Slug, req.Title, req.SourceURL, req.SourceBranch, req.SourceSubdir)
	default:
		writeErr(w, http.StatusBadRequest, "invalid source_type")
		return
	}

	switch {
	case errors.Is(err, wiki.ErrExists):
		writeErr(w, http.StatusConflict, "project already exists")
		return
	case errors.Is(err, gitsrc.ErrInvalidURL):
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, wiki.ErrBadInput):
		writeErr(w, http.StatusBadRequest, "invalid source (folder not found?)")
		return
	case err != nil:
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.DB.Audit(actorName(r), "wiki.create", wk.ID, 0, `{"source_type":"`+st+`"}`)
	st2, _ := s.DB.Stats(wk.ID)
	writeJSON(w, http.StatusCreated, wikiJSON(wk, st2))
}

// handleGitPull pulls latest for a git-backed project and reindexes.
func (s *Server) handleGitPull(w http.ResponseWriter, r *http.Request) {
	wk := s.lookupWiki(w, r)
	if wk == nil {
		return
	}
	if wk.SourceType != store.SourceGit {
		writeErr(w, http.StatusBadRequest, "project is not git-backed")
		return
	}
	pages, links, err := s.Svc.GitPull(wk)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.DB.Audit(actorName(r), "project.git_pull", wk.ID, 0, "")
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "pages_changed": pages, "links_reindexed": links,
	})
}

func (s *Server) handleGetWiki(w http.ResponseWriter, r *http.Request) {
	wk := s.lookupWiki(w, r)
	if wk == nil {
		return
	}
	st, err := s.DB.Stats(wk.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, wikiJSON(wk, st))
}
