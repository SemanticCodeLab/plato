package api

import "net/http"

// handleVerify reports cross-link integrity for a whole project: every
// missing/ambiguous outgoing link with its source page. This is the read side of
// cross-reference verification — agents can poll it to confirm a wiki's graph is
// consistent before/after edits.
func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	wk := s.lookupWiki(w, r)
	if wk == nil {
		return
	}
	broken, err := s.DB.BrokenLinks(wk.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	stats, err := s.DB.Stats(wk.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     len(broken) == 0,
		"stats":  stats,
		"broken": broken,
	})
}
