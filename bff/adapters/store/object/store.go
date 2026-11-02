// Package object is a ports.Store over a managed object bucket, the hosted example's persistence (spec 012
// FR-017): one object per note, named by the owner and the id in hex so a subject or an id that looks like a path
// can never leave the prefix, written and read through the bucket service's JSON API with a bearer token the
// runtime obtains from its own token endpoint. It uses no client library: two calls, a put and a get, are the
// whole surface. A note survives a restart of the service because it is in the bucket, not in the process;
// nothing here is a claim beyond what the bucket's own durability provides.
package object

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"example.com/bfftemplate/bff/ports"
)

// Settings name the bucket and the two endpoints the composition declares. Every field is required.
type Settings struct {
	// Bucket is the bucket's name; the infrastructure owner's contract names it.
	Bucket string
	// Endpoint is the bucket service's API origin, such as the public storage API.
	Endpoint string
	// TokenURL is where the runtime reads a bearer token for its own identity, such as the metadata server's
	// token endpoint; its response carries access_token and expires_in.
	TokenURL string
	// Prefix is the object-name prefix every note lives under.
	Prefix           string
	TokenMaxBytes    int64
	ObjectMaxBytes   int64
	TokenRefreshSkew time.Duration
}

// Check refuses an incomplete setting by name.
func (s Settings) Check() error {
	if err := s.checkBounds(); err != nil {
		return err
	}
	if s.Bucket == "" {
		return errors.New("store.bucket: required")
	}
	for name, raw := range map[string]string{"store.endpoint": s.Endpoint, "store.token_url": s.TokenURL} {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return fmt.Errorf("%s: %q is not an http or https URL", name, raw)
		}
	}
	if s.Prefix == "" || strings.Contains(s.Prefix, "..") {
		return errors.New("store.prefix: required, without a parent reference")
	}
	return nil
}

// Store keeps notes as objects.
type Store struct {
	settings Settings
	client   *http.Client
	now      func() time.Time
	gate     chan struct{}
	token    string
	expires  time.Time
}

type record struct {
	Owner      string    `json:"owner"`
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	CreateTime time.Time `json:"create_time"`
}

// Open prepares a store; it reads nothing until the first call, and a store that cannot reach its bucket reports
// that as unavailable on each call rather than refusing to start, since the bucket may return.
func Open(settings Settings, client *http.Client) (*Store, error) {
	if err := settings.Check(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("store: an HTTP client is required")
	}
	owned := *client
	owned.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Store{settings: settings, client: &owned, now: time.Now, gate: make(chan struct{}, 1)}, nil
}

func (s *Store) name(owner, id string) string {
	return strings.TrimSuffix(s.settings.Prefix, "/") + "/" + hex.EncodeToString([]byte(owner)) + "/" + hex.EncodeToString([]byte(id)) + ".json"
}

// bearer returns the runtime's token, read from the token endpoint and kept until shortly before it expires.
func (s *Store) bearer(ctx context.Context) (string, error) {
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s.token != "" && s.now().Before(s.expires) {
		return s.token, nil
	}
	issued, err := s.readToken(ctx)
	if err != nil {
		return "", err
	}
	s.token = issued.AccessToken
	s.expires = s.now().Add(time.Duration(issued.ExpiresIn)*time.Second - s.settings.TokenRefreshSkew)
	return s.token, nil
}

func (s *Store) call(ctx context.Context, method, target string, body []byte) (*http.Response, error) {
	token, err := s.bearer(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %v", ports.ErrUnavailable, err)
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ports.ErrUnavailable, err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := s.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %v", ports.ErrUnavailable, err)
	}
	return response, nil
}

// Put writes the note as one object; a later put with the same owner and id replaces it.
func (s *Store) Put(ctx context.Context, note ports.Note) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(record(note))
	if err != nil {
		return fmt.Errorf("%w: %v", ports.ErrUnavailable, err)
	}
	if int64(len(data)) > s.settings.ObjectMaxBytes {
		return fmt.Errorf("%w: note exceeds object byte limit", ports.ErrUnavailable)
	}
	target := s.settings.Endpoint + "/upload/storage/v1/b/" + url.PathEscape(s.settings.Bucket) + "/o?uploadType=media&name=" + url.QueryEscape(s.name(note.Owner, note.ID))
	response, err := s.call(ctx, http.MethodPost, target, data)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: the bucket answered %d to a put", ports.ErrUnavailable, response.StatusCode)
	}
	return nil
}

// Get reads the owner's note, or ports.ErrNotFound.
func (s *Store) Get(ctx context.Context, owner, id string) (ports.Note, error) {
	if err := ctx.Err(); err != nil {
		return ports.Note{}, err
	}
	target := s.settings.Endpoint + "/storage/v1/b/" + url.PathEscape(s.settings.Bucket) + "/o/" + url.PathEscape(s.name(owner, id)) + "?alt=media"
	response, err := s.call(ctx, http.MethodGet, target, nil)
	if err != nil {
		return ports.Note{}, err
	}
	defer func() { _ = response.Body.Close() }()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return ports.Note{}, ports.ErrNotFound
	default:
		return ports.Note{}, fmt.Errorf("%w: the bucket answered %d to a get", ports.ErrUnavailable, response.StatusCode)
	}
	var stored record
	if err := readDocument(response.Body, s.settings.ObjectMaxBytes, &stored); err != nil {
		if ctx.Err() != nil {
			return ports.Note{}, ctx.Err()
		}
		return ports.Note{}, fmt.Errorf("%w: the object holds no note: %v", ports.ErrUnavailable, err)
	}
	if stored.Owner != owner || stored.ID != id {
		return ports.Note{}, fmt.Errorf("%w: the stored note is not the one asked for", ports.ErrUnavailable)
	}
	return ports.Note(stored), nil
}

// Read the entire bounded document, rejecting a valid prefix followed by excess or garbage.
func readDocument(body io.Reader, maximum int64, value any) error {
	data, err := io.ReadAll(io.LimitReader(body, maximum+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > maximum {
		return errors.New("document exceeds byte limit")
	}
	return json.Unmarshal(data, value)
}

func (s Settings) checkBounds() error {
	if s.TokenMaxBytes < 1 || s.TokenMaxBytes > 1<<20 || s.ObjectMaxBytes < 1 || s.ObjectMaxBytes > 16<<20 {
		return errors.New("store: required payload bounds outside safety ceilings")
	}
	if s.TokenRefreshSkew < 0 || s.TokenRefreshSkew > time.Hour {
		return errors.New("store: token refresh skew outside safety ceilings")
	}
	return nil
}

type issuedToken struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

func (s *Store) readToken(ctx context.Context) (issuedToken, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.settings.TokenURL, nil)
	if err != nil {
		return issuedToken{}, err
	}
	request.Header.Set("Metadata-Flavor", "Google")
	response, err := s.client.Do(request)
	if err != nil {
		return issuedToken{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return issuedToken{}, fmt.Errorf("the token endpoint answered %d", response.StatusCode)
	}
	var issued issuedToken
	if err := readDocument(response.Body, s.settings.TokenMaxBytes, &issued); err != nil || issued.AccessToken == "" || issued.ExpiresIn <= 0 {
		return issuedToken{}, errors.New("the token endpoint answered with no token")
	}
	return issued, nil
}
