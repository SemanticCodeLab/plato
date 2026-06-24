package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/plato/plato/internal/auth"
)

type createTokenReq struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

type createTokenResp struct {
	ID     int64    `json:"id"`
	Name   string   `json:"name"`
	Token  string   `json:"token"`
	Scopes []string `json:"scopes"`
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	var req createTokenReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	scopes := auth.NormalizeScopes(req.Scopes)
	raw, hash := auth.Generate()
	id, err := s.DB.CreateToken(req.Name, hash, scopes)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.DB.Audit(actorName(r), "token.create", 0, 0, `{"name":"`+req.Name+`"}`)
	writeJSON(w, http.StatusCreated, createTokenResp{
		ID: id, Name: req.Name, Token: raw, Scopes: strings.Split(scopes, ","),
	})
}

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	toks, err := s.DB.ListTokens()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	type item struct {
		ID       int64    `json:"id"`
		Name     string   `json:"name"`
		Scopes   []string `json:"scopes"`
		Created  string   `json:"created_at"`
		Revoked  bool     `json:"revoked"`
		LastUsed string   `json:"last_used_at,omitempty"`
	}
	out := []item{}
	for _, t := range toks {
		out = append(out, item{
			ID: t.ID, Name: t.Name, Scopes: strings.Split(t.Scopes, ","),
			Created: t.CreatedAt, Revoked: t.RevokedAt.Valid, LastUsed: t.LastUsedAt.String,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": out})
}

func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	ok, err := s.DB.RevokeToken(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "token not found")
		return
	}
	_ = s.DB.Audit(actorName(r), "token.revoke", 0, 0, "")
	writeJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}
