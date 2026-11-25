package local

import (
	"context"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"example.com/bfftemplate/bff/ports"
	"example.com/bfftemplate/bff/ports/porttest"
)

func openStore(t *testing.T, dir string) *Store {
	t.Helper()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func mustPut(t *testing.T, store *Store, note ports.Note) {
	t.Helper()
	if err := store.Put(context.Background(), note); err != nil {
		t.Fatal(err)
	}
}

func mustReadDirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names
}

func TestContract(t *testing.T) {
	porttest.Run(t, func(t *testing.T) ports.Store {
		return openStore(t, filepath.Join(t.TempDir(), "notes"))
	})
}

func seedForRestart(t *testing.T) (*Store, ports.Note) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "notes")
	note := ports.Note{Owner: "ana", ID: "n1", Title: "kept", Body: "across restarts"}
	first := openStore(t, dir)
	mustPut(t, first, note)
	return openStore(t, dir), note
}

// A note survives a restart: a second store over the same directory reads what the first wrote. That is the
// local demo's persistence statement, and only that.
func TestANoteSurvivesARestart(t *testing.T) {
	t.Run("owner lookup succeeds", func(t *testing.T) {
		// Arrange
		reopened, _ := seedForRestart(t)

		// Act
		_, err := reopened.Get(context.Background(), "ana", "n1")

		// Assert
		if err != nil {
			t.Fatalf("after restart lookup error: got %v; want nil", err)
		}
	})

	t.Run("owner reads stored note", func(t *testing.T) {
		// Arrange
		reopened, want := seedForRestart(t)

		// Act
		got, _ := reopened.Get(context.Background(), "ana", "n1")

		// Assert
		if got != want {
			t.Fatalf("after restart note: got %+v; want %+v", got, want)
		}
	})

	t.Run("another owner gets not found", func(t *testing.T) {
		// Arrange
		reopened, _ := seedForRestart(t)

		// Act
		_, err := reopened.Get(context.Background(), "bob", "n1")

		// Assert
		if !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("another owner after restart: got %v; want %v", err, ports.ErrNotFound)
		}
	})
}

func setupAfterReset(t *testing.T) (*Store, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "notes")
	outside := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(outside, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := openStore(t, dir)
	mustPut(t, store, ports.Note{Owner: "ana", ID: "n1", Title: "gone after reset"})
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	return openStore(t, dir), outside
}

// A reset is the removal of the directory; the next open starts empty and nothing outside it is touched.
func TestResetIsTheDirectory(t *testing.T) {
	t.Run("stored note is absent after reset", func(t *testing.T) {
		// Arrange
		again, _ := setupAfterReset(t)

		// Act
		_, err := again.Get(context.Background(), "ana", "n1")

		// Assert
		if !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("after reset: got %v; want %v", err, ports.ErrNotFound)
		}
	})

	t.Run("outside file remains readable", func(t *testing.T) {
		// Arrange
		_, outside := setupAfterReset(t)

		// Act
		_, err := os.ReadFile(outside)

		// Assert
		if err != nil {
			t.Fatalf("outside file read error: got %v; want nil", err)
		}
	})

	t.Run("outside file content is unchanged", func(t *testing.T) {
		// Arrange
		_, outside := setupAfterReset(t)

		// Act
		data, _ := os.ReadFile(outside)

		// Assert
		if string(data) != "mine" {
			t.Fatalf("outside file content: got %q; want %q", data, "mine")
		}
	})
}

func TestNamesNeverLeaveTheDirectory(t *testing.T) {
	t.Run("traversal-like names are accepted", func(t *testing.T) {
		// Arrange
		store := openStore(t, filepath.Join(t.TempDir(), "notes"))

		// Act
		err := store.Put(context.Background(), ports.Note{Owner: "../../escape", ID: "../x", Title: "t"})

		// Assert
		if err != nil {
			t.Fatalf("traversal-like put error: got %v; want nil", err)
		}
	})

	t.Run("root contains only the store directory", func(t *testing.T) {
		// Arrange
		root := t.TempDir()
		store := openStore(t, filepath.Join(root, "notes"))
		mustPut(t, store, ports.Note{Owner: "../../escape", ID: "../x", Title: "t"})

		// Act
		names := mustReadDirNames(t, root)

		// Assert
		if !slices.Equal(names, []string{"notes"}) {
			t.Fatalf("root entries: got %v; want [notes]", names)
		}
	})
}

func TestADamagedFileIsUnavailableNotEmpty(t *testing.T) {
	cases := []struct {
		name    string
		payload string
	}{
		{"corrupt json", "{not json"},
		{"mismatched owner", `{"owner":"bob","id":"n1","title":"swapped"}`},
		{"mismatched id", `{"owner":"ana","id":"n2","title":"swapped"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			store := openStore(t, filepath.Join(t.TempDir(), "notes"))
			mustPut(t, store, ports.Note{Owner: "ana", ID: "n1", Title: "t"})
			if err := os.WriteFile(store.path("ana", "n1"), []byte(tc.payload), 0o600); err != nil {
				t.Fatal(err)
			}

			// Act
			_, err := store.Get(context.Background(), "ana", "n1")

			// Assert
			if !errors.Is(err, ports.ErrUnavailable) {
				t.Fatalf("%s: got %v; want %v", tc.name, err, ports.ErrUnavailable)
			}
		})
	}
}

func TestPutWithOutOfRangeTimestampIsUnavailable(t *testing.T) {
	// Arrange
	store := openStore(t, filepath.Join(t.TempDir(), "notes"))
	note := ports.Note{
		Owner:      "ana",
		ID:         "n1",
		Title:      "out of range",
		CreateTime: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	// Act
	err := store.Put(context.Background(), note)

	// Assert
	if !errors.Is(err, ports.ErrUnavailable) {
		t.Fatalf("out-of-range create time: got %v; want %v", err, ports.ErrUnavailable)
	}
}

func TestOpenUnderRegularFileFails(t *testing.T) {
	// Arrange
	asFile := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(asFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Act
	_, err := Open(filepath.Join(asFile, "notes"))

	// Assert
	if err == nil {
		t.Fatal("open under regular file: got nil error; want error")
	}
}

// The file system's refusals are unavailability, never silence or an empty read.
func TestFileSystemRefusalsAreUnavailable(t *testing.T) {
	cases := []struct {
		name    string
		arrange func(t *testing.T, dir string, store *Store)
		act     func(store *Store) error
	}{
		{
			name: "unreadable note file",
			arrange: func(t *testing.T, _ string, store *Store) {
				mustPut(t, store, ports.Note{Owner: "ana", ID: "n1", Title: "t"})
				target := store.path("ana", "n1")
				if err := os.Chmod(target, 0o000); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(target, 0o600) })
			},
			act: func(store *Store) error {
				_, err := store.Get(context.Background(), "ana", "n1")
				return err
			},
		},
		{
			name: "read-only owner directory",
			arrange: func(t *testing.T, _ string, store *Store) {
				mustPut(t, store, ports.Note{Owner: "ana", ID: "n1", Title: "t"})
				ownerDir := filepath.Dir(store.path("ana", "n1"))
				if err := os.Chmod(ownerDir, 0o500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(ownerDir, 0o700) })
			},
			act: func(store *Store) error {
				return store.Put(context.Background(), ports.Note{Owner: "ana", ID: "n2", Title: "t"})
			},
		},
		{
			name: "read-only store directory",
			arrange: func(t *testing.T, dir string, _ *Store) {
				if err := os.Chmod(dir, 0o500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
			},
			act: func(store *Store) error {
				return store.Put(context.Background(), ports.Note{Owner: "bob", ID: "n1", Title: "t"})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			dir := filepath.Join(t.TempDir(), "notes")
			store := openStore(t, dir)
			tc.arrange(t, dir, store)

			// Act
			err := tc.act(store)

			// Assert
			if !errors.Is(err, ports.ErrUnavailable) {
				t.Fatalf("%s: got %v; want %v", tc.name, err, ports.ErrUnavailable)
			}
		})
	}
}

func setupDirectoryCollision(t *testing.T) (*Store, string) {
	t.Helper()
	store := openStore(t, filepath.Join(t.TempDir(), "notes"))
	target := store.path("ana", "n1")
	if err := os.MkdirAll(filepath.Join(target, "child"), 0o700); err != nil {
		t.Fatal(err)
	}
	return store, target
}

func hasTempPutFile(t *testing.T, dir string) bool {
	t.Helper()
	for _, name := range mustReadDirNames(t, dir) {
		if strings.HasPrefix(name, ".put-") {
			return true
		}
	}
	return false
}

// A directory sitting where the note's file belongs defeats the final rename, and that is unavailability too.
func TestADirectoryInTheNotesPlaceIsUnavailable(t *testing.T) {
	t.Run("put returns unavailable", func(t *testing.T) {
		// Arrange
		store, _ := setupDirectoryCollision(t)

		// Act
		err := store.Put(context.Background(), ports.Note{Owner: "ana", ID: "n1", Title: "t"})

		// Assert
		if !errors.Is(err, ports.ErrUnavailable) {
			t.Fatalf("rename onto directory: got %v; want %v", err, ports.ErrUnavailable)
		}
	})

	t.Run("temporary file is cleaned up", func(t *testing.T) {
		// Arrange
		store, target := setupDirectoryCollision(t)

		// Act
		_ = store.Put(context.Background(), ports.Note{Owner: "ana", ID: "n1", Title: "t"})
		leftover := hasTempPutFile(t, filepath.Dir(target))

		// Assert
		if leftover {
			t.Fatalf("temporary .put-* file left behind in %s", filepath.Dir(target))
		}
	})
}

var pathAlphabet = []rune("abcABC012/\\..~ \t\n\x00é中")

type pathLikePair struct {
	owner string
	id    string
}

func randomPathLikeString(r *rand.Rand, maxLen int) string {
	n := r.Intn(maxLen + 1)
	var b strings.Builder
	for range n {
		b.WriteRune(pathAlphabet[r.Intn(len(pathAlphabet))])
	}
	return b.String()
}

func (pathLikePair) Generate(r *rand.Rand, _ int) reflect.Value {
	return reflect.ValueOf(pathLikePair{
		owner: randomPathLikeString(r, 24),
		id:    randomPathLikeString(r, 24),
	})
}

func TestPathStaysInsideStoreDirectoryProperty(t *testing.T) {
	// Arrange
	store := &Store{dir: t.TempDir()}
	prefix := filepath.Clean(store.dir) + string(filepath.Separator)
	cfg := &quick.Config{Rand: rand.New(rand.NewSource(1))}
	property := func(pair pathLikePair) bool {
		got := filepath.Clean(store.path(pair.owner, pair.id))
		return strings.HasPrefix(got, prefix)
	}

	// Act
	err := quick.Check(property, cfg)

	// Assert
	if err != nil {
		t.Fatalf("path containment property failed: %v", err)
	}
}

func TestPathEndsWithJSONExtensionProperty(t *testing.T) {
	// Arrange
	store := &Store{dir: t.TempDir()}
	cfg := &quick.Config{Rand: rand.New(rand.NewSource(1))}
	property := func(pair pathLikePair) bool {
		return filepath.Ext(store.path(pair.owner, pair.id)) == ".json"
	}

	// Act
	err := quick.Check(property, cfg)

	// Assert
	if err != nil {
		t.Fatalf("path extension property failed: %v", err)
	}
}

type admissibleNote struct {
	note ports.Note
}

func (admissibleNote) Generate(r *rand.Rand, _ int) reflect.Value {
	title := "t" + randomPathLikeString(r, 24)
	return reflect.ValueOf(admissibleNote{
		note: ports.Note{
			Owner:      randomPathLikeString(r, 16),
			ID:         randomPathLikeString(r, 16),
			Title:      title,
			Body:       randomPathLikeString(r, 32),
			CreateTime: time.Unix(r.Int63n(2_000_000_000), 0).UTC(),
		},
	})
}

func TestPutAcceptsAdmissibleNoteProperty(t *testing.T) {
	// Arrange
	store := openStore(t, filepath.Join(t.TempDir(), "notes"))
	ctx := context.Background()
	cfg := &quick.Config{Rand: rand.New(rand.NewSource(1))}
	property := func(sample admissibleNote) bool {
		return store.Put(ctx, sample.note) == nil
	}

	// Act
	err := quick.Check(property, cfg)

	// Assert
	if err != nil {
		t.Fatalf("put admissible note property failed: %v", err)
	}
}

func TestPutGetRoundTripPreservesNoteProperty(t *testing.T) {
	// Arrange
	store := openStore(t, filepath.Join(t.TempDir(), "notes"))
	ctx := context.Background()
	cfg := &quick.Config{Rand: rand.New(rand.NewSource(1))}
	property := func(sample admissibleNote) bool {
		_ = store.Put(ctx, sample.note)
		got, _ := store.Get(ctx, sample.note.Owner, sample.note.ID)
		return got == sample.note
	}

	// Act
	err := quick.Check(property, cfg)

	// Assert
	if err != nil {
		t.Fatalf("put/get round-trip property failed: %v", err)
	}
}
