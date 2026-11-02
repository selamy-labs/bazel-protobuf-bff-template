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

func TestCancellationDuringTokenFetch(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	store, _ := regressionStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-release }))
	defer close(release)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- store.Put(ctx, ports.Note{}) }()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("want canceled, got %v", err)
	}
}

func TestCanceledCallerDoesNotWaitForAnotherTokenFetch(t *testing.T) {
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
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	second := make(chan error, 1)
	go func() { second <- store.Put(ctx, ports.Note{}) }()
	defer func() { close(release); <-first }()
	select {
	case err := <-second:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("want deadline, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("caller waited for another request's token fetch")
	}
}

func TestRejectsTrailingAndOversizedObjectDocuments(t *testing.T) {
	b := newBucket(t)
	s := b.open(t)
	for _, tail := range []string{"garbage", strings.Repeat(" ", 1<<20)} {
		b.objects[s.name("ana", "n1")] = []byte(`{"owner":"ana","id":"n1"}` + tail)
		if _, err := s.Get(context.Background(), "ana", "n1"); !errors.Is(err, ports.ErrUnavailable) {
			t.Fatalf("accepted corrupt/oversized object: %v", err)
		}
	}
}

func TestTokenAndObjectRedirectsAreRefused(t *testing.T) {
	for _, tokenRedirect := range []bool{false, true} {
		t.Run(map[bool]string{false: "object", true: "token"}[tokenRedirect], func(t *testing.T) {
			received := make(chan struct{}, 1)
			sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received <- struct{}{} }))
			defer sink.Close()
			s, _ := regressionStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" && !tokenRedirect {
					_, _ = io.WriteString(w, `{"access_token":"private","expires_in":3600}`)
					return
				}
				http.Redirect(w, r, sink.URL, http.StatusTemporaryRedirect)
			}))
			if err := s.Put(context.Background(), ports.Note{}); !errors.Is(err, ports.ErrUnavailable) {
				t.Fatalf("redirect not refused: %v", err)
			}
			select {
			case <-received:
				t.Fatal("redirect target was contacted")
			default:
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
	s := newBucket(t).open(t)
	s.token = "test"
	s.expires = time.Now().Add(time.Hour)
	started := make(chan struct{})
	s.client.Transport = regressionTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: &canceledBody{ctx: r.Context(), started: started}, Header: make(http.Header)}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := s.Get(ctx, "ana", "n1"); done <- err }()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("body cancellation: %v", err)
	}
}
