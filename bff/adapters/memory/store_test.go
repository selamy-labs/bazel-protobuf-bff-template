package memory

import (
	"context"
	"errors"
	"testing"

	"example.com/bfftemplate/bff/ports"
	"example.com/bfftemplate/bff/ports/porttest"
)

func TestContract(t *testing.T) {
	porttest.Run(t, func(t *testing.T) ports.Store { return New(100) })
}

func TestOwnerScope(t *testing.T) {
	store := New(100)
	ctx := context.Background()
	note := ports.Note{Owner: "a", ID: "1", Title: "t"}
	if err := store.Put(ctx, note); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, "a", "1")
	if err != nil || got != note {
		t.Fatalf("owner read: %+v, %v", got, err)
	}
	if _, err := store.Get(ctx, "b", "1"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("other owner: want not found, got %v", err)
	}
	if _, err := store.Get(ctx, "a", "2"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("absent id: want not found, got %v", err)
	}
}

func TestEndedContextStopsTheCall(t *testing.T) {
	store := New(100)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Put(ctx, ports.Note{Owner: "a", ID: "1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("put: %v", err)
	}
	if _, err := store.Get(ctx, "a", "1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("get: %v", err)
	}
	if len(store.notes) != 0 {
		t.Fatal("a cancelled put stored a note")
	}
}

func TestFullStoreIsUnavailable(t *testing.T) {
	store := New(1)
	ctx := context.Background()
	if err := store.Put(ctx, ports.Note{Owner: "a", ID: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, ports.Note{Owner: "a", ID: "2"}); !errors.Is(err, ports.ErrUnavailable) {
		t.Fatalf("want unavailable, got %v", err)
	}
}

// Nothing here chooses a size: a limit below one refuses every put, visibly, rather than defaulting.
func TestNoLimitIsNoDefault(t *testing.T) {
	store := New(0)
	if err := store.Put(context.Background(), ports.Note{Owner: "a", ID: "1"}); !errors.Is(err, ports.ErrUnavailable) {
		t.Fatalf("want unavailable, got %v", err)
	}
}
