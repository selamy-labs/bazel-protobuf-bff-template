// Package app holds the use cases. It knows nothing about transport or generated code.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"
	"time"

	"example.com/bfftemplate/bff/ports"
)

const namePrefix = "notes/"

var idPattern = regexp.MustCompile(`\A[a-z0-9-]{1,64}\z`)

// Notes is the notes use cases.
type Notes struct {
	store ports.Store
	now   func() time.Time
}

// New returns the use cases over a store.
func New(store ports.Store) *Notes { return &Notes{store: store, now: time.Now} }

// Create stores a note for owner. The caller has validated title and body.
func (n *Notes) Create(ctx context.Context, owner, title, body string) (ports.Note, error) {
	note := ports.Note{
		Owner:      owner,
		ID:         newID(),
		Title:      strings.TrimSpace(title),
		Body:       body,
		CreateTime: n.now().UTC(),
	}
	if err := n.store.Put(ctx, note); err != nil {
		return ports.Note{}, err
	}
	return note, nil
}

// Get reads owner's note by resource name. A name that cannot exist is not found.
func (n *Notes) Get(ctx context.Context, owner, name string) (ports.Note, error) {
	id, ok := ParseName(name)
	if !ok {
		return ports.Note{}, ports.ErrNotFound
	}
	return n.store.Get(ctx, owner, id)
}

// Name returns a note's resource name.
func Name(id string) string { return namePrefix + id }

// ParseName returns the ID in a resource name.
func ParseName(name string) (string, bool) {
	id, found := strings.CutPrefix(name, namePrefix)
	if !found || !idPattern.MatchString(id) {
		return "", false
	}
	return id, true
}

func newID() string {
	var raw [16]byte
	_, _ = rand.Read(raw[:]) // crypto/rand.Read never returns an error.
	return hex.EncodeToString(raw[:])
}
