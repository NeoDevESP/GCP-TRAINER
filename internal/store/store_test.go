package store

import (
	"errors"
	"path/filepath"
	"testing"
)

type rec struct {
	Name string `json:"name"`
	N    int    `json:"n"`
}

func TestMemoryPersistsAcrossRestarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "db.json")
	m, err := NewMemory(path)
	if err != nil {
		t.Fatal(err)
	}
	for i, n := range []string{"b", "a", "c"} {
		if err := m.Put("users", n, rec{Name: n, N: i}); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Delete("users", "c"); err != nil {
		t.Fatal(err)
	}
	var r rec
	if err := m.Get("users", "missing", &r); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	m2, err := NewMemory(path)
	if err != nil {
		t.Fatal(err)
	}
	all, err := ListAs[rec](m2, "users")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Name != "a" || all[1].Name != "b" {
		t.Fatalf("reloaded %+v (want a, b sorted by id)", all)
	}
	if err := m2.Get("users", "b", &r); err != nil || r.N != 0 {
		t.Fatalf("get %+v %v", r, err)
	}
	if empty, _ := ListAs[rec](m2, "none"); len(empty) != 0 {
		t.Fatalf("empty collection returned %v", empty)
	}
}
