// Package faults decorates a store so tests can force each failure through the real server. The
// owner's subject selects the fault, which keeps no shared state and lets parallel tests inject
// different faults. The package is testonly in Bazel.
package faults

import (
	"context"
	"errors"

	"example.com/bfftemplate/bff/ports"
)

// Subjects that trigger a fault.
const (
	Unavailable = "fault-unavailable"
	Hang        = "fault-hang"
	Secret      = "fault-secret"
	Panic       = "fault-panic"
)

// PlantedSecret is in the unexpected error, so a test can show it reaches the log and no response.
const PlantedSecret = "planted-internal-detail"

// Store injects the fault named by the owner, and otherwise calls Next.
type Store struct{ Next ports.Store }

func inject(ctx context.Context, owner string) error {
	switch owner {
	case Unavailable:
		return ports.ErrUnavailable
	case Hang:
		<-ctx.Done()
		return ctx.Err()
	case Secret:
		return errors.New("connection failed: " + PlantedSecret)
	case Panic:
		panic(PlantedSecret)
	default:
		return nil
	}
}

// Put injects the owner's fault, or stores the note.
func (s Store) Put(ctx context.Context, note ports.Note) error {
	if err := inject(ctx, note.Owner); err != nil {
		return err
	}
	return s.Next.Put(ctx, note)
}

// Get injects the owner's fault, or reads the note.
func (s Store) Get(ctx context.Context, owner, id string) (ports.Note, error) {
	if err := inject(ctx, owner); err != nil {
		return ports.Note{}, err
	}
	return s.Next.Get(ctx, owner, id)
}
