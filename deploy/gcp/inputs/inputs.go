// Package inputs is the hosted example's one typed funnel (spec 012 FR-003): every value is declared once against
// inputs.schema.json, resolved from a base file and an operator's overlays where the last file wins per leaf,
// validated here before any plan with the source of every effective value, and passed down as required
// parameters. Nothing below this package carries a default. The public example holds synthetic values only.
package inputs

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
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
	NotesBucket      string `json:"notes_bucket"`
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

// Telemetry is what the production binary's composition requires or defaults for a hosted run (spec 011 G09). The
// release revision is not here: it is the revision of the digest served, which the module derives from the image
// and the release.
type Telemetry struct {
	Endpoint    string  `json:"endpoint"`
	Environment string  `json:"environment"`
	SampleRatio float64 `json:"sample_ratio"`
}

type Runtime struct {
	Port          int         `json:"port"`
	AllowedOrigin string      `json:"allowed_origin"`
	Verifier      Verifier    `json:"verifier"`
	Persistence   Persistence `json:"persistence"`
	Telemetry     Telemetry   `json:"telemetry"`
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
	ServingDigest    string  `json:"serving_digest"`
	PreviousDigest   *string `json:"previous_digest"`
	PreviousRevision *string `json:"previous_revision"`
	CancelInFlight   bool    `json:"cancel_in_flight"`
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
	bucket    = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,61}[a-z0-9]$`)
)

// Validate applies every rule the schema states and the ones a schema cannot: the identities are distinct, the
// region is the contract's, the serving digest is the image's or the previous one, the origin has no path, and
// the previous digest and its revision come together.
func (in *Inputs) Validate() error {
	checks := []func() error{in.validateVersions, in.validateContract, in.validateProject, in.validateImage, in.validateRuntime, in.validateTelemetry, in.validateFrontend, in.validateBounds, in.validateRelease, in.validatePreviousRevision}
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
	case !bucket.MatchString(c.NotesBucket):
		return fmt.Errorf("contract.notes_bucket: %q is not a bucket name", c.NotesBucket)
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
