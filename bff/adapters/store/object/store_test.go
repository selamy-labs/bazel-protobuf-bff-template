package object

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/quick"
	"time"

	"example.com/bfftemplate/bff/ports"
	"example.com/bfftemplate/bff/ports/porttest"
)

// A fake of the two calls the adapter makes and of the token endpoint: objects by name in memory, a token with
// an expiry, and failures a test can plant. It stands for the bucket service's API surface, not for the service.
type bucket struct {
	t         *testing.T
	mu        sync.Mutex
	objects   map[string][]byte
	tokens    int
	status    int // a planted status for every object call; 0 means none
	noToken   bool
	tokenBody string
	server    *httptest.Server
	settings  Settings
}

func newBucket(t *testing.T) *bucket {
	t.Helper()
	b := &bucket{t: t, objects: map[string][]byte{}}
	b.server = httptest.NewServer(http.HandlerFunc(b.handle))
	t.Cleanup(b.server.Close)
	b.settings = Settings{
		Bucket:           "notes-bucket",
		Endpoint:         b.server.URL,
		TokenURL:         b.server.URL + "/token",
		Prefix:           "notes",
		TokenMaxBytes:    64 << 10,
		ObjectMaxBytes:   1 << 20,
		TokenRefreshSkew: time.Minute,
	}
	return b
}

func (b *bucket) tokenCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tokens
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
		if b.tokenBody != "" {
			_, _ = io.WriteString(w, b.tokenBody)
			return
		}
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

func mustPutObject(t *testing.T, store *Store, note ports.Note) {
	t.Helper()
	if err := store.Put(context.Background(), note); err != nil {
		t.Fatal(err)
	}
}

func TestContract(t *testing.T) {
	porttest.Run(t, func(t *testing.T) ports.Store { return newBucket(t).open(t) })
}

func seedForRestart(t *testing.T) (*Store, ports.Note) {
	t.Helper()
	b := newBucket(t)
	note := ports.Note{
		Owner:      "ana",
		ID:         "n1",
		Title:      "kept",
		Body:       "across a restart",
		CreateTime: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
	}
	mustPutObject(t, b.open(t), note)
	return b.open(t), note
}

// The hosted persistence statement (spec 012 FR-017): a note survives a restart of the service, because it is in
// the bucket; the second store reads what the first wrote, and another owner is still refused.
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

var objectPathAlphabet = []rune("abcABC012/\\..%~ \t\n\x00é中")

func randomObjectString(r *rand.Rand, maxLen int) string {
	n := r.Intn(maxLen + 1)
	var b strings.Builder
	for range n {
		b.WriteRune(objectPathAlphabet[r.Intn(len(objectPathAlphabet))])
	}
	return b.String()
}

type objectPathInput struct {
	prefix string
	owner  string
	id     string
}

func (objectPathInput) Generate(r *rand.Rand, _ int) reflect.Value {
	prefix := "notes"
	if r.Intn(2) == 1 {
		prefix = "notes/"
	}
	return reflect.ValueOf(objectPathInput{
		prefix: prefix,
		owner:  randomObjectString(r, 24),
		id:     randomObjectString(r, 24),
	})
}

func TestObjectNameInvariantsProperty(t *testing.T) {
	cases := []struct {
		name      string
		predicate func(got string) bool
	}{
		{
			name:      "starts with normalized prefix",
			predicate: func(got string) bool { return strings.HasPrefix(got, "notes/") },
		},
		{
			name:      "contains no parent reference",
			predicate: func(got string) bool { return !strings.Contains(got, "..") },
		},
		{
			name:      "has exactly two path separators",
			predicate: func(got string) bool { return strings.Count(got, "/") == 2 },
		},
		{
			name:      "ends with json extension",
			predicate: func(got string) bool { return strings.HasSuffix(got, ".json") },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			cfg := &quick.Config{Rand: rand.New(rand.NewSource(1))}
			property := func(in objectPathInput) bool {
				store := &Store{settings: Settings{Prefix: in.prefix}}
				return tc.predicate(store.name(in.owner, in.id))
			}

			// Act
			err := quick.Check(property, cfg)

			// Assert
			if err != nil {
				t.Fatalf("object name property %q failed: %v", tc.name, err)
			}
		})
	}
}

type admissibleObjectNote struct {
	note ports.Note
}

func (admissibleObjectNote) Generate(r *rand.Rand, _ int) reflect.Value {
	return reflect.ValueOf(admissibleObjectNote{
		note: ports.Note{
			Owner:      randomObjectString(r, 16),
			ID:         randomObjectString(r, 16),
			Title:      "t" + randomObjectString(r, 24),
			Body:       randomObjectString(r, 32),
			CreateTime: time.Unix(r.Int63n(2_000_000_000), 0).UTC(),
		},
	})
}

func TestPutAcceptsAdmissibleNoteProperty(t *testing.T) {
	// Arrange
	store := newBucket(t).open(t)
	ctx := context.Background()
	cfg := &quick.Config{Rand: rand.New(rand.NewSource(1))}
	property := func(sample admissibleObjectNote) bool {
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
	store := newBucket(t).open(t)
	ctx := context.Background()
	cfg := &quick.Config{Rand: rand.New(rand.NewSource(1))}
	property := func(sample admissibleObjectNote) bool {
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

func TestTheTokenIsCachedUntilItExpires(t *testing.T) {
	cases := []struct {
		name       string
		advance    time.Duration
		wantTokens int
	}{
		{"reused across calls before refresh boundary", 58 * time.Minute, 1},
		{"reused one second before refresh boundary", 59*time.Minute - time.Second, 1},
		{"refreshed at exact refresh boundary", 59 * time.Minute, 2},
		{"refreshed after full expiry", time.Hour, 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			b := newBucket(t)
			store := b.open(t)
			now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
			store.now = func() time.Time { return now }
			mustPutObject(t, store, ports.Note{Owner: "ana", ID: "n1"})
			now = now.Add(tc.advance)

			// Act
			_, _ = store.Get(context.Background(), "ana", "n1")
			got := b.tokenCount()

			// Assert
			if got != tc.wantTokens {
				t.Fatalf("%s: tokens fetched = %d; want %d", tc.name, got, tc.wantTokens)
			}
		})
	}
}

func TestReadTokenRejectsInvalidResponses(t *testing.T) {
	cases := []struct {
		name    string
		arrange func(b *bucket)
	}{
		{
			name:    "empty access token",
			arrange: func(b *bucket) { b.tokenBody = `{"access_token":"","expires_in":3600}` },
		},
		{
			name:    "zero expires_in",
			arrange: func(b *bucket) { b.tokenBody = `{"access_token":"issued-z","expires_in":0}` },
		},
		{
			name:    "negative expires_in",
			arrange: func(b *bucket) { b.tokenBody = `{"access_token":"issued-n","expires_in":-1}` },
		},
		{
			name:    "malformed token json",
			arrange: func(b *bucket) { b.tokenBody = "{not json" },
		},
		{
			name: "oversized token response",
			arrange: func(b *bucket) {
				b.settings.TokenMaxBytes = 48
				b.tokenBody = `{"access_token":"issued-oversized","expires_in":3600}`
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			b := newBucket(t)
			tc.arrange(b)
			store := b.open(t)

			// Act
			_, err := store.readToken(context.Background())

			// Assert
			if err == nil {
				t.Fatalf("%s: got nil error; want error", tc.name)
			}
		})
	}
}

func TestGetFailuresReturnUnavailable(t *testing.T) {
	cases := []struct {
		name    string
		arrange func(t *testing.T, b *bucket)
	}{
		{
			name:    "token endpoint refuses",
			arrange: func(_ *testing.T, b *bucket) { b.noToken = true },
		},
		{
			name:    "token response has empty access token",
			arrange: func(_ *testing.T, b *bucket) { b.tokenBody = `{"access_token":"","expires_in":3600}` },
		},
		{
			name:    "token response has zero expires_in",
			arrange: func(_ *testing.T, b *bucket) { b.tokenBody = `{"access_token":"issued-z","expires_in":0}` },
		},
		{
			name:    "token response has negative expires_in",
			arrange: func(_ *testing.T, b *bucket) { b.tokenBody = `{"access_token":"issued-n","expires_in":-1}` },
		},
		{
			name: "token response exceeds TokenMaxBytes",
			arrange: func(_ *testing.T, b *bucket) {
				b.settings.TokenMaxBytes = 48
				b.tokenBody = `{"access_token":"issued-oversized","expires_in":3600}`
			},
		},
		{
			name:    "bucket answers 500",
			arrange: func(_ *testing.T, b *bucket) { b.status = http.StatusInternalServerError },
		},
		{
			name:    "bucket answers 403",
			arrange: func(_ *testing.T, b *bucket) { b.status = http.StatusForbidden },
		},
		{
			name: "object is not valid json",
			arrange: func(t *testing.T, b *bucket) {
				b.objects[b.open(t).name("ana", "n1")] = []byte("not json")
			},
		},
		{
			name: "object belongs to another owner",
			arrange: func(t *testing.T, b *bucket) {
				b.objects[b.open(t).name("ana", "n1")] = []byte(`{"owner":"bob","id":"n1"}`)
			},
		},
		{
			name: "object has mismatched id",
			arrange: func(t *testing.T, b *bucket) {
				b.objects[b.open(t).name("ana", "n1")] = []byte(`{"owner":"ana","id":"n2"}`)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			b := newBucket(t)
			tc.arrange(t, b)
			store := b.open(t)

			// Act
			_, err := store.Get(context.Background(), "ana", "n1")

			// Assert
			if !errors.Is(err, ports.ErrUnavailable) {
				t.Fatalf("%s: got %v; want %v", tc.name, err, ports.ErrUnavailable)
			}
		})
	}
}

func TestPutFailuresReturnUnavailable(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T) (*Store, ports.Note)
	}{
		{
			name: "token endpoint refuses",
			setup: func(t *testing.T) (*Store, ports.Note) {
				b := newBucket(t)
				b.noToken = true
				return b.open(t), ports.Note{Owner: "ana", ID: "n1"}
			},
		},
		{
			name: "bucket answers 500",
			setup: func(t *testing.T) (*Store, ports.Note) {
				b := newBucket(t)
				b.status = http.StatusInternalServerError
				return b.open(t), ports.Note{Owner: "ana", ID: "n1"}
			},
		},
		{
			name: "bucket answers 403",
			setup: func(t *testing.T) (*Store, ports.Note) {
				b := newBucket(t)
				b.status = http.StatusForbidden
				return b.open(t), ports.Note{Owner: "ana", ID: "n1"}
			},
		},
		{
			name: "unreachable bucket",
			setup: func(t *testing.T) (*Store, ports.Note) {
				b := newBucket(t)
				b.server.Close()
				store, err := Open(b.settings, &http.Client{Timeout: time.Second})
				if err != nil {
					t.Fatal(err)
				}
				return store, ports.Note{Owner: "ana", ID: "n1"}
			},
		},
		{
			name: "out-of-range create time",
			setup: func(t *testing.T) (*Store, ports.Note) {
				return newBucket(t).open(t), ports.Note{
					Owner:      "ana",
					ID:         "n1",
					Title:      "bad year",
					CreateTime: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC),
				}
			},
		},
		{
			name: "note exceeds ObjectMaxBytes by one byte",
			setup: func(t *testing.T) (*Store, ports.Note) {
				b := newBucket(t)
				note := ports.Note{Owner: "ana", ID: "n1", Title: "oversized"}
				encoded, err := json.Marshal(record(note))
				if err != nil {
					t.Fatal(err)
				}
				b.settings.ObjectMaxBytes = int64(len(encoded) - 1)
				return b.open(t), note
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			store, note := tc.setup(t)

			// Act
			err := store.Put(context.Background(), note)

			// Assert
			if !errors.Is(err, ports.ErrUnavailable) {
				t.Fatalf("%s: got %v; want %v", tc.name, err, ports.ErrUnavailable)
			}
		})
	}
}

func TestExactPayloadByteLimitsAreAccepted(t *testing.T) {
	t.Run("put at exact ObjectMaxBytes", func(t *testing.T) {
		// Arrange
		b := newBucket(t)
		note := ports.Note{Owner: "ana", ID: "n1", Title: "exact"}
		encoded, err := json.Marshal(record(note))
		if err != nil {
			t.Fatal(err)
		}
		b.settings.ObjectMaxBytes = int64(len(encoded))
		store := b.open(t)

		// Act
		err = store.Put(context.Background(), note)

		// Assert
		if err != nil {
			t.Fatalf("put at exact ObjectMaxBytes: got %v; want nil", err)
		}
	})

	t.Run("get at exact ObjectMaxBytes", func(t *testing.T) {
		// Arrange
		b := newBucket(t)
		note := ports.Note{Owner: "ana", ID: "n1", Title: "exact"}
		encoded, err := json.Marshal(record(note))
		if err != nil {
			t.Fatal(err)
		}
		b.settings.ObjectMaxBytes = int64(len(encoded))
		store := b.open(t)
		b.objects[store.name("ana", "n1")] = encoded

		// Act
		_, err = store.Get(context.Background(), "ana", "n1")

		// Assert
		if err != nil {
			t.Fatalf("get at exact ObjectMaxBytes: got %v; want nil", err)
		}
	})

	t.Run("token response at exact TokenMaxBytes", func(t *testing.T) {
		// Arrange
		b := newBucket(t)
		b.tokenBody = `{"access_token":"issued-exact","expires_in":3600}`
		b.settings.TokenMaxBytes = int64(len(b.tokenBody))
		store := b.open(t)

		// Act
		err := store.Put(context.Background(), ports.Note{Owner: "ana", ID: "n1", Title: "t"})

		// Assert
		if err != nil {
			t.Fatalf("token at exact TokenMaxBytes: got %v; want nil", err)
		}
	})
}

func TestAnEndedContextIsReportedAsSuch(t *testing.T) {
	cases := []struct {
		name string
		act  func(ctx context.Context, store *Store) error
	}{
		{
			name: "put",
			act: func(ctx context.Context, store *Store) error {
				return store.Put(ctx, ports.Note{Owner: "ana", ID: "n1"})
			},
		},
		{
			name: "get",
			act: func(ctx context.Context, store *Store) error {
				_, err := store.Get(ctx, "ana", "n1")
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			store := newBucket(t).open(t)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			// Act
			err := tc.act(ctx, store)

			// Assert
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%s with canceled context: got %v; want %v", tc.name, err, context.Canceled)
			}
		})
	}
}

func TestSettingsAreCheckedByName(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Settings)
		want   string
	}{
		{"no bucket", func(s *Settings) { s.Bucket = "" }, "store.bucket: required"},
		{"bad endpoint scheme", func(s *Settings) { s.Endpoint = "ftp://x" }, "store.endpoint"},
		{"endpoint missing host", func(s *Settings) { s.Endpoint = "https://" }, "store.endpoint"},
		{"invalid endpoint url", func(s *Settings) { s.Endpoint = "http://[::1" }, "store.endpoint"},
		{"bad token url", func(s *Settings) { s.TokenURL = "metadata" }, "store.token_url"},
		{"token url missing host", func(s *Settings) { s.TokenURL = "http://" }, "store.token_url"},
		{"no prefix", func(s *Settings) { s.Prefix = "" }, "store.prefix"},
		{"parent prefix", func(s *Settings) { s.Prefix = "../notes" }, "store.prefix"},
		{"token max bytes below min", func(s *Settings) { s.TokenMaxBytes = 0 }, "payload bounds"},
		{"token max bytes above max", func(s *Settings) { s.TokenMaxBytes = (1 << 20) + 1 }, "payload bounds"},
		{"object max bytes below min", func(s *Settings) { s.ObjectMaxBytes = 0 }, "payload bounds"},
		{"object max bytes above max", func(s *Settings) { s.ObjectMaxBytes = (16 << 20) + 1 }, "payload bounds"},
		{"negative token refresh skew", func(s *Settings) { s.TokenRefreshSkew = -time.Nanosecond }, "token refresh skew"},
		{"token refresh skew above max", func(s *Settings) { s.TokenRefreshSkew = time.Hour + time.Nanosecond }, "token refresh skew"},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/returns error", func(t *testing.T) {
			// Arrange
			b := newBucket(t)
			settings := b.settings
			tc.change(&settings)

			// Act
			_, err := Open(settings, b.server.Client())

			// Assert
			if err == nil {
				t.Fatalf("%s: got nil error; want error containing %q", tc.name, tc.want)
			}
		})

		t.Run(tc.name+"/names offending setting", func(t *testing.T) {
			// Arrange
			b := newBucket(t)
			settings := b.settings
			tc.change(&settings)

			// Act
			_, err := Open(settings, b.server.Client())
			msg := ""
			if err != nil {
				msg = err.Error()
			}

			// Assert
			if !strings.Contains(msg, tc.want) {
				t.Fatalf("%s: error=%q; want substring %q", tc.name, msg, tc.want)
			}
		})
	}
}

func TestOpenRefusesNilClient(t *testing.T) {
	// Arrange
	b := newBucket(t)

	// Act
	_, err := Open(b.settings, nil)

	// Assert
	if err == nil {
		t.Fatal("Open with nil client: got nil error; want error")
	}
}

func TestSettingsBoundaryValuesAccepted(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Settings)
	}{
		{"token max bytes at lower bound", func(s *Settings) { s.TokenMaxBytes = 1 }},
		{"token max bytes at upper bound", func(s *Settings) { s.TokenMaxBytes = 1 << 20 }},
		{"object max bytes at lower bound", func(s *Settings) { s.ObjectMaxBytes = 1 }},
		{"object max bytes at upper bound", func(s *Settings) { s.ObjectMaxBytes = 16 << 20 }},
		{"token refresh skew at zero", func(s *Settings) { s.TokenRefreshSkew = 0 }},
		{"token refresh skew at upper bound", func(s *Settings) { s.TokenRefreshSkew = time.Hour }},
		{"https endpoint and token url", func(s *Settings) {
			s.Endpoint = "https://storage.example.test"
			s.TokenURL = "https://metadata.example.test/token"
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			b := newBucket(t)
			settings := b.settings
			tc.change(&settings)

			// Act
			_, err := Open(settings, b.server.Client())

			// Assert
			if err != nil {
				t.Fatalf("%s: got %v; want nil", tc.name, err)
			}
		})
	}
}
