package api

import (
	"errors"
	"net/http"

	"github.com/plato/plato/internal/wiki"
)

type addLinkReq struct {
	// Exactly one target form, priority: to_title > to_path > to.
	To      string `json:"to"`       // target page slug
	ToPath  string `json:"to_path"`  // target rel_path (emits relative .md link)
	ToTitle string `json:"to_title"` // target title (emits [[Title]] wikilink)
	Label   string `json:"label"`    // optional display label
	// Direction: "forward" (default) adds the link to {pageSlug}; "backlink"
	// means "make {pageSlug} referenced by `from`" — i.e. add a forward link to
	// the page named by `from`, pointing back at {pageSlug}.
	Direction string `json:"direction"`
	From      string `json:"from"` // source page slug for direction=backlink
}

// handleAddLink adds an explicit forward (or backlink) cross-link by editing the
// source page's Markdown. The link is recorded with origin=manual.
func (s *Server) handleAddLink(w http.ResponseWriter, r *http.Request) {
	wk := s.lookupWiki(w, r)
	if wk == nil {
		return
	}
	pageSlug := r.PathValue("pageSlug")
	var req addLinkReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}

	spec := wiki.AddLinkSpec{ToSlug: req.To, ToPath: req.ToPath, ToTitle: req.ToTitle, Label: req.Label}

	var sourceSlug string
	if req.Direction == "backlink" {
		// Add a forward link FROM req.From pointing at this page (pageSlug).
		if req.From == "" {
			writeErr(w, http.StatusBadRequest, "backlink requires 'from'")
			return
		}
		sourceSlug = req.From
		spec = wiki.AddLinkSpec{ToSlug: pageSlug, Label: req.Label}
	} else {
		sourceSlug = pageSlug
		if req.To == "" && req.ToPath == "" && req.ToTitle == "" {
			writeErr(w, http.StatusBadRequest, "one of to, to_path, to_title required")
			return
		}
	}

	p, err := s.Svc.AddForwardLink(wk, sourceSlug, spec)
	switch {
	case errors.Is(err, wiki.ErrNotFound):
		writeErr(w, http.StatusNotFound, "page or target not found")
		return
	case errors.Is(err, wiki.ErrBadInput) || errors.Is(err, wiki.ErrUnsafePath):
		writeErr(w, http.StatusBadRequest, "invalid link target")
		return
	case err != nil:
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.DB.Audit(actorName(r), "page.link_add", wk.ID, p.ID, "")
	links, _ := s.DB.OutgoingLinks(p.ID)
	writeJSON(w, http.StatusOK, map[string]any{"page": p, "outgoing": links})
}

type removeLinkReq struct {
	Raw string `json:"raw"` // exact markdown of the link to remove, e.g. "[[database]]"
}

// handleRemoveLink removes a manual link bullet from a page's Markdown.
func (s *Server) handleRemoveLink(w http.ResponseWriter, r *http.Request) {
	wk := s.lookupWiki(w, r)
	if wk == nil {
		return
	}
	var req removeLinkReq
	if err := decodeJSON(r, &req); err != nil || req.Raw == "" {
		writeErr(w, http.StatusBadRequest, "raw link text required")
		return
	}
	p, err := s.Svc.RemoveForwardLink(wk, r.PathValue("pageSlug"), req.Raw)
	switch {
	case errors.Is(err, wiki.ErrNotFound):
		writeErr(w, http.StatusNotFound, "page or link not found")
		return
	case err != nil:
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.DB.Audit(actorName(r), "page.link_remove", wk.ID, p.ID, "")
	writeJSON(w, http.StatusOK, map[string]any{"removed": true})
}

// handleGraph returns the project's resolved-link graph (nodes + edges) for
// external agent analysis.
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	wk := s.lookupWiki(w, r)
	if wk == nil {
		return
	}
	nodes, edges, err := s.DB.Graph(wk.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"wiki":  wk.Slug,
		"nodes": nodes,
		"edges": edges,
	})
}
