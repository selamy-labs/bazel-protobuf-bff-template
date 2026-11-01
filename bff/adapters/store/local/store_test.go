package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"example.com/bfftemplate/bff/ports"
	"example.com/bfftemplate/bff/ports/porttest"
)

func TestContract(t *testing.T) {
	porttest.Run(t, func(t *testing.T) ports.Store {
		store, err := Open(filepath.Join(t.TempDir(), "notes"))
		if err != nil {
			t.Fatal(err)
		}
		return store
	})
}

// A note survives a restart: a second store over the same directory reads what the first wrote. That is the
// local demo's persistence statement, and only that.
func TestANoteSurvivesARestart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "notes")
	ctx := context.Background()
	first, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	note := ports.Note{Owner: "ana", ID: "n1", Title: "kept", Body: "across restarts"}
	if err := first.Put(ctx, note); err != nil {
		t.Fatal(err)
	}
	second, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := second.Get(ctx, "ana", "n1")
	if err != nil || got != note {
		t.Fatalf("after restart: %+v, %v", got, err)
	}
	if _, err := second.Get(ctx, "bob", "n1"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("another owner after restart: %v", err)
	}
}

// A reset is the removal of the directory; the next open starts empty and nothing outside it is touched.
func TestResetIsTheDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "notes")
	outside := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(outside, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, ports.Note{Owner: "ana", ID: "n1", Title: "gone after reset"}); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := again.Get(ctx, "ana", "n1"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("after reset: %v", err)
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "mine" {
		t.Fatalf("reset touched a file outside the directory: %q %v", data, err)
	}
}

func TestNamesNeverLeaveTheDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "notes")
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), ports.Note{Owner: "../../escape", ID: "../x", Title: "t"}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "notes" {
		t.Fatalf("a name escaped the store's directory: %v", entries)
	}
}

func TestADamagedFileIsUnavailableNotEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "notes")
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Put(ctx, ports.Note{Owner: "ana", ID: "n1", Title: "t"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.path("ana", "n1"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "ana", "n1"); !errors.Is(err, ports.ErrUnavailable) {
		t.Fatalf("damaged file: want unavailable, got %v", err)
	}
	if err := os.WriteFile(store.path("ana", "n1"), []byte(`{"owner":"bob","id":"n1","title":"swapped"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "ana", "n1"); !errors.Is(err, ports.ErrUnavailable) {
		t.Fatalf("a file holding another owner's note: want unavailable, got %v", err)
	}
}
