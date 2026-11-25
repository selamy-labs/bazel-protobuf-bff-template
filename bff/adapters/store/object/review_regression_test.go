package object

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func awaitPhaseAndCancel(t *testing.T, phase string, started <-chan struct{}, done <-chan error, cancel context.CancelFunc) error {
	t.Helper()
	select {
	case <-started:
		cancel()
		return <-done
	case err := <-done:
		t.Fatalf("returned before %s started: %v", phase, err)
		return nil
	}
}

func TestCancellationDuringTokenFetch(t *testing.T) {
	// Arrange
	started, release := make(chan struct{}), make(chan struct{})
	store, _ := regressionStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
	}))
	t.Cleanup(func() { close(release) })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Act
	done := make(chan error, 1)
	go func() { done <- store.Put(ctx, ports.Note{}) }()
	err := awaitPhaseAndCancel(t, "token fetch", started, done, cancel)

	// Assert
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation during token fetch: got %v; want %v", err, context.Canceled)
	}
}

func TestCancellationDuringObjectRequest(t *testing.T) {
	// Arrange
	started, release := make(chan struct{}), make(chan struct{})
	store, _ := regressionStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
	}))
	store.token = "cached-token"
	store.expires = time.Now().Add(time.Hour)
	t.Cleanup(func() { close(release) })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Act
	done := make(chan error, 1)
	go func() { done <- store.Put(ctx, ports.Note{Owner: "ana", ID: "n1", Title: "t"}) }()
	err := awaitPhaseAndCancel(t, "object request", started, done, cancel)

	// Assert
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation during object request: got %v; want %v", err, context.Canceled)
	}
}

func TestCanceledCallerDoesNotWaitForAnotherTokenFetch(t *testing.T) {
	// Arrange
	started, release := make(chan struct{}), make(chan struct{})
	store, _ := regressionStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			select {
			case <-started:
			default:
				close(started)
			}
			<-release
			_, _ = io.WriteString(w, `{"access_token":"test","expires_in":3600}`)
		}
	}))
	first := make(chan error, 1)
	go func() { first <- store.Put(context.Background(), ports.Note{}) }()
	select {
	case <-started:
		t.Cleanup(func() {
			close(release)
			<-first
		})
	case err := <-first:
		close(release)
		t.Fatalf("first caller returned before token fetch started: %v", err)
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
}

func (b *canceledBody) Read([]byte) (int, error) {
	close(b.started)
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}

func (*canceledBody) Close() error { return nil }

func TestCancellationWhileReadingObjectBody(t *testing.T) {
	// Arrange
	s := newBucket(t).open(t)
	s.token = "test"
	s.expires = time.Now().Add(time.Hour)
	started := make(chan struct{})
	s.client.Transport = regressionTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       &canceledBody{ctx: r.Context(), started: started},
			Header:     make(http.Header),
		}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Act
	done := make(chan error, 1)
	go func() {
		_, err := s.Get(ctx, "ana", "n1")
		done <- err
	}()
	err := awaitPhaseAndCancel(t, "body read", started, done, cancel)

	// Assert
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("body cancellation: got %v; want %v", err, context.Canceled)
	}
}
