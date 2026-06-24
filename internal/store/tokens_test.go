package store

import (
	"path/filepath"
	"testing"

	"github.com/plato/plato/internal/auth"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestTokenLifecycle(t *testing.T) {
	db := testDB(t)

	raw, hash := auth.Generate()
	id, err := db.CreateToken("agent", hash, "read,write")
	if err != nil {
		t.Fatal(err)
	}

	// Verify lookup by hash succeeds.
	tok, err := db.TokenByHash(auth.Hash(raw))
	if err != nil || tok == nil {
		t.Fatalf("expected token, got %v err %v", tok, err)
	}
	if !auth.HasScope(tok.Scopes, auth.ScopeWrite) {
		t.Error("expected write scope")
	}

	// Revoke -> lookup fails.
	ok, err := db.RevokeToken(id)
	if err != nil || !ok {
		t.Fatalf("revoke failed: ok=%v err=%v", ok, err)
	}
	tok, err = db.TokenByHash(auth.Hash(raw))
	if err != nil {
		t.Fatal(err)
	}
	if tok != nil {
		t.Error("revoked token should not validate")
	}
}

func TestReadTokenLacksWrite(t *testing.T) {
	if auth.HasScope("read", auth.ScopeWrite) {
		t.Error("read-only token should not have write scope")
	}
	if !auth.HasScope("read", auth.ScopeRead) {
		t.Error("read token should have read scope")
	}
}
