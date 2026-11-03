package transport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	validatepb "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	connect "connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	"connectrpc.com/connect/v2/connectproto"

	notesv1 "example.com/bfftemplate/api/template/notes/v1"
	"example.com/bfftemplate/api/template/notes/v1/notesv1connect"
	"example.com/bfftemplate/bff/adapters/memory"
	"example.com/bfftemplate/bff/app"
	"example.com/bfftemplate/bff/identity"
	"example.com/bfftemplate/bff/ports"
	"example.com/bfftemplate/bff/testing/faults"
	testidentity "example.com/bfftemplate/bff/testing/identity"
	"example.com/bfftemplate/bff/transport"
)

const origin = "https://app.example.com"

type harness struct {
	url    string
	client notesv1connect.NoteServiceClient
	http   *http.Client
	log    *syncBuffer
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// The bounds these tests declare; the handler has none of its own.
var limits = transport.Limits{DefaultDeadline: 2 * time.Second, MaxDeadline: 10 * time.Second, MaxRequestBytes: 64 << 10}

func start(t *testing.T, store ports.Store, allowTestIdentity bool) harness {
	return startWith(t, store, allowTestIdentity, limits)
}

func startWith(t *testing.T, store ports.Store, allowTestIdentity bool, bounds transport.Limits) harness {
	t.Helper()
	log := &syncBuffer{}
	server := httptest.NewServer(transport.NewHandler(transport.Options{
		Notes:             app.New(store),
		Authenticator:     testidentity.Authenticator{},
		AllowTestIdentity: allowTestIdentity,
		AllowedOrigin:     origin,
		Logger:            slog.New(slog.NewJSONHandler(log, nil)),
		Limits:            bounds,
	}))
	t.Cleanup(server.Close)
	client := notesv1connect.NewNoteServiceClient(connect.NewClient(connecthttp.NewTransport(server.Client(), server.URL)))
	return harness{server.URL, client, server.Client(), log}
}

func as(ctx context.Context, subject string) context.Context {
	ctx, info := connect.NewClientContext(ctx)
	info.RequestHeader().Set("Authorization", testidentity.Header(subject))
	return ctx
}

func requireCode(t *testing.T, err error, want connect.Code) *connect.Error {
	t.Helper()
	var public *connect.Error
	if !errors.As(err, &public) {
		t.Fatalf("want %v, got %v", want, err)
	}
	if public.Code() != want {
		t.Fatalf("want %v, got %v: %v", want, public.Code(), err)
	}
	return public
}

func TestCreateThenRead(t *testing.T) {
	h := start(t, memory.New(100), true)
	ctx := context.Background()
	created, err := h.client.CreateNote(as(ctx, "ana"), &notesv1.CreateNoteRequest{Note: &notesv1.Note{
		Name:  "notes/chosen-by-the-client",
		Title: "  a title ",
		Body:  "a body",
	}})
	if err != nil {
		t.Fatal(err)
	}
	note := created.GetNote()
	if note.GetName() == "notes/chosen-by-the-client" || !strings.HasPrefix(note.GetName(), "notes/") {
		t.Fatalf("the server must assign the name, got %q", note.GetName())
	}
	if note.GetTitle() != "a title" || note.GetCreateTime() == nil {
		t.Fatalf("unexpected note %+v", note)
	}
	read, err := h.client.GetNote(as(ctx, "ana"), &notesv1.GetNoteRequest{Name: note.GetName()})
	if err != nil {
		t.Fatal(err)
	}
	if read.GetNote().GetBody() != "a body" || read.GetNote().GetName() != note.GetName() {
		t.Fatalf("unexpected note %+v", read.GetNote())
	}
}

func TestInvalidInputNamesTheFieldAndSkipsStorage(t *testing.T) {
	h := start(t, panicStore{}, true)
	cases := []struct {
		name  string
		call  func(context.Context) error
		field string
		rule  string
	}{
		{"empty title", func(ctx context.Context) error {
			_, err := h.client.CreateNote(ctx, &notesv1.CreateNoteRequest{Note: &notesv1.Note{Title: " "}})
			return err
		}, "note.title", "note.title.required"},
		{"long title", func(ctx context.Context) error {
			_, err := h.client.CreateNote(ctx, &notesv1.CreateNoteRequest{Note: &notesv1.Note{Title: strings.Repeat("a", 121)}})
			return err
		}, "note.title", "note.title.max_len"},
		{"long body", func(ctx context.Context) error {
			_, err := h.client.CreateNote(ctx, &notesv1.CreateNoteRequest{Note: &notesv1.Note{Title: "a", Body: strings.Repeat("b", 4001)}})
			return err
		}, "note.body", "string.max_len"},
		{"bad name", func(ctx context.Context) error {
			_, err := h.client.GetNote(ctx, &notesv1.GetNoteRequest{Name: "tasks/1"})
			return err
		}, "name", "string.pattern"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			public := requireCode(t, c.call(as(context.Background(), "ana")), connect.CodeInvalidArgument)
			if len(public.Details()) != 1 {
				t.Fatalf("want one detail, got %d", len(public.Details()))
			}
			message, err := connectproto.UnmarshalErrorDetail(public.Details()[0])
			if err != nil {
				t.Fatal(err)
			}
			violations, ok := message.(*validatepb.Violations)
			if !ok || len(violations.GetViolations()) != 1 {
				t.Fatalf("unexpected detail %v", message)
			}
			violation := violations.GetViolations()[0]
			if violation.GetRuleId() != c.rule {
				t.Fatalf("want rule %s, got %s", c.rule, violation.GetRuleId())
			}
			var path []string
			for _, element := range violation.GetField().GetElements() {
				path = append(path, element.GetFieldName())
			}
			if strings.Join(path, ".") != c.field {
				t.Fatalf("want field %s, got %v", c.field, path)
			}
		})
	}
}

// panicStore proves a code path never reaches storage: a call would surface as internal.
type panicStore struct{}

func (panicStore) Put(context.Context, ports.Note) error { panic("storage was called") }

func (panicStore) Get(context.Context, string, string) (ports.Note, error) {
	panic("storage was called")
}

func post(t *testing.T, h harness, method string, body string, header http.Header) (int, []byte) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, h.url+"/"+notesv1connect.NoteServiceName+"/"+method, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	for key, values := range header {
		request.Header[key] = values
	}
	response, err := h.http.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, data
}

func TestForeignAndAbsentNotesAreIndistinguishable(t *testing.T) {
	h := start(t, memory.New(100), true)
	ctx := context.Background()
	created, err := h.client.CreateNote(as(ctx, "ana"), &notesv1.CreateNoteRequest{Note: &notesv1.Note{Title: "private"}})
	if err != nil {
		t.Fatal(err)
	}
	foreign := created.GetNote().GetName()
	absent := "notes/" + strings.Repeat("0", len(strings.TrimPrefix(foreign, "notes/")))
	bob := http.Header{"Authorization": {testidentity.Header("bob")}}

	foreignStatus, foreignBody := post(t, h, "GetNote", `{"name":"`+foreign+`"}`, bob)
	absentStatus, absentBody := post(t, h, "GetNote", `{"name":"`+absent+`"}`, bob)
	if foreignStatus != absentStatus || !bytes.Equal(foreignBody, absentBody) {
		t.Fatalf("responses differ: %d %s, %d %s", foreignStatus, foreignBody, absentStatus, absentBody)
	}
	_, err = h.client.GetNote(as(ctx, "bob"), &notesv1.GetNoteRequest{Name: foreign})
	requireCode(t, err, connect.CodeNotFound)
	if _, err := h.client.GetNote(as(ctx, "ana"), &notesv1.GetNoteRequest{Name: foreign}); err != nil {
		t.Fatalf("the owner lost access: %v", err)
	}
}

func TestStorageFailuresMapToContractCodes(t *testing.T) {
	h := start(t, faults.Store{Next: memory.New(100)}, true)
	ctx := context.Background()
	_, err := h.client.CreateNote(as(ctx, faults.Unavailable), &notesv1.CreateNoteRequest{Note: &notesv1.Note{Title: "a"}})
	requireCode(t, err, connect.CodeUnavailable)
	_, err = h.client.GetNote(as(ctx, faults.Unavailable), &notesv1.GetNoteRequest{Name: "notes/a"})
	requireCode(t, err, connect.CodeUnavailable)
}

func TestInternalErrorsLeakOnlyACorrelationID(t *testing.T) {
	for _, subject := range []string{faults.Secret, faults.Panic} {
		t.Run(subject, func(t *testing.T) {
			h := start(t, faults.Store{Next: memory.New(100)}, true)
			status, body := post(t, h, "GetNote", `{"name":"notes/a"}`, http.Header{"Authorization": {testidentity.Header(subject)}})
			if status != http.StatusInternalServerError {
				t.Fatalf("status %d: %s", status, body)
			}
			if bytes.Contains(body, []byte(faults.PlantedSecret)) {
				t.Fatalf("the response leaked: %s", body)
			}
			var parsed struct {
				Code    string            `json:"code"`
				Message string            `json:"message"`
				Details []json.RawMessage `json:"details"`
			}
			if err := json.Unmarshal(body, &parsed); err != nil {
				t.Fatal(err)
			}
			id, found := strings.CutPrefix(parsed.Message, "internal error, correlation id ")
			if !found || parsed.Code != "internal" || len(parsed.Details) != 0 {
				t.Fatalf("unexpected body %s", body)
			}
			var entry struct {
				CorrelationID string `json:"correlation_id"`
				RPCStatusCode string `json:"rpc.status_code"`
				Error         string `json:"error"`
			}
			if err := json.Unmarshal([]byte(h.log.String()), &entry); err != nil {
				t.Fatalf("log %q: %v", h.log.String(), err)
			}
			if entry.CorrelationID != id || entry.RPCStatusCode != "INTERNAL" || !strings.Contains(entry.Error, faults.PlantedSecret) {
				t.Fatalf("the log does not hold the detail under %s: %q", id, h.log.String())
			}
			// The server keeps serving after a panic.
			_, err := h.client.CreateNote(as(context.Background(), "ana"), &notesv1.CreateNoteRequest{Note: &notesv1.Note{Title: "a"}})
			if err != nil {
				t.Fatalf("the server stopped serving: %v", err)
			}
		})
	}
}

// A downstream dependency cannot label its own failure as safe. plant returns a Connect error with a
// public-looking code that carries private text in its message and in a detail.
func plant(t *testing.T, code connect.Code) *connect.Error {
	t.Helper()
	detail, err := connectproto.NewErrorDetail(&notesv1.Note{Title: faults.PlantedSecret})
	if err != nil {
		t.Fatal(err)
	}
	return connect.NewError(code, faults.PlantedSecret).WithDetail(detail)
}

type failingStore struct{ err error }

func (s failingStore) Put(context.Context, ports.Note) error { return s.err }

func (s failingStore) Get(context.Context, string, string) (ports.Note, error) {
	return ports.Note{}, s.err
}

type failingAuthenticator struct{ err error }

func (failingAuthenticator) Kind() identity.Kind { return identity.KindVerified }

func (a failingAuthenticator) Authenticate(context.Context, string) (identity.Principal, error) {
	return identity.Principal{}, a.err
}

func serve(t *testing.T, store ports.Store, authenticator identity.Authenticator, allowTestIdentity bool) harness {
	t.Helper()
	log := &syncBuffer{}
	server := httptest.NewServer(transport.NewHandler(transport.Options{
		Notes:             app.New(store),
		Authenticator:     authenticator,
		AllowTestIdentity: allowTestIdentity,
		AllowedOrigin:     origin,
		Logger:            slog.New(slog.NewJSONHandler(log, nil)),
		Limits:            limits,
	}))
	t.Cleanup(server.Close)
	client := notesv1connect.NewNoteServiceClient(connect.NewClient(connecthttp.NewTransport(server.Client(), server.URL)))
	return harness{server.URL, client, server.Client(), log}
}

func TestDownstreamCannotLabelItsOwnErrorPublic(t *testing.T) {
	codes := []connect.Code{
		connect.CodeInvalidArgument,
		connect.CodeNotFound,
		connect.CodeUnauthenticated,
		connect.CodeUnavailable,
		connect.CodeFailedPrecondition,
		connect.CodePermissionDenied,
		connect.CodeAlreadyExists,
		connect.CodeDataLoss,
	}
	shapes := map[string]func(*connect.Error) error{
		"direct":  func(e *connect.Error) error { return e },
		"wrapped": func(e *connect.Error) error { return fmt.Errorf("while reading: %w", e) },
		"joined":  func(e *connect.Error) error { return errors.Join(errors.New("while reading"), e) },
	}
	boundaries := map[string]func(*testing.T, error) harness{
		"store": func(t *testing.T, err error) harness {
			return serve(t, failingStore{err}, testidentity.Authenticator{}, true)
		},
		"authenticator": func(t *testing.T, err error) harness {
			return serve(t, memory.New(100), failingAuthenticator{err}, false)
		},
	}
	credential := http.Header{"Authorization": {testidentity.Header("ana")}}
	for boundary, build := range boundaries {
		for shape, wrap := range shapes {
			for _, code := range codes {
				t.Run(boundary+"/"+shape+"/"+code.String(), func(t *testing.T) {
					h := build(t, wrap(plant(t, code)))
					status, body := post(t, h, "GetNote", `{"name":"notes/a"}`, credential)
					if status != http.StatusInternalServerError || !bytes.Contains(body, []byte(`"internal"`)) {
						t.Fatalf("status %d: %s", status, body)
					}
					if bytes.Contains(body, []byte(faults.PlantedSecret)) || bytes.Contains(body, []byte("details")) {
						t.Fatalf("the response leaked: %s", body)
					}
					if !strings.Contains(h.log.String(), faults.PlantedSecret) {
						t.Fatalf("the log lost the detail: %q", h.log.String())
					}
				})
			}
		}
	}
}

// Sentinels keep their contract meaning across the boundary, with constant text, even when a
// downstream error carrying private text wraps them.
func TestDownstreamSentinelsMapToConstantText(t *testing.T) {
	cases := []struct {
		name     string
		sentinel error
		status   int
		code     string
	}{
		{"not found", ports.ErrNotFound, http.StatusNotFound, `"not_found"`},
		{"unavailable", ports.ErrUnavailable, http.StatusServiceUnavailable, `"unavailable"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := serve(t, failingStore{fmt.Errorf("%s: %w", faults.PlantedSecret, c.sentinel)}, testidentity.Authenticator{}, true)
			status, body := post(t, h, "GetNote", `{"name":"notes/a"}`, http.Header{"Authorization": {testidentity.Header("ana")}})
			if status != c.status || !bytes.Contains(body, []byte(c.code)) || bytes.Contains(body, []byte(faults.PlantedSecret)) {
				t.Fatalf("status %d: %s", status, body)
			}
		})
	}
}

// The framework's own decode and size errors stay public: they say only what the caller sent.
func TestFrameworkErrorsStayPublic(t *testing.T) {
	h := start(t, panicStore{}, true)
	status, body := post(t, h, "GetNote", `not json`, http.Header{"Authorization": {testidentity.Header("ana")}})
	if status != http.StatusBadRequest || !bytes.Contains(body, []byte(`"invalid_argument"`)) {
		t.Fatalf("status %d: %s", status, body)
	}
}

// watchingStore reports the context each call received, and blocks until it ends.
type watchingStore struct {
	deadline chan time.Duration
	ended    chan error
}

func newWatchingStore() watchingStore {
	return watchingStore{make(chan time.Duration, 1), make(chan error, 1)}
}

func (s watchingStore) Put(ctx context.Context, _ ports.Note) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		s.deadline <- -1
	} else {
		s.deadline <- time.Until(deadline)
	}
	<-ctx.Done()
	s.ended <- ctx.Err()
	return ctx.Err()
}

func (s watchingStore) Get(context.Context, string, string) (ports.Note, error) {
	return ports.Note{}, ports.ErrNotFound
}

func TestDeadlinesReachStorage(t *testing.T) {
	cases := []struct {
		name     string
		header   string
		low      time.Duration
		high     time.Duration
		testsEnd bool
	}{
		{"default", "", limits.DefaultDeadline - time.Second, limits.DefaultDeadline, false},
		{"client asks for more than the maximum", "60000", limits.MaxDeadline - time.Second, limits.MaxDeadline, false},
		{"client asks for less", "150", 0, 150 * time.Millisecond, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := newWatchingStore()
			h := start(t, store, true)
			header := http.Header{"Authorization": {testidentity.Header("ana")}}
			if c.header != "" {
				header["Connect-Timeout-Ms"] = []string{c.header}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			results := make(chan []byte, 1)
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url+"/"+notesv1connect.NoteServiceName+"/CreateNote", strings.NewReader(`{"note":{"title":"a"}}`))
			if err != nil {
				t.Fatal(err)
			}
			request.Header = header
			request.Header.Set("Content-Type", "application/json")
			go func() {
				response, err := h.http.Do(request)
				if err != nil {
					results <- nil
					return
				}
				defer func() { _ = response.Body.Close() }()
				body, _ := io.ReadAll(response.Body)
				results <- body
			}()
			got := <-store.deadline
			if got <= c.low || got > c.high {
				t.Fatalf("storage saw %v, want within (%v, %v]", got, c.low, c.high)
			}
			if !c.testsEnd {
				cancel()
				return
			}
			if err := <-store.ended; !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("storage saw %v", err)
			}
			if body := <-results; !bytes.Contains(body, []byte(`"deadline_exceeded"`)) {
				t.Fatalf("response %s", body)
			}
		})
	}
}

func TestClientCancellationReachesStorage(t *testing.T) {
	store := newWatchingStore()
	h := start(t, store, true)
	ctx, cancel := context.WithCancel(as(context.Background(), "ana"))
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := h.client.CreateNote(ctx, &notesv1.CreateNoteRequest{Note: &notesv1.Note{Title: "a"}})
		done <- err
	}()
	<-store.deadline
	cancel()
	if err := <-store.ended; !errors.Is(err, context.Canceled) {
		t.Fatalf("storage saw %v", err)
	}
	requireCode(t, <-done, connect.CodeCanceled)
}

func TestUnauthenticated(t *testing.T) {
	h := start(t, panicStore{}, true)
	cases := []struct {
		name   string
		header http.Header
	}{
		{"no credential", http.Header{}},
		{"cookie only", http.Header{"Cookie": {"session=" + testidentity.Header("ana"), "Authorization=" + testidentity.Header("ana")}}},
		{"malformed scheme", http.Header{"Authorization": {"Basic abc"}}},
		{"a token without the test label", http.Header{"Authorization": {"Bearer ana"}}},
		{"bad subject", http.Header{"Authorization": {testidentity.Header("Ana!")}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, body := post(t, h, "GetNote", `{"name":"notes/a"}`, c.header)
			if status != http.StatusUnauthorized || !bytes.Contains(body, []byte(`"unauthenticated"`)) {
				t.Fatalf("status %d: %s", status, body)
			}
		})
	}
}

func TestAuthenticationRunsBeforeDecoding(t *testing.T) {
	h := start(t, panicStore{}, true)
	status, body := post(t, h, "GetNote", `not json`, http.Header{})
	if status != http.StatusUnauthorized {
		t.Fatalf("an unauthenticated caller reached the decoder: %d %s", status, body)
	}
}

func TestCookieDoesNotOverrideACredential(t *testing.T) {
	store := memory.New(100)
	h := start(t, store, true)
	created, err := h.client.CreateNote(as(context.Background(), "ana"), &notesv1.CreateNoteRequest{Note: &notesv1.Note{Title: "a"}})
	if err != nil {
		t.Fatal(err)
	}
	header := http.Header{"Authorization": {testidentity.Header("ana")}, "Cookie": {"session=" + testidentity.Header("bob")}}
	status, body := post(t, h, "GetNote", `{"name":"`+created.GetNote().GetName()+`"}`, header)
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}
}

func TestProductionRefusesATestPrincipal(t *testing.T) {
	h := start(t, panicStore{}, false)
	_, err := h.client.GetNote(as(context.Background(), "ana"), &notesv1.GetNoteRequest{Name: "notes/a"})
	requireCode(t, err, connect.CodeUnauthenticated)
}

func TestVerifierFailureIsInternalNotAReject(t *testing.T) {
	log := &syncBuffer{}
	server := httptest.NewServer(transport.NewHandler(transport.Options{
		Notes:         app.New(panicStore{}),
		Authenticator: brokenAuthenticator{},
		AllowedOrigin: origin,
		Logger:        slog.New(slog.NewJSONHandler(log, nil)),
		Limits:        limits,
	}))
	t.Cleanup(server.Close)
	client := notesv1connect.NewNoteServiceClient(connect.NewClient(connecthttp.NewTransport(server.Client(), server.URL)))
	_, err := client.GetNote(context.Background(), &notesv1.GetNoteRequest{Name: "notes/a"})
	requireCode(t, err, connect.CodeInternal)
}

type brokenAuthenticator struct{}

func (brokenAuthenticator) Kind() identity.Kind { return identity.KindVerified }

func (brokenAuthenticator) Authenticate(context.Context, string) (identity.Principal, error) {
	return identity.Principal{}, errors.New("verifier is down")
}

func TestRequestBodyLimit(t *testing.T) {
	h := start(t, panicStore{}, true)
	body := `{"note":{"title":"` + strings.Repeat("a", limits.MaxRequestBytes) + `"}}`
	status, response := post(t, h, "CreateNote", body, http.Header{"Authorization": {testidentity.Header("ana")}})
	if status == http.StatusOK || !bytes.Contains(response, []byte(`"resource_exhausted"`)) {
		t.Fatalf("status %d: %s", status, response)
	}
}

func preflight(t *testing.T, h harness, from string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodOptions, h.url+"/"+notesv1connect.NoteServiceName+"/GetNote", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", from)
	request.Header.Set("Access-Control-Request-Method", "POST")
	request.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
	response, err := h.http.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	return response
}

func TestCORS(t *testing.T) {
	h := start(t, memory.New(100), true)
	allowed := preflight(t, h, origin)
	if got := allowed.Header.Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("allow-origin %q", got)
	}
	if got := allowed.Header.Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Authorization") {
		t.Fatalf("allow-headers %q", got)
	}
	if allowed.Header.Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("credentials must never be allowed")
	}
	if !strings.Contains(strings.Join(allowed.Header.Values("Vary"), ","), "Origin") {
		t.Fatal("responses must vary by origin")
	}
	for _, other := range []string{
		"https://evil.example.com",
		origin + ".evil.example",
		origin + ":8443",
		"http://app.example.com",
		"https://app.example.co",
		"null",
	} {
		response := preflight(t, h, other)
		if got := response.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("%s: allow-origin %q", other, got)
		}
		if got := response.Header.Get("Access-Control-Allow-Methods"); got != "" {
			t.Errorf("%s: allow-methods %q", other, got)
		}
	}
	status, _ := post(t, h, "GetNote", `{"name":"notes/a"}`, http.Header{"Origin": {origin}})
	if status != http.StatusUnauthorized {
		t.Fatalf("status %d", status)
	}
}

// Configuration alone changes the behavior (spec 011 FR-016, T021): the same handler with a smaller
// maximum caps a client that asks for a minute at that maximum, and a smaller body bound refuses a body
// the default bound accepted.
func TestTheBoundsAreConfiguration(t *testing.T) {
	tight := transport.Limits{DefaultDeadline: 200 * time.Millisecond, MaxDeadline: 300 * time.Millisecond, MaxRequestBytes: 8 << 10}
	store := newWatchingStore()
	h := startWith(t, store, true, tight)
	header := http.Header{"Authorization": {testidentity.Header("ana")}, "Connect-Timeout-Ms": {"60000"}}
	post(t, h, "CreateNote", `{"note":{"title":"a"}}`, header) // returns once the configured deadline ends the call
	deadline := <-store.deadline
	if deadline > tight.MaxDeadline || deadline < tight.MaxDeadline-100*time.Millisecond {
		t.Fatalf("the configured maximum did not bound the call: storage saw %v", deadline)
	}
	body := `{"note":{"title":"` + strings.Repeat("a", 10<<10) + `"}}`
	status, response := post(t, h, "CreateNote", body, http.Header{"Authorization": {testidentity.Header("ana")}})
	if status == http.StatusOK || !bytes.Contains(response, []byte(`"resource_exhausted"`)) {
		t.Fatalf("the configured body bound did not refuse: %d %s", status, response)
	}
	// Under the larger bound the same body is decoded, and then the schema refuses its title: a different
	// refusal, from the configuration alone.
	status, response = post(t, start(t, memory.New(100), true), "CreateNote", body, http.Header{"Authorization": {testidentity.Header("ana")}})
	if status != http.StatusBadRequest || !bytes.Contains(response, []byte(`"invalid_argument"`)) {
		t.Fatalf("the same body under the larger bound: %d %s", status, response)
	}
}

// Every refusal of a bound names its field, and a valid set passes.
func TestLimitsCheck(t *testing.T) {
	cases := map[string]struct {
		limits transport.Limits
		want   string
	}{
		"valid":                  {limits, ""},
		"default omitted":        {transport.Limits{MaxDeadline: time.Second, MaxRequestBytes: 64 << 10}, "calls.default_deadline: required"},
		"maximum omitted":        {transport.Limits{DefaultDeadline: time.Second, MaxRequestBytes: 64 << 10}, "calls.max_deadline: required"},
		"default above maximum":  {transport.Limits{DefaultDeadline: 2 * time.Second, MaxDeadline: time.Second, MaxRequestBytes: 64 << 10}, "exceeds calls.max_deadline"},
		"maximum beyond ceiling": {transport.Limits{DefaultDeadline: time.Second, MaxDeadline: time.Hour, MaxRequestBytes: 64 << 10}, "exceeds the ceiling"},
		"bytes below floor":      {transport.Limits{DefaultDeadline: time.Second, MaxDeadline: time.Second, MaxRequestBytes: 100}, "is below"},
		"bytes beyond ceiling":   {transport.Limits{DefaultDeadline: time.Second, MaxDeadline: time.Second, MaxRequestBytes: 8 << 20}, "exceeds the ceiling"},
	}
	for name, c := range cases {
		err := c.limits.Check()
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: %v", name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: got %v, want %q", name, err, c.want)
		}
	}
}
