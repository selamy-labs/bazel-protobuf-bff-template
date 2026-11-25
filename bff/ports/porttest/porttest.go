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
		{"another owner does not see a stored note", otherOwnerNotFound},
		{"an absent id is not found", absentIDNotFound},
		{"an empty owner does not see a stored note", emptyOwnerNotFound},
		{"the same id for two owners holds two notes", sameIDTwoOwners},
		{"a put with the same owner and id replaces the note", putReplaces},
		{"an ended context stops Put", endedContextStopsPut},
		{"an ended context stops Get", endedContextStopsGet},
		{"a canceled Put stores no note", canceledPutStoresNoNote},
		{"owners and ids that look like paths stay separate", pathLikeNames},
	} {
		t.Run(clause.name, func(t *testing.T) { clause.run(t, open) })
	}
}

func readsBack(t *testing.T, open func(t *testing.T) ports.Store) {
	note := ports.Note{Owner: "ana", ID: "n1", Title: "Shopping", Body: "milk", CreateTime: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}

	t.Run("succeeds", func(t *testing.T) {
		// Arrange
		store := open(t)
		ctx := context.Background()
		if err := store.Put(ctx, note); err != nil {
			t.Fatal(err)
		}

		// Act
		_, err := store.Get(ctx, "ana", "n1")

		// Assert
		if err != nil {
			t.Fatalf("read back error: got %v; want nil", err)
		}
	})

	t.Run("returns stored note", func(t *testing.T) {
		// Arrange
		store := open(t)
		ctx := context.Background()
		if err := store.Put(ctx, note); err != nil {
			t.Fatal(err)
		}

		// Act
		got, _ := store.Get(ctx, "ana", "n1")

		// Assert
		if got != note {
			t.Fatalf("read back note: got %+v; want %+v", got, note)
		}
	})
}

func otherOwnerNotFound(t *testing.T, open func(t *testing.T) ports.Store) {
	// Arrange
	store := open(t)
	ctx := context.Background()
	if err := store.Put(ctx, ports.Note{Owner: "ana", ID: "n1", Title: "t"}); err != nil {
		t.Fatal(err)
	}

	// Act
	_, err := store.Get(ctx, "bob", "n1")

	// Assert
	if !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("other owner: got %v, want %v", err, ports.ErrNotFound)
	}
}

func absentIDNotFound(t *testing.T, open func(t *testing.T) ports.Store) {
	// Arrange
	store := open(t)
	ctx := context.Background()
	if err := store.Put(ctx, ports.Note{Owner: "ana", ID: "n1", Title: "t"}); err != nil {
		t.Fatal(err)
	}

	// Act
	_, err := store.Get(ctx, "ana", "n2")

	// Assert
	if !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("absent id: got %v, want %v", err, ports.ErrNotFound)
	}
}

func emptyOwnerNotFound(t *testing.T, open func(t *testing.T) ports.Store) {
	// Arrange
	store := open(t)
	ctx := context.Background()
	if err := store.Put(ctx, ports.Note{Owner: "ana", ID: "n1", Title: "t"}); err != nil {
		t.Fatal(err)
	}

	// Act
	_, err := store.Get(ctx, "", "n1")

	// Assert
	if !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("empty owner: got %v, want %v", err, ports.ErrNotFound)
	}
}

func sameIDTwoOwners(t *testing.T, open func(t *testing.T) ports.Store) {
	owners := []string{"ana", "bob"}
	seedBoth := func(t *testing.T) ports.Store {
		t.Helper()
		store := open(t)
		ctx := context.Background()
		for _, owner := range owners {
			if err := store.Put(ctx, ports.Note{Owner: owner, ID: "shared", Title: owner}); err != nil {
				t.Fatal(err)
			}
		}
		return store
	}

	for _, targetOwner := range owners {
		t.Run(targetOwner+"/succeeds", func(t *testing.T) {
			// Arrange
			store := seedBoth(t)

			// Act
			_, err := store.Get(context.Background(), targetOwner, "shared")

			// Assert
			if err != nil {
				t.Fatalf("%s lookup error: got %v; want nil", targetOwner, err)
			}
		})

		t.Run(targetOwner+"/returns owner title", func(t *testing.T) {
			// Arrange
			store := seedBoth(t)

			// Act
			got, _ := store.Get(context.Background(), targetOwner, "shared")

			// Assert
			if got.Title != targetOwner {
				t.Fatalf("%s title: got %q; want %q", targetOwner, got.Title, targetOwner)
			}
		})
	}
}

func putReplaces(t *testing.T, open func(t *testing.T) ports.Store) {
	seedOverwrite := func(t *testing.T) ports.Store {
		t.Helper()
		store := open(t)
		ctx := context.Background()
		if err := store.Put(ctx, ports.Note{Owner: "ana", ID: "n1", Title: "first"}); err != nil {
			t.Fatal(err)
		}
		if err := store.Put(ctx, ports.Note{Owner: "ana", ID: "n1", Title: "second"}); err != nil {
			t.Fatal(err)
		}
		return store
	}

	t.Run("succeeds", func(t *testing.T) {
		// Arrange
		store := seedOverwrite(t)

		// Act
		_, err := store.Get(context.Background(), "ana", "n1")

		// Assert
		if err != nil {
			t.Fatalf("after overwrite lookup error: got %v; want nil", err)
		}
	})

	t.Run("returns replacement title", func(t *testing.T) {
		// Arrange
		store := seedOverwrite(t)

		// Act
		got, _ := store.Get(context.Background(), "ana", "n1")

		// Assert
		if got.Title != "second" {
			t.Fatalf("after overwrite title: got %q; want %q", got.Title, "second")
		}
	})
}

func endedContextStopsPut(t *testing.T, open func(t *testing.T) ports.Store) {
	// Arrange
	store := open(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act
	err := store.Put(ctx, ports.Note{Owner: "ana", ID: "n1"})

	// Assert
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("put with canceled context: got %v, want %v", err, context.Canceled)
	}
}

func endedContextStopsGet(t *testing.T, open func(t *testing.T) ports.Store) {
	// Arrange
	store := open(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act
	_, err := store.Get(ctx, "ana", "n1")

	// Assert
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("get with canceled context: got %v, want %v", err, context.Canceled)
	}
}

func canceledPutStoresNoNote(t *testing.T, open func(t *testing.T) ports.Store) {
	// Arrange
	store := open(t)
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = store.Put(canceledCtx, ports.Note{Owner: "ana", ID: "n1"})

	// Act
	_, err := store.Get(context.Background(), "ana", "n1")

	// Assert
	if !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("after canceled put: got %v, want %v", err, ports.ErrNotFound)
	}
}

func pathLikeNames(t *testing.T, open func(t *testing.T) ports.Store) {
	cases := []struct {
		name  string
		owner string
		id    string
		title string
	}{
		{"slash in owner", "a/b", "c", "a"},
		{"slash in id", "a", "b/c", "b"},
		{"dotdot owner", "..", "x", "c"},
		{"dotdot id", "a", "..", "d"},
		{"nul byte in owner", "a\x00b", "c", "e"},
	}

	seedAll := func(t *testing.T) ports.Store {
		t.Helper()
		store := open(t)
		ctx := context.Background()
		for _, seed := range cases {
			if err := store.Put(ctx, ports.Note{Owner: seed.owner, ID: seed.id, Title: seed.title}); err != nil {
				t.Fatalf("seed %s: %v", seed.name, err)
			}
		}
		return store
	}

	for _, tc := range cases {
		t.Run(tc.name+"/succeeds", func(t *testing.T) {
			// Arrange
			store := seedAll(t)

			// Act
			_, err := store.Get(context.Background(), tc.owner, tc.id)

			// Assert
			if err != nil {
				t.Fatalf("%s lookup error: got %v; want nil", tc.name, err)
			}
		})

		t.Run(tc.name+"/returns stored title", func(t *testing.T) {
			// Arrange
			store := seedAll(t)

			// Act
			got, _ := store.Get(context.Background(), tc.owner, tc.id)

			// Assert
			if got.Title != tc.title {
				t.Fatalf("%s title: got %q; want %q", tc.name, got.Title, tc.title)
			}
		})
	}
}
