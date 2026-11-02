package object

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/bfftemplate/bff/ports"
	"example.com/bfftemplate/bff/ports/porttest"
)

// A fake of the two calls the adapter makes and of the token endpoint: objects by name in memory, a token with
// an expiry, and failures a test can plant. It stands for the bucket service's API surface, not for the service.
type bucket struct {
	t        *testing.T
	mu       sync.Mutex
	objects  map[string][]byte
	tokens   int
	status   int // a planted status for every object call; 0 means none
	noToken  bool
	server   *httptest.Server
	settings Settings
}

func newBucket(t *testing.T) *bucket {
	t.Helper()
	b := &bucket{t: t, objects: map[string][]byte{}}
	b.server = httptest.NewServer(http.HandlerFunc(b.handle))
	t.Cleanup(b.server.Close)
	b.settings = Settings{Bucket: "notes-bucket", Endpoint: b.server.URL, TokenURL: b.server.URL + "/token", Prefix: "notes", TokenMaxBytes: 64 << 10, ObjectMaxBytes: 1 << 20, TokenRefreshSkew: time.Minute}
	return b
}

func (b *bucket) handle(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r.URL.Path == "/token" {
		if r.Header.Get("Metadata-Flavor") != "Google" || b.noToken {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		b.tokens++
		_, _ = io.WriteString(w, `{"access_token":"issued-`+strings.Repeat("x", b.tokens)+`","expires_in":3600}`)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer issued-") {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if b.status != 0 {
		w.WriteHeader(b.status)
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/upload/storage/v1/b/notes-bucket/o":
		name := r.URL.Query().Get("name")
		if r.URL.Query().Get("uploadType") != "media" || name == "" || r.Header.Get("Content-Type") != "application/json" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		data, _ := io.ReadAll(r.Body)
		b.objects[name] = data
		_, _ = io.WriteString(w, `{"name":"`+name+`"}`)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/storage/v1/b/notes-bucket/o/"):
		name := strings.TrimPrefix(r.URL.Path, "/storage/v1/b/notes-bucket/o/")
		data, ok := b.objects[name]
		if !ok || r.URL.Query().Get("alt") != "media" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(data)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (b *bucket) open(t *testing.T) *Store {
	t.Helper()
	store, err := Open(b.settings, b.server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestContract(t *testing.T) {
	porttest.Run(t, func(t *testing.T) ports.Store { return newBucket(t).open(t) })
}

// The hosted persistence statement (spec 012 FR-017): a note survives a restart of the service, because it is in
// the bucket; the second store reads what the first wrote, and another owner is still refused.
func TestANoteSurvivesARestart(t *testing.T) {
	b := newBucket(t)
	ctx := context.Background()
	first := b.open(t)
	note := ports.Note{Owner: "ana", ID: "n1", Title: "kept", Body: "across a restart", CreateTime: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}
	if err := first.Put(ctx, note); err != nil {
		t.Fatal(err)
	}
	second := b.open(t)
	got, err := second.Get(ctx, "ana", "n1")
	if err != nil || got != note {
		t.Fatalf("after a restart: %+v, %v", got, err)
	}
	if _, err := second.Get(ctx, "bob", "n1"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("another owner after a restart: %v", err)
	}
}

func TestNamesNeverLeaveThePrefix(t *testing.T) {
	b := newBucket(t)
	store := b.open(t)
	for _, name := range []string{"../etc", "a/b", "..", "notes/x", "%2e%2e"} {
		object := store.name(name, name)
		if !strings.HasPrefix(object, "notes/") || strings.Contains(object, "..") || strings.Count(object, "/") != 2 {
			t.Errorf("%q names %q", name, object)
		}
	}
}

func TestTheTokenIsCachedUntilItExpires(t *testing.T) {
	b := newBucket(t)
	store := b.open(t)
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	ctx := context.Background()
	for range 3 {
		if err := store.Put(ctx, ports.Note{Owner: "ana", ID: "n1"}); err != nil {
			t.Fatal(err)
		}
	}
	if b.tokens != 1 {
		t.Fatalf("%d tokens read for three calls", b.tokens)
	}
	now = now.Add(time.Hour)
	if _, err := store.Get(ctx, "ana", "n1"); err != nil {
		t.Fatal(err)
	}
	if b.tokens != 2 {
		t.Fatalf("%d tokens after the expiry", b.tokens)
	}
}

func TestEveryFailureIsUnavailableNeverEmpty(t *testing.T) {
	ctx := context.Background()
	cases := map[string]func(b *bucket){
		"the token endpoint refuses":   func(b *bucket) { b.noToken = true },
		"the bucket answers 500":       func(b *bucket) { b.status = http.StatusInternalServerError },
		"the bucket answers 403":       func(b *bucket) { b.status = http.StatusForbidden },
		"the object is not a note":     func(b *bucket) { b.objects[b.open(b.t).name("ana", "n1")] = []byte("not json") },
		"the object is another's note": func(b *bucket) { b.objects[b.open(b.t).name("ana", "n1")] = []byte(`{"owner":"bob","id":"n1"}`) },
	}
	for name, plant := range cases {
		b := newBucket(t)
		store := b.open(t)
		plant(b)
		_, err := store.Get(ctx, "ana", "n1")
		if !errors.Is(err, ports.ErrUnavailable) {
			t.Errorf("%s: get: %v", name, err)
		}
		if name != "the object is not a note" && name != "the object is another's note" {
			if err := store.Put(ctx, ports.Note{Owner: "ana", ID: "n1"}); !errors.Is(err, ports.ErrUnavailable) {
				t.Errorf("%s: put: %v", name, err)
			}
		}
	}
	b := newBucket(t)
	b.server.Close()
	store, err := Open(b.settings, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, ports.Note{Owner: "ana", ID: "n1"}); !errors.Is(err, ports.ErrUnavailable) {
		t.Errorf("an unreachable bucket: %v", err)
	}
}

func TestAnEndedContextIsReportedAsSuch(t *testing.T) {
	b := newBucket(t)
	store := b.open(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Put(ctx, ports.Note{Owner: "ana", ID: "n1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("put: %v", err)
	}
	if _, err := store.Get(ctx, "ana", "n1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("get: %v", err)
	}
}

func TestSettingsAreCheckedByName(t *testing.T) {
	b := newBucket(t)
	cases := map[string]struct {
		change func(*Settings)
		want   string
	}{
		"no bucket":     {func(s *Settings) { s.Bucket = "" }, "store.bucket: required"},
		"bad endpoint":  {func(s *Settings) { s.Endpoint = "ftp://x" }, "store.endpoint"},
		"bad token url": {func(s *Settings) { s.TokenURL = "metadata" }, "store.token_url"},
		"no prefix":     {func(s *Settings) { s.Prefix = "" }, "store.prefix"},
		"parent prefix": {func(s *Settings) { s.Prefix = "../notes" }, "store.prefix"},
	}
	for name, c := range cases {
		settings := b.settings
		c.change(&settings)
		if _, err := Open(settings, b.server.Client()); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", name, err, c.want)
		}
	}
	if _, err := Open(b.settings, nil); err == nil {
		t.Error("a store without a client was opened")
	}
}
