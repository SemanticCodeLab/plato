package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/plato/plato/internal/auth"
	"github.com/plato/plato/internal/store"
)

type ctxKey int

const tokenCtxKey ctxKey = 0

// scope requirement constants for require().
const (
	readScope  = auth.ScopeRead
	writeScope = auth.ScopeWrite
)

// require wraps a handler, enforcing a valid Bearer token with the given scope.
func (s *Server) require(scope string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := bearerToken(r)
		if raw == "" {
			writeErr(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		tok, err := s.DB.TokenByHash(auth.Hash(raw))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if tok == nil {
			writeErr(w, http.StatusUnauthorized, "invalid or revoked token")
			return
		}
		if !auth.HasScope(tok.Scopes, scope) {
			writeErr(w, http.StatusForbidden, "token missing scope: "+scope)
			return
		}
		ctx := context.WithValue(r.Context(), tokenCtxKey, tok)
		next(w, r.WithContext(ctx))
	}
}

// bearerToken extracts the raw token from the Authorization header.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const p = "Bearer "
	if !strings.HasPrefix(h, p) {
		return ""
	}
	return strings.TrimSpace(h[len(p):])
}

// actorName returns the token name for audit logging, or "unknown".
func actorName(r *http.Request) string {
	if tok, ok := r.Context().Value(tokenCtxKey).(*store.Token); ok {
		return tok.Name
	}
	return "unknown"
}
