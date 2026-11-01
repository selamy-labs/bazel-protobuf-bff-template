// Package inputs is the hosted example's one typed funnel (spec 012 FR-003): every value is declared once against
// inputs.schema.json, resolved from a base file and an operator's overlays where the last file wins per leaf,
// validated here before any plan with the source of every effective value, and passed down as required
// parameters. Nothing below this package carries a default. The public example holds synthetic values only.
package inputs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

// Inputs mirrors inputs.schema.json field for field; a file with an unknown or missing field is refused by name.
type Inputs struct {
	Version  int      `json:"version"`
	Contract Contract `json:"contract"`
	Project  Project  `json:"project"`
	Region   string   `json:"region"`
	Image    Image    `json:"image"`
	Runtime  Runtime  `json:"runtime"`
	Frontend Frontend `json:"frontend"`
	Bounds   Bounds   `json:"bounds"`
	Release  Release  `json:"release"`
}

// Contract is the infrastructure owner's versioned output contract, consumed as inputs.
type Contract struct {
	Version          int    `json:"version"`
	ProjectID        string `json:"project_id"`
	Region           string `json:"region"`
	RuntimeIdentity  string `json:"runtime_identity"`
	BuildIdentity    string `json:"build_identity"`
	ArtifactRegistry string `json:"artifact_registry"`
	StateBucket      string `json:"state_bucket"`
}

type Project struct {
	DisplayName             string `json:"display_name"`
	Parent                  string `json:"parent"`
	OwnerGroup              string `json:"owner_group"`
	BillingAccountReference string `json:"billing_account_reference"`
}

type Image struct {
	Digest         string `json:"digest"`
	SourceRevision string `json:"source_revision"`
}

type Verifier struct {
	Issuer   string `json:"issuer"`
	Audience string `json:"audience"`
	JWKSURL  string `json:"jwks_url"`
}

type Persistence struct {
	Kind      string `json:"kind"`
	Statement string `json:"statement"`
}

type Runtime struct {
	Port          int         `json:"port"`
	AllowedOrigin string      `json:"allowed_origin"`
	Verifier      Verifier    `json:"verifier"`
	Persistence   Persistence `json:"persistence"`
}

type Frontend struct {
	Host       string `json:"host"`
	BasePath   string `json:"base_path"`
	RPCBaseURL string `json:"rpc_base_url"`
}

type Bounds struct {
	MaxInstances          int     `json:"max_instances"`
	Concurrency           int     `json:"concurrency"`
	RequestTimeoutSeconds int     `json:"request_timeout_seconds"`
	BudgetAlertAmount     float64 `json:"budget_alert_amount"`
	BudgetAlertCurrency   string  `json:"budget_alert_currency"`
}

type Release struct {
	ServingDigest  string  `json:"serving_digest"`
	PreviousDigest *string `json:"previous_digest"`
	CancelInFlight bool    `json:"cancel_in_flight"`
}

var (
	projectID = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)
	region    = regexp.MustCompile(`^[a-z]+-[a-z]+[0-9]$`)
	parent    = regexp.MustCompile(`^(organizations|folders)/[0-9]+$`)
	digest    = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	revision  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	basePath  = regexp.MustCompile(`^/([a-z0-9-]+/)*$`)
	currency  = regexp.MustCompile(`^[A-Z]{3}$`)
	email     = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

// Validate applies every rule the schema states and the ones a schema cannot: the identities are distinct, the
// region is the contract's, the serving digest is the image's or the previous one, the origin has no path.
func (in *Inputs) Validate() error {
	checks := []func() error{in.validateVersions, in.validateContract, in.validateProject, in.validateImage, in.validateRuntime, in.validateFrontend, in.validateBounds, in.validateRelease}
	for _, check := range checks {
		if err := check(); err != nil {
			return err
		}
	}
	return nil
}

func (in *Inputs) validateVersions() error {
	if in.Version != 1 {
		return fmt.Errorf("version: %d is not the supported schema version 1", in.Version)
	}
	if in.Contract.Version != 1 {
		return fmt.Errorf("contract.version: %d is not the supported contract version 1", in.Contract.Version)
	}
	return nil
}

func (in *Inputs) validateContract() error {
	c := in.Contract
	switch {
	case !projectID.MatchString(c.ProjectID):
		return fmt.Errorf("contract.project_id: %q is not a project id", c.ProjectID)
	case !region.MatchString(c.Region):
		return fmt.Errorf("contract.region: %q is not a region", c.Region)
	case !email.MatchString(c.RuntimeIdentity):
		return fmt.Errorf("contract.runtime_identity: %q is not an identity", c.RuntimeIdentity)
	case !email.MatchString(c.BuildIdentity):
		return fmt.Errorf("contract.build_identity: %q is not an identity", c.BuildIdentity)
	case c.RuntimeIdentity == c.BuildIdentity:
		return errors.New("contract.runtime_identity and contract.build_identity: the same identity; the runtime must hold no build or deploy permission")
	case c.ArtifactRegistry == "" || c.StateBucket == "":
		return errors.New("contract.artifact_registry and contract.state_bucket: required")
	case in.Region != c.Region:
		return fmt.Errorf("region: %q differs from the contract's %q", in.Region, c.Region)
	}
	return nil
}

func (in *Inputs) validateProject() error {
	p := in.Project
	switch {
	case len(p.DisplayName) < 4 || len(p.DisplayName) > 30:
		return fmt.Errorf("project.display_name: %q must be 4 to 30 characters", p.DisplayName)
	case !parent.MatchString(p.Parent):
		return fmt.Errorf("project.parent: %q is not an organization or folder", p.Parent)
	case !email.MatchString(p.OwnerGroup):
		return fmt.Errorf("project.owner_group: %q is not a group", p.OwnerGroup)
	case p.BillingAccountReference == "":
		return errors.New("project.billing_account_reference: required")
	}
	return nil
}

func (in *Inputs) validateImage() error {
	if !digest.MatchString(in.Image.Digest) {
		return fmt.Errorf("image.digest: %q is not a sha256 digest", in.Image.Digest)
	}
	if !revision.MatchString(in.Image.SourceRevision) {
		return fmt.Errorf("image.source_revision: %q is not a full revision", in.Image.SourceRevision)
	}
	return nil
}

func (in *Inputs) validateRuntime() error {
	r := in.Runtime
	if r.Port < 1 || r.Port > 65535 {
		return fmt.Errorf("runtime.port: %d is not a port", r.Port)
	}
	if err := exactOrigin("runtime.allowed_origin", r.AllowedOrigin); err != nil {
		return err
	}
	for name, raw := range map[string]string{"runtime.verifier.issuer": r.Verifier.Issuer, "runtime.verifier.jwks_url": r.Verifier.JWKSURL} {
		if u, err := url.Parse(raw); err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("%s: %q is not an https URL", name, raw)
		}
	}
	if r.Verifier.Audience == "" {
		return errors.New("runtime.verifier.audience: required")
	}
	if r.Persistence.Kind != "managed-store" && r.Persistence.Kind != "memory" {
		return fmt.Errorf("runtime.persistence.kind: %q is neither managed-store nor memory", r.Persistence.Kind)
	}
	if len(strings.TrimSpace(r.Persistence.Statement)) < 20 {
		return errors.New("runtime.persistence.statement: say what survives a revision change")
	}
	return nil
}

func (in *Inputs) validateFrontend() error {
	f := in.Frontend
	if f.Host != "github-pages" {
		return fmt.Errorf("frontend.host: %q is not github-pages", f.Host)
	}
	if !basePath.MatchString(f.BasePath) {
		return fmt.Errorf("frontend.base_path: %q must start and end with a slash", f.BasePath)
	}
	if err := exactOrigin("frontend.rpc_base_url", f.RPCBaseURL); err != nil {
		return err
	}
	if f.RPCBaseURL == in.Runtime.AllowedOrigin {
		return errors.New("frontend.rpc_base_url equals runtime.allowed_origin: the backend's origin and the page's origin are different hosts")
	}
	return nil
}

func (in *Inputs) validateBounds() error {
	b := in.Bounds
	switch {
	case b.MaxInstances < 1 || b.MaxInstances > 10:
		return fmt.Errorf("bounds.max_instances: %d is not between 1 and 10", b.MaxInstances)
	case b.Concurrency < 1 || b.Concurrency > 80:
		return fmt.Errorf("bounds.concurrency: %d is not between 1 and 80", b.Concurrency)
	case b.RequestTimeoutSeconds < 1 || b.RequestTimeoutSeconds > 300:
		return fmt.Errorf("bounds.request_timeout_seconds: %d is not between 1 and 300", b.RequestTimeoutSeconds)
	case b.BudgetAlertAmount <= 0:
		return errors.New("bounds.budget_alert_amount: must be positive; it is an alert, never a cap")
	case !currency.MatchString(b.BudgetAlertCurrency):
		return fmt.Errorf("bounds.budget_alert_currency: %q is not a currency code", b.BudgetAlertCurrency)
	}
	return nil
}

func (in *Inputs) validateRelease() error {
	r := in.Release
	if !digest.MatchString(r.ServingDigest) {
		return fmt.Errorf("release.serving_digest: %q is not a sha256 digest", r.ServingDigest)
	}
	if r.PreviousDigest != nil && !digest.MatchString(*r.PreviousDigest) {
		return fmt.Errorf("release.previous_digest: %q is not a sha256 digest", *r.PreviousDigest)
	}
	if r.ServingDigest != in.Image.Digest && (r.PreviousDigest == nil || r.ServingDigest != *r.PreviousDigest) {
		return fmt.Errorf("release.serving_digest: %s is neither the image digest nor the previous digest; nothing else is ever deployed", r.ServingDigest)
	}
	return nil
}

// exactOrigin accepts one https origin with a host and nothing else: no path, query, user or fragment.
func exactOrigin(name, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.User != nil || u.Fragment != "" || strings.Contains(u.Host, "*") {
		return fmt.Errorf("%s: %q must be one exact https origin with no path", name, raw)
	}
	return nil
}

// Provenance names, for each leaf of the schema by its dotted path, the file that gave its effective value.
type Provenance map[string]string

// Load reads and validates one inputs file: Resolve with no overlay.
func Load(path string) (*Inputs, error) {
	in, _, _, err := Resolve(path)
	return in, err
}

// Resolve reads the base file and the overlays in order and merges them leaf by leaf: a later file wins. Each
// file may hold any subset of the schema's leaves, in the schema's shape; a key outside the schema, a section
// given as a value or a value given as a section is refused naming the file and the key. A leaf given by no
// file is refused by name: nothing is defaulted here. The merged document is validated as one file would be,
// and returned with the provenance of every leaf and its canonical JSON, which is what a plan reads.
func Resolve(base string, overlays ...string) (*Inputs, Provenance, []byte, error) {
	leaves, sections := shape()
	values := map[string]any{}
	provenance := Provenance{}
	for _, path := range append([]string{base}, overlays...) {
		document, err := readDocument(path)
		if err != nil {
			return nil, nil, nil, err
		}
		if err := collect(filepath.Base(path), "", document, leaves, sections, values, provenance); err != nil {
			return nil, nil, nil, err
		}
	}
	for _, leaf := range sortedKeys(leaves) {
		if _, ok := values[leaf]; !ok {
			return nil, nil, nil, fmt.Errorf("%s: given by no file, and nothing is defaulted here", leaf)
		}
	}
	// Values that came out of a JSON decode always marshal.
	effective, _ := json.MarshalIndent(nest(values), "", "  ")
	decoder := json.NewDecoder(bytes.NewReader(effective))
	decoder.DisallowUnknownFields()
	var in Inputs
	if err := decoder.Decode(&in); err != nil {
		return nil, nil, nil, fmt.Errorf("%s: %w", filepath.Base(base), err)
	}
	if err := in.Validate(); err != nil {
		return nil, nil, nil, err
	}
	return &in, provenance, append(effective, '\n'), nil
}

// readDocument reads one file as the generic JSON shape, so that the presence of each key can be seen: a struct
// decode cannot tell an omitted leaf from its zero value, nor say which file gave a value.
func readDocument(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return document, nil
}

// shape derives the schema's leaves and sections from the struct, which the schema test keeps equal to the
// schema: a leaf is a field that is not a nested struct, named by its dotted path.
func shape() (leaves, sections map[string]bool) {
	leaves, sections = map[string]bool{}, map[string]bool{}
	var walk func(t reflect.Type, prefix string)
	walk = func(t reflect.Type, prefix string) {
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			path := prefix + field.Tag.Get("json")
			if field.Type.Kind() == reflect.Struct {
				sections[path] = true
				walk(field.Type, path+".")
			} else {
				leaves[path] = true
			}
		}
	}
	walk(reflect.TypeOf(Inputs{}), "")
	return leaves, sections
}

// collect walks one document against the shape, recording each leaf's value and the file that gave it.
func collect(file, prefix string, node map[string]any, leaves, sections map[string]bool, values map[string]any, provenance Provenance) error {
	for _, key := range sortedKeys(node) {
		path := prefix + key
		value := node[key]
		child, isSection := value.(map[string]any)
		switch {
		case sections[path] && isSection:
			if err := collect(file, path+".", child, leaves, sections, values, provenance); err != nil {
				return err
			}
		case sections[path]:
			return fmt.Errorf("%s: %s: is a section of the schema, not a value", file, path)
		case leaves[path] && isSection:
			return fmt.Errorf("%s: %s: is a value of the schema, not a section", file, path)
		case leaves[path]:
			values[path] = value
			provenance[path] = file
		default:
			return fmt.Errorf("%s: %s: not in the schema", file, path)
		}
	}
	return nil
}

// nest rebuilds the document from the dotted leaf paths.
func nest(values map[string]any) map[string]any {
	document := map[string]any{}
	for path, value := range values {
		node := document
		parts := strings.Split(path, ".")
		for _, part := range parts[:len(parts)-1] {
			next, ok := node[part].(map[string]any)
			if !ok {
				next = map[string]any{}
				node[part] = next
			}
			node = next
		}
		node[parts[len(parts)-1]] = value
	}
	return document
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Render prints every effective leaf with its source, one `path=value # from file` line each, in path order.
func Render(values []byte, provenance Provenance) ([]string, error) {
	var document map[string]any
	decoder := json.NewDecoder(bytes.NewReader(values))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	flat := map[string]any{}
	flatten("", document, flat)
	var lines []string
	for _, path := range sortedKeys(flat) {
		text, _ := json.Marshal(flat[path]) // decoded from JSON, so it marshals
		lines = append(lines, fmt.Sprintf("%s=%s # from %s", path, text, provenance[path]))
	}
	return lines, nil
}

func flatten(prefix string, node map[string]any, into map[string]any) {
	for key, value := range node {
		if child, ok := value.(map[string]any); ok {
			flatten(prefix+key+".", child, into)
		} else {
			into[prefix+key] = value
		}
	}
}
