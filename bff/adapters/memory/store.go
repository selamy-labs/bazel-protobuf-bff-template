// Package memory is an in-memory ports.Store. Data does not survive a restart.
package memory

import (
	"context"
	"sync"

	"example.com/bfftemplate/bff/ports"
)

// MaxNotes bounds the store, so the example server cannot grow without limit.
const MaxNotes = 10000

type key struct{ owner, id string }

// Store keeps notes in a map keyed by owner and ID, so one owner cannot reach another's note.
type Store struct {
	mu    sync.RWMutex
	notes map[key]ports.Note
	limit int
}

// New returns an empty store.
func New() *Store { return &Store{notes: make(map[key]ports.Note), limit: MaxNotes} }

// Put stores a note, and reports ports.ErrUnavailable once the store is full.
func (s *Store) Put(ctx context.Context, note ports.Note) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.notes) >= s.limit {
		return ports.ErrUnavailable
	}
	s.notes[key{note.Owner, note.ID}] = note
	return nil
}

// Get returns the owner's note, or ports.ErrNotFound.
func (s *Store) Get(ctx context.Context, owner, id string) (ports.Note, error) {
	if err := ctx.Err(); err != nil {
		return ports.Note{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	note, ok := s.notes[key{owner, id}]
	if !ok {
		return ports.Note{}, ports.ErrNotFound
	}
	return note, nil
}
