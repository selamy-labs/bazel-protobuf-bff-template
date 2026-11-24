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

func TestGetReturnsStoredNoteForOwner(t *testing.T) {
	// Arrange
	store := New(100)
	ctx := context.Background()
	note := ports.Note{Owner: "a", ID: "1", Title: "t"}
	if err := store.Put(ctx, note); err != nil {
		t.Fatal(err)
	}

	// Act
	got, err := store.Get(ctx, "a", "1")

	// Assert
	if err != nil || got != note {
		t.Fatalf("owner read: got %+v, %v; want %+v, nil", got, err, note)
	}
}

func TestGetReturnsNotFoundForOtherOwner(t *testing.T) {
	// Arrange
	store := New(100)
	ctx := context.Background()
	if err := store.Put(ctx, ports.Note{Owner: "a", ID: "1", Title: "t"}); err != nil {
		t.Fatal(err)
	}

	// Act
	_, err := store.Get(ctx, "b", "1")

	// Assert
	if !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("other owner: got %v, want %v", err, ports.ErrNotFound)
	}
}

func TestGetReturnsNotFoundForAbsentID(t *testing.T) {
	// Arrange
	store := New(100)
	ctx := context.Background()
	if err := store.Put(ctx, ports.Note{Owner: "a", ID: "1", Title: "t"}); err != nil {
		t.Fatal(err)
	}

	// Act
	_, err := store.Get(ctx, "a", "2")

	// Assert
	if !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("absent id: got %v, want %v", err, ports.ErrNotFound)
	}
}

func TestPutRefusesCanceledContext(t *testing.T) {
	// Arrange
	store := New(100)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act
	err := store.Put(ctx, ports.Note{Owner: "a", ID: "1"})

	// Assert
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("put with canceled context: got %v, want %v", err, context.Canceled)
	}
}

func TestGetRefusesCanceledContext(t *testing.T) {
	// Arrange
	store := New(100)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act
	_, err := store.Get(ctx, "a", "1")

	// Assert
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("get with canceled context: got %v, want %v", err, context.Canceled)
	}
}

func TestCanceledPutStoresNoNote(t *testing.T) {
	// Arrange
	store := New(100)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act
	_ = store.Put(ctx, ports.Note{Owner: "a", ID: "1"})

	// Assert
	if len(store.notes) != 0 {
		t.Fatalf("canceled put stored %d note(s); want 0", len(store.notes))
	}
}

func TestFullStoreIsUnavailable(t *testing.T) {
	// Arrange
	store := New(1)
	ctx := context.Background()
	if err := store.Put(ctx, ports.Note{Owner: "a", ID: "1"}); err != nil {
		t.Fatal(err)
	}

	// Act
	err := store.Put(ctx, ports.Note{Owner: "a", ID: "2"})

	// Assert
	if !errors.Is(err, ports.ErrUnavailable) {
		t.Fatalf("full store put: got %v, want %v", err, ports.ErrUnavailable)
	}
}

// Nothing here chooses a size: a limit below one refuses every put, visibly, rather than defaulting.
func TestNoLimitIsNoDefault(t *testing.T) {
	// Arrange
	store := New(0)

	// Act
	err := store.Put(context.Background(), ports.Note{Owner: "a", ID: "1"})

	// Assert
	if !errors.Is(err, ports.ErrUnavailable) {
		t.Fatalf("zero-limit store put: got %v, want %v", err, ports.ErrUnavailable)
	}
}
