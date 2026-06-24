package api

import (
	"errors"
	"net/http"

	"github.com/plato/plato/internal/mdlink"
	"github.com/plato/plato/internal/store"
	"github.com/plato/plato/internal/wiki"
)

// isStrict reports whether the request asked for cross-link enforcement, via
// ?strict=true|1 or the X-Plato-Strict-Links header.
func isStrict(r *http.Request) bool {
	q := r.URL.Query().Get("strict")
	if q == "true" || q == "1" {
		return true
	}
	h := r.Header.Get("X-Plato-Strict-Links")
	return h == "true" || h == "1"
}

// writeBrokenLinks rejects a write that would introduce broken cross-references,
// returning 422 with the offending links so the caller (often an agent) can fix
// them before retrying.
func writeBrokenLinks(w http.ResponseWriter, broken []mdlink.Resolution) {
	type item struct {
		Raw    string `json:"raw"`
		Target string `json:"target"`
		Kind   string `json:"kind"`
		Status string `json:"status"`
	}
	out := make([]item, 0, len(broken))
	for _, b := range broken {
		out = append(out, item{Raw: b.Ref.Raw, Target: b.Ref.Target, Kind: b.Ref.Kind, Status: b.Status})
	}
	writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
		"error":  "broken_links",
		"message": "write rejected: page introduces unresolved cross-links (strict mode)",
		"broken": out,
	})
}

func (s *Server) handleListPages(w http.ResponseWriter, r *http.Request) {
	wk := s.lookupWiki(w, r)
	if wk == nil {
		return
	}
	pages, err := s.DB.ListPages(wk.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	counts, err := s.DB.PageCountsInWiki(wk.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	type pageWithCounts struct {
		store.Page
		Counts store.PageLinkCounts `json:"counts"`
	}
	out := make([]pageWithCounts, 0, len(pages))
	for _, p := range pages {
		c := counts[p.ID]
		if c == nil {
			c = &store.PageLinkCounts{}
		}
		out = append(out, pageWithCounts{Page: p, Counts: *c})
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": out})
}

type createPageReq struct {
	Title   string `json:"title"`
	RelPath string `json:"rel_path"`
	Content string `json:"content"`
}

func (s *Server) handleCreatePage(w http.ResponseWriter, r *http.Request) {
	wk := s.lookupWiki(w, r)
	if wk == nil {
		return
	}
	var req createPageReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	// Strict cross-link enforcement: reject if this content introduces broken links.
	if isStrict(r) {
		if broken, err := s.Svc.PageBrokenLinks(wk, req.RelPath, req.Content); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		} else if len(broken) > 0 {
			writeBrokenLinks(w, broken)
			return
		}
	}
	p, err := s.Svc.CreatePage(wk, req.Title, req.RelPath, req.Content)
	switch {
	case errors.Is(err, wiki.ErrUnsafePath):
		writeErr(w, http.StatusBadRequest, "unsafe rel_path")
		return
	case errors.Is(err, wiki.ErrExists):
		writeErr(w, http.StatusConflict, "page already exists")
		return
	case err != nil:
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.DB.Audit(actorName(r), "page.create", wk.ID, p.ID, "")
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleGetPage(w http.ResponseWriter, r *http.Request) {
	wk := s.lookupWiki(w, r)
	if wk == nil {
		return
	}
	pwc, err := s.Svc.GetPage(wk, r.PathValue("pageSlug"))
	if errors.Is(err, wiki.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pwc)
}

type updatePageReq struct {
	Content  string `json:"content"`
	BaseHash string `json:"base_hash"`
}

func (s *Server) handleUpdatePage(w http.ResponseWriter, r *http.Request) {
	wk := s.lookupWiki(w, r)
	if wk == nil {
		return
	}
	var req updatePageReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if isStrict(r) {
		// Resolve against the page's existing rel_path.
		existing, _ := s.DB.PageBySlug(wk.ID, r.PathValue("pageSlug"))
		if existing != nil {
			if broken, err := s.Svc.PageBrokenLinks(wk, existing.RelPath, req.Content); err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			} else if len(broken) > 0 {
				writeBrokenLinks(w, broken)
				return
			}
		}
	}
	p, err := s.Svc.UpdatePage(wk, r.PathValue("pageSlug"), req.Content, req.BaseHash)
	var conflict *wiki.ConflictError
	switch {
	case errors.As(err, &conflict):
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "conflict", "current_hash": conflict.CurrentHash,
		})
		return
	case errors.Is(err, wiki.ErrNotFound):
		writeErr(w, http.StatusNotFound, "page not found")
		return
	case err != nil:
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.DB.Audit(actorName(r), "page.update", wk.ID, p.ID, "")
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeletePage(w http.ResponseWriter, r *http.Request) {
	wk := s.lookupWiki(w, r)
	if wk == nil {
		return
	}
	slug := r.PathValue("pageSlug")
	p, _ := s.DB.PageBySlug(wk.ID, slug)
	err := s.Svc.DeletePage(wk, slug)
	if errors.Is(err, wiki.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if p != nil {
		_ = s.DB.Audit(actorName(r), "page.delete", wk.ID, p.ID, "")
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) handlePageLinks(w http.ResponseWriter, r *http.Request) {
	wk := s.lookupWiki(w, r)
	if wk == nil {
		return
	}
	p, err := s.DB.PageBySlug(wk.ID, r.PathValue("pageSlug"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if p == nil {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	outgoing, err := s.DB.OutgoingLinks(p.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	backlinks, err := s.DB.Backlinks(p.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"outgoing": outgoing, "backlinks": backlinks,
	})
}
