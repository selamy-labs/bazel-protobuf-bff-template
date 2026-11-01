package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"example.com/bfftemplate/bff/adapters/memory"
	"example.com/bfftemplate/bff/ports"
)

func TestCreateThenGet(t *testing.T) {
	notes := New(memory.New(100))
	notes.now = func() time.Time { return time.Date(2026, 10, 31, 1, 2, 3, 0, time.FixedZone("x", 3600)) }
	ctx := context.Background()
	created, err := notes.Create(ctx, "a", "  title  ", "body")
	if err != nil {
		t.Fatal(err)
	}
	if created.Title != "title" {
		t.Errorf("title not trimmed: %q", created.Title)
	}
	if created.CreateTime.Location() != time.UTC {
		t.Errorf("create time not in UTC: %v", created.CreateTime)
	}
	got, err := notes.Get(ctx, "a", Name(created.ID))
	if err != nil || got != created {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := notes.Get(ctx, "b", Name(created.ID)); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("other owner: %v", err)
	}
}

func TestGetNeverPassesAnInvalidNameToStorage(t *testing.T) {
	notes := New(failingStore{t})
	for _, name := range []string{"", "notes/", "notes/A", "tasks/a", "notes/a/b", "notes/a\n"} {
		if _, err := notes.Get(context.Background(), "a", name); !errors.Is(err, ports.ErrNotFound) {
			t.Errorf("%q: %v", name, err)
		}
	}
}

type failingStore struct{ t *testing.T }

func (s failingStore) Put(context.Context, ports.Note) error {
	s.t.Error("Put was called")
	return nil
}

func (s failingStore) Get(context.Context, string, string) (ports.Note, error) {
	s.t.Error("Get was called")
	return ports.Note{}, nil
}

func TestStoreFailureIsReturned(t *testing.T) {
	notes := New(unavailableStore{})
	if _, err := notes.Create(context.Background(), "a", "t", ""); !errors.Is(err, ports.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
}

type unavailableStore struct{}

func (unavailableStore) Put(context.Context, ports.Note) error { return ports.ErrUnavailable }

func (unavailableStore) Get(context.Context, string, string) (ports.Note, error) {
	return ports.Note{}, ports.ErrUnavailable
}

func TestNameProperties(t *testing.T) {
	// Every generated ID round-trips through its name.
	roundTrip := func(uint8) bool {
		id := newID()
		parsed, ok := ParseName(Name(id))
		return ok && parsed == id
	}
	if err := quick.Check(roundTrip, nil); err != nil {
		t.Error(err)
	}
	// Whatever parses is a valid ID, and rebuilding the name gives the input back.
	parser := func(name string) bool {
		id, ok := ParseName(name)
		if !ok {
			return true
		}
		return Name(id) == name && len(id) >= 1 && len(id) <= 64 && id == strings.ToLower(id)
	}
	if err := quick.Check(parser, nil); err != nil {
		t.Error(err)
	}
}

func TestTitleProperty(t *testing.T) {
	// A stored title is always trimmed, whatever surrounds it.
	notes := New(memory.New(100))
	property := func(prefix, title string) bool {
		created, err := notes.Create(context.Background(), "a", strings.Repeat(" ", len(prefix)%5)+title, "")
		return err == nil && created.Title == strings.TrimSpace(created.Title)
	}
	if err := quick.Check(property, nil); err != nil {
		t.Error(err)
	}
}
