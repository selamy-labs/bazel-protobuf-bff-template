package app

import (
	"context"
	"errors"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"example.com/bfftemplate/bff/adapters/memory"
	"example.com/bfftemplate/bff/ports"
)

func fixedNonUTCClock() func() time.Time {
	return func() time.Time { return time.Date(2026, 10, 31, 1, 2, 3, 0, time.FixedZone("x", 3600)) }
}

func TestCreateTrimsTitle(t *testing.T) {
	// Arrange
	notes := New(memory.New(100))
	notes.now = fixedNonUTCClock()

	// Act
	created, err := notes.Create(context.Background(), "a", "  title  ", "body")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if created.Title != "title" {
		t.Fatalf("title not trimmed: got %q, want %q", created.Title, "title")
	}
}

func TestCreateNormalizesCreateTimeToUTC(t *testing.T) {
	// Arrange
	notes := New(memory.New(100))
	notes.now = fixedNonUTCClock()

	// Act
	created, err := notes.Create(context.Background(), "a", "title", "body")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if created.CreateTime.Location() != time.UTC {
		t.Fatalf("create time not in UTC: got %v", created.CreateTime)
	}
}

func TestGetReturnsCreatedNoteForOwner(t *testing.T) {
	// Arrange
	notes := New(memory.New(100))
	notes.now = fixedNonUTCClock()
	ctx := context.Background()
	created, err := notes.Create(ctx, "a", "  title  ", "body")
	if err != nil {
		t.Fatal(err)
	}

	// Act
	got, err := notes.Get(ctx, "a", Name(created.ID))

	// Assert
	if err != nil || got != created {
		t.Fatalf("got %+v, %v; want %+v, nil", got, err, created)
	}
}

func TestGetRefusesOtherOwner(t *testing.T) {
	// Arrange
	notes := New(memory.New(100))
	notes.now = fixedNonUTCClock()
	ctx := context.Background()
	created, err := notes.Create(ctx, "a", "title", "body")
	if err != nil {
		t.Fatal(err)
	}

	// Act
	_, err = notes.Get(ctx, "b", Name(created.ID))

	// Assert
	if !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("other owner: got %v, want %v", err, ports.ErrNotFound)
	}
}

func TestGetNeverPassesAnInvalidNameToStorage(t *testing.T) {
	cases := []struct {
		label string
		name  string
	}{
		{"empty", ""},
		{"missing id", "notes/"},
		{"uppercase id", "notes/A"},
		{"wrong collection", "tasks/a"},
		{"nested path", "notes/a/b"},
		{"trailing newline", "notes/a\n"},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			// Arrange
			notes := New(failingStore{t})

			// Act
			_, err := notes.Get(context.Background(), "a", tc.name)

			// Assert
			if !errors.Is(err, ports.ErrNotFound) {
				t.Fatalf("%q: got %v, want %v", tc.name, err, ports.ErrNotFound)
			}
		})
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
	// Arrange
	notes := New(unavailableStore{})

	// Act
	_, err := notes.Create(context.Background(), "a", "t", "")

	// Assert
	if !errors.Is(err, ports.ErrUnavailable) {
		t.Fatalf("got %v, want %v", err, ports.ErrUnavailable)
	}
}

type unavailableStore struct{}

func (unavailableStore) Put(context.Context, ports.Note) error { return ports.ErrUnavailable }

func (unavailableStore) Get(context.Context, string, string) (ports.Note, error) {
	return ports.Note{}, ports.ErrUnavailable
}

func TestGeneratedIDRoundTripsThroughNameProperty(t *testing.T) {
	// Arrange: every generated ID round-trips through its resource name.
	property := func(uint8) bool {
		id := newID()
		parsed, ok := ParseName(Name(id))
		return ok && parsed == id
	}

	// Act
	err := quick.Check(property, nil)

	// Assert
	if err != nil {
		t.Fatalf("generated ID round-trip property failed: %v", err)
	}
}

const admissibleIDAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789-"

type admissibleName string

func (admissibleName) Generate(r *rand.Rand, _ int) reflect.Value {
	n := 1 + r.Intn(64)
	var b strings.Builder
	b.Grow(len("notes/") + n)
	b.WriteString("notes/")
	for range n {
		b.WriteByte(admissibleIDAlphabet[r.Intn(len(admissibleIDAlphabet))])
	}
	return reflect.ValueOf(admissibleName(b.String()))
}

func TestParsedNameRebuildsCanonicalIDProperty(t *testing.T) {
	// Arrange: every admissible resource name parses to a valid lowercase ID and rebuilds identically.
	cfg := &quick.Config{Rand: rand.New(rand.NewSource(1))}
	property := func(raw admissibleName) bool {
		name := string(raw)
		id, ok := ParseName(name)
		return ok && Name(id) == name && len(id) >= 1 && len(id) <= 64 && id == strings.ToLower(id)
	}

	// Act
	err := quick.Check(property, cfg)

	// Assert
	if err != nil {
		t.Fatalf("parsed name canonical rebuild property failed: %v", err)
	}
}

func TestTitleProperty(t *testing.T) {
	// Arrange: a stored title is always trimmed, whatever whitespace surrounds it.
	notes := New(memory.New(100))
	property := func(prefix, title string) bool {
		created, err := notes.Create(context.Background(), "a", strings.Repeat(" ", len(prefix)%5)+title, "")
		return err == nil && created.Title == strings.TrimSpace(created.Title)
	}

	// Act
	err := quick.Check(property, nil)

	// Assert
	if err != nil {
		t.Fatalf("title trimming property failed: %v", err)
	}
}
