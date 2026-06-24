package api

import (
	"net/http"

	"github.com/plato/plato/internal/sync"
)

type syncReq struct {
	Dir    string `json:"dir"`    // server-local directory to import
	Delete bool   `json:"delete"` // soft-delete vanished pages
}

// handleSync imports a server-local directory into the wiki. The directory must be
// accessible to the server process.
func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	wk := s.lookupWiki(w, r)
	if wk == nil {
		return
	}
	var req syncReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Dir == "" {
		writeErr(w, http.StatusBadRequest, "dir required")
		return
	}
	rep, err := sync.Run(s.Svc, wk, req.Dir, req.Delete, actorName(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
