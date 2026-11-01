// Package porttest is the contract every ports.Store must keep (spec 011 FR-011, spec 012 FR-017, FR-022): the
// working fake and every real adapter run the same suite, so a difference between them is a finding, not a
// special case. open returns a fresh, empty store for one test; the suite never reaches outside the port.
package porttest

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/bfftemplate/bff/ports"
)

// Run exercises one store implementation against the contract, one subtest per clause.
func Run(t *testing.T, open func(t *testing.T) ports.Store) {
	t.Helper()
	for _, clause := range []struct {
		name string
		run  func(t *testing.T, open func(t *testing.T) ports.Store)
	}{
		{"a note reads back for its owner", readsBack},
		{"another owner never sees it, and an absent id is not found", ownerIsolation},
		{"the same id for two owners holds two notes", sameIDTwoOwners},
		{"a put with the same owner and id replaces the note", putReplaces},
		{"an ended context stops the call and stores nothing", endedContext},
		{"owners and ids that look like paths stay separate", pathLikeNames},
	} {
		t.Run(clause.name, func(t *testing.T) { clause.run(t, open) })
	}
}

func readsBack(t *testing.T, open func(t *testing.T) ports.Store) {
	store := open(t)
	ctx := context.Background()
	note := ports.Note{Owner: "ana", ID: "n1", Title: "Shopping", Body: "milk", CreateTime: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	if err := store.Put(ctx, note); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, "ana", "n1")
	if err != nil || got != note {
		t.Fatalf("read back %+v, %v; want %+v", got, err, note)
	}
}

func ownerIsolation(t *testing.T, open func(t *testing.T) ports.Store) {
	store := open(t)
	ctx := context.Background()
	if err := store.Put(ctx, ports.Note{Owner: "ana", ID: "n1", Title: "t"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "bob", "n1"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("other owner: want not found, got %v", err)
	}
	if _, err := store.Get(ctx, "ana", "n2"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("absent id: want not found, got %v", err)
	}
	if _, err := store.Get(ctx, "", "n1"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("empty owner: want not found, got %v", err)
	}
}

func sameIDTwoOwners(t *testing.T, open func(t *testing.T) ports.Store) {
	store := open(t)
	ctx := context.Background()
	for _, owner := range []string{"ana", "bob"} {
		if err := store.Put(ctx, ports.Note{Owner: owner, ID: "shared", Title: owner}); err != nil {
			t.Fatal(err)
		}
	}
	for _, owner := range []string{"ana", "bob"} {
		got, err := store.Get(ctx, owner, "shared")
		if err != nil || got.Title != owner {
			t.Fatalf("%s: %+v, %v", owner, got, err)
		}
	}
}

func putReplaces(t *testing.T, open func(t *testing.T) ports.Store) {
	store := open(t)
	ctx := context.Background()
	if err := store.Put(ctx, ports.Note{Owner: "ana", ID: "n1", Title: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, ports.Note{Owner: "ana", ID: "n1", Title: "second"}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, "ana", "n1")
	if err != nil || got.Title != "second" {
		t.Fatalf("%+v, %v", got, err)
	}
}

func endedContext(t *testing.T, open func(t *testing.T) ports.Store) {
	store := open(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Put(ctx, ports.Note{Owner: "ana", ID: "n1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("put: %v", err)
	}
	if _, err := store.Get(ctx, "ana", "n1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("get: %v", err)
	}
	if _, err := store.Get(context.Background(), "ana", "n1"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("a cancelled put stored a note: %v", err)
	}
}

func pathLikeNames(t *testing.T, open func(t *testing.T) ports.Store) {
	store := open(t)
	ctx := context.Background()
	pairs := [][2]string{{"a/b", "c"}, {"a", "b/c"}, {"..", "x"}, {"a", ".."}, {"a\x00b", "c"}}
	for i, pair := range pairs {
		if err := store.Put(ctx, ports.Note{Owner: pair[0], ID: pair[1], Title: string(rune('a' + i))}); err != nil {
			t.Fatalf("%v: %v", pair, err)
		}
	}
	for i, pair := range pairs {
		got, err := store.Get(ctx, pair[0], pair[1])
		if err != nil || got.Title != string(rune('a'+i)) {
			t.Fatalf("%v: %+v, %v", pair, got, err)
		}
	}
}
