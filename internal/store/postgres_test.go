package store

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// TestPostgres runs against real databases listed in GCPLAB_TEST_POSTGRES
// (space-separated DSNs), e.g. a direct connection and a transaction-mode
// pooler on port 6543. Skipped when unset.
func TestPostgres(t *testing.T) {
	dsns := strings.Fields(os.Getenv("GCPLAB_TEST_POSTGRES"))
	if len(dsns) == 0 {
		t.Skip("GCPLAB_TEST_POSTGRES not set")
	}
	for _, dsn := range dsns {
		p, err := NewPostgres(context.Background(), dsn)
		if err != nil {
			t.Fatalf("%s: %v", dsn, err)
		}
		// migrations are idempotent
		if _, err := NewPostgres(context.Background(), dsn); err != nil {
			t.Fatalf("second migration: %v", err)
		}
		coll := "test-" + strings.ReplaceAll(t.Name(), "/", "-")
		for i, n := range []string{"b", "a", "c"} {
			if err := p.Put(coll, n, rec{Name: n, N: i}); err != nil {
				t.Fatal(err)
			}
		}
		if err := p.Put(coll, "a", rec{Name: "a", N: 9}); err != nil { // upsert
			t.Fatal(err)
		}
		if err := p.Delete(coll, "c"); err != nil {
			t.Fatal(err)
		}
		var r rec
		if err := p.Get(coll, "missing", &r); !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
		all, err := ListAs[rec](p, coll)
		if err != nil {
			t.Fatal(err)
		}
		if len(all) != 2 || all[0].Name != "a" || all[0].N != 9 || all[1].Name != "b" {
			t.Fatalf("unexpected documents: %+v", all)
		}
		var rls bool
		if err := p.pool.QueryRow(context.Background(), `SELECT relrowsecurity FROM pg_class WHERE relname='documents'`).Scan(&rls); err != nil || !rls {
			t.Fatalf("row-level security must be enabled (got %v, %v)", rls, err)
		}
		for _, n := range []string{"a", "b"} {
			_ = p.Delete(coll, n)
		}
		p.Close()
	}
}
