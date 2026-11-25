package object

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/bfftemplate/bff/ports"
)

func regressionStore(t *testing.T, handler http.Handler) (*Store, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	settings := newBucket(t).settings
	settings.Endpoint, settings.TokenURL = srv.URL, srv.URL+"/token"
	store, err := Open(settings, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return store, srv
}

func awaitPhaseAndCancel(phase string, startWait, stopWait time.Duration, started <-chan struct{}, done <-chan error, cancel context.CancelFunc) (error, error) {
	select {
	case <-started:
	case err := <-done:
		return nil, fmt.Errorf("returned before %s started: %w", phase, err)
	case <-time.After(startWait):
		return nil, fmt.Errorf("timed out waiting for %s to start", phase)
	}
	cancel()
	select {
	case err := <-done:
		return err, nil
	case <-time.After(stopWait):
		return nil, fmt.Errorf("timed out waiting for %s to stop after cancel", phase)
	}
}

func startWitnessWorker(t *testing.T, phase string, callCtx context.Context, release chan struct{}, run func(context.Context) error) <-chan error {
	t.Helper()
	workerCtx, cancelWorker := context.WithCancel(callCtx)
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		done <- run(workerCtx)
	}()
	t.Cleanup(func() {
		cancelWorker()
		close(release)
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Errorf("timed out waiting for %s worker cleanup", phase)
		}
	})
	return done
}

func startTokenFetchWitness(t *testing.T, callCtx context.Context) (<-chan struct{}, <-chan error) {
	t.Helper()
	started, release := make(chan struct{}), make(chan struct{})
	var markOnce sync.Once
	store, _ := regressionStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		markOnce.Do(func() { close(started) })
		<-release
	}))
	done := startWitnessWorker(t, "token fetch", callCtx, release, func(ctx context.Context) error {
		return store.Put(ctx, ports.Note{})
	})
	return started, done
}

func startObjectRequestWitness(t *testing.T, callCtx context.Context) (<-chan struct{}, <-chan error) {
	t.Helper()
	started, release := make(chan struct{}), make(chan struct{})
	var markOnce sync.Once
	store, _ := regressionStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/upload/storage/v1/b/notes-bucket/o" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		markOnce.Do(func() { close(started) })
		<-release
	}))
	store.token = "cached-token"
	store.expires = time.Now().Add(time.Hour)
	done := startWitnessWorker(t, "object request", callCtx, release, func(ctx context.Context) error {
		return store.Put(ctx, ports.Note{Owner: "ana", ID: "n1", Title: "t"})
	})
	return started, done
}

func startBodyReadWitness(t *testing.T, callCtx context.Context) (<-chan struct{}, <-chan error) {
	t.Helper()
	store := newBucket(t).open(t)
	store.token = "test"
	store.expires = time.Now().Add(time.Hour)
	started, release := make(chan struct{}), make(chan struct{})
	wantPath := "/storage/v1/b/notes-bucket/o/" + store.name("ana", "n1")
	store.client.Transport = regressionTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != wantPath {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       &canceledBody{ctx: r.Context(), started: started, release: release},
			Header:     make(http.Header),
		}, nil
	})
	done := startWitnessWorker(t, "body read", callCtx, release, func(ctx context.Context) error {
		_, err := store.Get(ctx, "ana", "n1")
		return err
	})
	return started, done
}

func TestCancellationDuringTokenFetch(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Act
	started, done := startTokenFetchWitness(t, ctx)
	err, witnessErr := awaitPhaseAndCancel("token fetch", time.Second, time.Second, started, done, cancel)
	if witnessErr != nil {
		t.Fatal(witnessErr)
	}

	// Assert
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation during token fetch: got %v; want %v", err, context.Canceled)
	}
}

func TestCancellationDuringObjectRequest(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Act
	started, done := startObjectRequestWitness(t, ctx)
	err, witnessErr := awaitPhaseAndCancel("object request", time.Second, time.Second, started, done, cancel)
	if witnessErr != nil {
		t.Fatal(witnessErr)
	}

	// Assert
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation during object request: got %v; want %v", err, context.Canceled)
	}
}

func TestDetachedContextCancellationWitnessFailsPromptly(t *testing.T) {
	cases := []struct {
		phase string
		start func(t *testing.T, callCtx context.Context) (<-chan struct{}, <-chan error)
	}{
		{"token fetch", startTokenFetchWitness},
		{"object request", startObjectRequestWitness},
		{"body read", startBodyReadWitness},
	}

	for _, tc := range cases {
		t.Run(tc.phase, func(t *testing.T) {
			// Arrange
			_, unrelatedCancel := context.WithCancel(context.Background())
			t.Cleanup(unrelatedCancel)
			want := "timed out waiting for " + tc.phase + " to stop after cancel"

			// Act
			started, done := tc.start(t, context.Background())
			_, witnessErr := awaitPhaseAndCancel(tc.phase, time.Second, 50*time.Millisecond, started, done, unrelatedCancel)
			got := ""
			if witnessErr != nil {
				got = witnessErr.Error()
			}

			// Assert
			if !strings.Contains(got, want) {
				t.Fatalf("detached %s witness: got %q; want substring %q", tc.phase, got, want)
			}
		})
	}
}

func TestStartupTimeoutCancelsAndDrainsBlockedWorker(t *testing.T) {
	// Arrange
	store := newBucket(t).open(t)
	store.gate <- struct{}{}
	started, release := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	want := "timed out waiting for blocked gate to start"

	// Act
	done := startWitnessWorker(t, "blocked gate", ctx, release, func(wctx context.Context) error {
		return store.Put(wctx, ports.Note{Owner: "ana", ID: "n1", Title: "t"})
	})
	_, witnessErr := awaitPhaseAndCancel("blocked gate", 20*time.Millisecond, 50*time.Millisecond, started, done, cancel)
	got := ""
	if witnessErr != nil {
		got = witnessErr.Error()
	}

	// Assert
	if !strings.Contains(got, want) {
		t.Fatalf("blocked-before-phase witness: got %q; want substring %q", got, want)
	}
}

func TestCanceledCallerDoesNotWaitForAnotherTokenFetch(t *testing.T) {
	// Arrange
	started, release := make(chan struct{}), make(chan struct{})
	var markOnce sync.Once
	store, _ := regressionStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/token" {
			markOnce.Do(func() { close(started) })
			<-release
			_, _ = io.WriteString(w, `{"access_token":"test","expires_in":3600}`)
		}
	}))
	first := startWitnessWorker(t, "first token fetch", context.Background(), release, func(ctx context.Context) error {
		return store.Put(ctx, ports.Note{})
	})
	select {
	case <-started:
	case err := <-first:
		t.Fatalf("first caller returned before token fetch started: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first caller token fetch to start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	t.Cleanup(cancel)

	// Act
	second := make(chan error, 1)
	go func() { second <- store.Put(ctx, ports.Note{}) }()
	var err error
	select {
	case err = <-second:
	case <-time.After(time.Second):
		err = errors.New("caller waited for another request's token fetch")
	}

	// Assert
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second caller while token fetch in flight: got %v; want %v", err, context.DeadlineExceeded)
	}
}

func TestRejectsTrailingAndOversizedObjectDocuments(t *testing.T) {
	cases := []struct {
		name string
		tail string
	}{
		{"trailing garbage", "garbage"},
		{"oversized padding", strings.Repeat(" ", 1<<20)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			b := newBucket(t)
			s := b.open(t)
			b.objects[s.name("ana", "n1")] = []byte(`{"owner":"ana","id":"n1"}` + tc.tail)

			// Act
			_, err := s.Get(context.Background(), "ana", "n1")

			// Assert
			if !errors.Is(err, ports.ErrUnavailable) {
				t.Fatalf("%s: got %v; want %v", tc.name, err, ports.ErrUnavailable)
			}
		})
	}
}

func setupRedirectStore(t *testing.T, tokenRedirect bool) (*Store, <-chan struct{}) {
	t.Helper()
	received := make(chan struct{}, 1)
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
	}))
	t.Cleanup(sink.Close)
	store, _ := regressionStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" && !tokenRedirect {
			_, _ = io.WriteString(w, `{"access_token":"private","expires_in":3600}`)
			return
		}
		http.Redirect(w, r, sink.URL, http.StatusTemporaryRedirect)
	}))
	return store, received
}

func TestTokenAndObjectRedirectsAreRefused(t *testing.T) {
	cases := []struct {
		name          string
		tokenRedirect bool
	}{
		{"object", false},
		{"token", true},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/returns unavailable", func(t *testing.T) {
			// Arrange
			store, _ := setupRedirectStore(t, tc.tokenRedirect)

			// Act
			err := store.Put(context.Background(), ports.Note{})

			// Assert
			if !errors.Is(err, ports.ErrUnavailable) {
				t.Fatalf("%s redirect: got %v; want %v", tc.name, err, ports.ErrUnavailable)
			}
		})

		t.Run(tc.name+"/redirect target is not contacted", func(t *testing.T) {
			// Arrange
			store, received := setupRedirectStore(t, tc.tokenRedirect)

			// Act
			_ = store.Put(context.Background(), ports.Note{})
			contacted := false
			select {
			case <-received:
				contacted = true
			default:
			}

			// Assert
			if contacted {
				t.Fatalf("%s redirect target was contacted", tc.name)
			}
		})
	}
}

type regressionTransport func(*http.Request) (*http.Response, error)

func (f regressionTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type canceledBody struct {
	ctx     context.Context
	started chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (b *canceledBody) Read([]byte) (int, error) {
	b.once.Do(func() { close(b.started) })
	select {
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	case <-b.release:
		return 0, io.EOF
	}
}

func (*canceledBody) Close() error { return nil }

func TestCancellationWhileReadingObjectBody(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Act
	started, done := startBodyReadWitness(t, ctx)
	err, witnessErr := awaitPhaseAndCancel("body read", time.Second, time.Second, started, done, cancel)
	if witnessErr != nil {
		t.Fatal(witnessErr)
	}

	// Assert
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("body cancellation: got %v; want %v", err, context.Canceled)
	}
}
