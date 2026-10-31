// Package ports declares what the application needs from the outside. An adopter attaches a real
// backend by implementing Store.
package ports

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNotFound means the owner has no note with that ID. It says nothing about other owners.
	ErrNotFound = errors.New("note not found")
	// ErrUnavailable means storage cannot serve the call now, and a retry may work.
	ErrUnavailable = errors.New("storage unavailable")
)

// Note is a stored note.
type Note struct {
	Owner      string
	ID         string
	Title      string
	Body       string
	CreateTime time.Time
}

// Store saves and reads notes. Every call must stop when ctx ends and return ctx.Err().
type Store interface {
	Put(ctx context.Context, note Note) error
	Get(ctx context.Context, owner, id string) (Note, error)
}
