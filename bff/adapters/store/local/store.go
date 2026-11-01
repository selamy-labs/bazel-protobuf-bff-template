// Package local is a file-backed ports.Store for the local demo (spec 012 FR-022): one file per note under a
// directory the profile names, so notes survive a restart of the demo and a reset is the removal of that
// directory by the launcher. Owners and ids become file names by hex encoding, so a subject or an id that looks
// like a path can never leave the directory. Writes are atomic through a temporary file and a rename. It is
// one process's store; nothing here is a claim about durability beyond the host's file system.
package local

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"example.com/bfftemplate/bff/ports"
)

// Store keeps notes under Dir.
type Store struct{ dir string }

type record struct {
	Owner      string    `json:"owner"`
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	CreateTime time.Time `json:"create_time"`
}

// Open prepares a store under dir, creating it if needed.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("local store: %w", err)
	}
	return &Store{dir: dir}, nil
}

func (s *Store) path(owner, id string) string {
	return filepath.Join(s.dir, hex.EncodeToString([]byte(owner)), hex.EncodeToString([]byte(id))+".json")
}

// Put writes the note atomically.
func (s *Store) Put(ctx context.Context, note ports.Note) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target := s.path(note.Owner, note.ID)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("%w: %v", ports.ErrUnavailable, err)
	}
	data, err := json.Marshal(record(note))
	if err != nil {
		return fmt.Errorf("%w: %v", ports.ErrUnavailable, err)
	}
	temp, err := os.CreateTemp(filepath.Dir(target), ".put-*")
	if err != nil {
		return fmt.Errorf("%w: %v", ports.ErrUnavailable, err)
	}
	name := temp.Name()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		_ = os.Remove(name)
		return fmt.Errorf("%w: %v", ports.ErrUnavailable, err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("%w: %v", ports.ErrUnavailable, err)
	}
	if err := os.Rename(name, target); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("%w: %v", ports.ErrUnavailable, err)
	}
	return nil
}

// Get reads the owner's note, or ports.ErrNotFound.
func (s *Store) Get(ctx context.Context, owner, id string) (ports.Note, error) {
	if err := ctx.Err(); err != nil {
		return ports.Note{}, err
	}
	data, err := os.ReadFile(s.path(owner, id))
	if errors.Is(err, fs.ErrNotExist) {
		return ports.Note{}, ports.ErrNotFound
	}
	if err != nil {
		return ports.Note{}, fmt.Errorf("%w: %v", ports.ErrUnavailable, err)
	}
	var stored record
	if err := json.Unmarshal(data, &stored); err != nil {
		return ports.Note{}, fmt.Errorf("%w: %s holds no note: %v", ports.ErrUnavailable, filepath.Base(s.path(owner, id)), err)
	}
	if stored.Owner != owner || stored.ID != id {
		return ports.Note{}, fmt.Errorf("%w: the stored note is not the one asked for", ports.ErrUnavailable)
	}
	return ports.Note(stored), nil
}
