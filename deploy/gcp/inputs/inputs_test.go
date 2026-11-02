package inputs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func example(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv("EXAMPLE")))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func write(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inputs.json")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTheSyntheticExampleValidates(t *testing.T) {
	in, err := Load(write(t, example(t)))
	if err != nil {
		t.Fatal(err)
	}
	if in.Contract.ProjectID != "example-bff-hosted-000000" || in.Frontend.BasePath != "/bazel-protobuf-bff-template/" {
		t.Errorf("%+v", in)
	}
	for _, value := range []string{in.Contract.ProjectID, in.Contract.RuntimeIdentity, in.Project.OwnerGroup, in.Runtime.Verifier.Issuer, in.Runtime.Telemetry.Endpoint, in.Frontend.RPCBaseURL} {
		if !strings.Contains(value, "example") {
			t.Errorf("the public example holds a value that does not look synthetic: %q", value)
		}
	}
}

// The schema and the struct name the same fields: a field added to one without the other is a refusal here.
func TestTheSchemaAndTheStructAgree(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv("SCHEMA")))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	var fields []string
	typ := reflect.TypeOf(Inputs{})
	for i := 0; i < typ.NumField(); i++ {
		fields = append(fields, typ.Field(i).Tag.Get("json"))
	}
	if !reflect.DeepEqual(fields, schema.Required) {
		t.Errorf("struct fields %v, schema required %v", fields, schema.Required)
	}
	for _, field := range fields {
		if _, ok := schema.Properties[field]; !ok {
			t.Errorf("the schema lacks %s", field)
		}
	}
}

func TestEveryRefusalNamesTheField(t *testing.T) {
	base := example(t)
	cases := map[string]struct{ old, new, want string }{
		"unknown field":             {`"version": 1,`, `"version": 1, "waiver": true,`, "waiver"},
		"wrong version":             {`"version": 1,`, `"version": 2,`, "version"},
		"bad project id":            {`"project_id": "example-bff-hosted-000000"`, `"project_id": "Bad_Project"`, "contract.project_id"},
		"same identities":           {`"build_identity": "bff-build@example.invalid"`, `"build_identity": "bff-runtime@example.invalid"`, "runtime must hold no build"},
		"region differs":            {"\"region\": \"example-region1\",\n  \"image\"", "\"region\": \"example-region2\",\n  \"image\"", "region"},
		"digest not sha256":         {`"digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"`, `"digest": "latest"`, "image.digest"},
		"origin with a path":        {`"allowed_origin": "https://example-owner.github.example.invalid"`, `"allowed_origin": "https://example-owner.github.example.invalid/app"`, "runtime.allowed_origin"},
		"wildcard origin":           {`"allowed_origin": "https://example-owner.github.example.invalid"`, `"allowed_origin": "https://*.example.invalid"`, "runtime.allowed_origin"},
		"http issuer":               {`"issuer": "https://issuer.example.invalid/"`, `"issuer": "http://issuer.example.invalid/"`, "runtime.verifier.issuer"},
		"unknown persistence":       {`"kind": "managed-store"`, `"kind": "one-instance"`, "runtime.persistence.kind"},
		"base path without slash":   {`"base_path": "/bazel-protobuf-bff-template/"`, `"base_path": "bazel-protobuf-bff-template"`, "frontend.base_path"},
		"rpc url equals the page":   {`"rpc_base_url": "https://bff.example.invalid"`, `"rpc_base_url": "https://example-owner.github.example.invalid"`, "different hosts"},
		"too many instances":        {`"max_instances": 2`, `"max_instances": 50`, "bounds.max_instances"},
		"budget zero":               {`"budget_alert_amount": 10`, `"budget_alert_amount": 0`, "bounds.budget_alert_amount"},
		"serving an unknown digest": {`"serving_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"`, `"serving_digest": "sha256:1111111111111111111111111111111111111111111111111111111111111111"`, "neither the image digest nor the previous"},
		"not json":                  {`"version": 1,`, `"version": 1`, "inputs.json"},
		"wrong contract version":    {"\"version\": 1,\n    \"project_id\"", "\"version\": 3,\n    \"project_id\"", "contract.version"},
		"bad contract region":       {"\"region\": \"example-region1\",\n    \"runtime_identity\"", "\"region\": \"Region!\",\n    \"runtime_identity\"", "contract.region"},
		"bad runtime identity":      {`"runtime_identity": "bff-runtime@example.invalid"`, `"runtime_identity": "not-an-identity"`, "contract.runtime_identity"},
		"bad build identity":        {`"build_identity": "bff-build@example.invalid"`, `"build_identity": "nope"`, "contract.build_identity"},
		"missing registry":          {`"artifact_registry": "example-region1-docker.example.invalid/example-bff-hosted-000000/bff"`, `"artifact_registry": ""`, "contract.artifact_registry"},
		"bad notes bucket":          {`"notes_bucket": "example-bff-hosted-000000-notes"`, `"notes_bucket": "Notes Bucket"`, "contract.notes_bucket"},
		"short display name":        {`"display_name": "BFF hosted example"`, `"display_name": "BFF"`, "project.display_name"},
		"bad parent":                {`"parent": "organizations/000000000000"`, `"parent": "projects/x"`, "project.parent"},
		"bad owner group":           {`"owner_group": "bff-owners@example.invalid"`, `"owner_group": "owners"`, "project.owner_group"},
		"missing billing reference": {`"billing_account_reference": "synthetic-reference-resolved-by-the-infrastructure-owner"`, `"billing_account_reference": ""`, "project.billing_account_reference"},
		"bad source revision":       {`"source_revision": "0000000000000000000000000000000000000000"`, `"source_revision": "main"`, "image.source_revision"},
		"port zero":                 {`"port": 8080`, `"port": 0`, "runtime.port"},
		"bad jwks url":              {`"jwks_url": "https://issuer.example.invalid/.well-known/jwks.json"`, `"jwks_url": "ftp://issuer.example.invalid/jwks"`, "runtime.verifier.jwks_url"},
		"empty audience":            {`"audience": "bff-hosted-example"`, `"audience": ""`, "runtime.verifier.audience"},
		"short statement":           {`"statement": "a note survives a revision change and a restart; it is read by its owner only"`, `"statement": "it persists"`, "runtime.persistence.statement"},
		"unknown host":              {`"host": "github-pages"`, `"host": "pages"`, "frontend.host"},
		"rpc url with a path":       {`"rpc_base_url": "https://bff.example.invalid"`, `"rpc_base_url": "https://bff.example.invalid/api"`, "frontend.rpc_base_url"},
		"concurrency too high":      {`"concurrency": 20`, `"concurrency": 500`, "bounds.concurrency"},
		"timeout too long":          {`"request_timeout_seconds": 30`, `"request_timeout_seconds": 900`, "bounds.request_timeout_seconds"},
		"bad currency":              {`"budget_alert_currency": "USD"`, `"budget_alert_currency": "dollars"`, "bounds.budget_alert_currency"},
		"bad previous digest":       {`"previous_digest": null`, `"previous_digest": "sha256:short"`, "release.previous_digest"},
		"bad serving digest":        {`"serving_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"`, `"serving_digest": "v1"`, "release.serving_digest"},
		"plain http telemetry":      {`"endpoint": "https://otel.example.invalid"`, `"endpoint": "http://otel.example.invalid"`, "runtime.telemetry.endpoint"},
		"telemetry with a path":     {`"endpoint": "https://otel.example.invalid"`, `"endpoint": "https://otel.example.invalid/v1/traces"`, "runtime.telemetry.endpoint"},
		"unknown environment":       {`"environment": "prod"`, `"environment": "production"`, "runtime.telemetry.environment"},
		"sample ratio above one":    {`"sample_ratio": 1`, `"sample_ratio": 1.5`, "runtime.telemetry.sample_ratio"},
		"sample ratio below zero":   {`"sample_ratio": 1`, `"sample_ratio": -0.1`, "runtime.telemetry.sample_ratio"},
		"revision without a digest": {`"previous_revision": null`, `"previous_revision": "2222222222222222222222222222222222222222"`, "release.previous_revision"},
		"telemetry with no host":    {`"endpoint": "https://otel.example.invalid"`, `"endpoint": "https://:443"`, "runtime.telemetry.endpoint"},
		"telemetry on localhost":    {`"endpoint": "https://otel.example.invalid"`, `"endpoint": "https://localhost:4318"`, "runtime.telemetry.endpoint"},
		"telemetry on upper case":   {`"endpoint": "https://otel.example.invalid"`, `"endpoint": "https://LOCALHOST:4318"`, "runtime.telemetry.endpoint"},
		"telemetry on a dotted one": {`"endpoint": "https://otel.example.invalid"`, `"endpoint": "https://localhost.:4318"`, "runtime.telemetry.endpoint"},
		"telemetry on a subdomain":  {`"endpoint": "https://otel.example.invalid"`, `"endpoint": "https://otel.localhost:4318"`, "runtime.telemetry.endpoint"},
		"telemetry on loopback":     {`"endpoint": "https://otel.example.invalid"`, `"endpoint": "https://127.0.0.1:4318"`, "runtime.telemetry.endpoint"},
		"telemetry on ipv6":         {`"endpoint": "https://otel.example.invalid"`, `"endpoint": "https://[::1]:4318"`, "runtime.telemetry.endpoint"},
		"telemetry on mapped ipv4":  {`"endpoint": "https://otel.example.invalid"`, `"endpoint": "https://[::ffff:127.0.0.1]:4318"`, "runtime.telemetry.endpoint"},
		"telemetry unspecified":     {`"endpoint": "https://otel.example.invalid"`, `"endpoint": "https://0.0.0.0:4318"`, "runtime.telemetry.endpoint"},
		"telemetry unspecified v6":  {`"endpoint": "https://otel.example.invalid"`, `"endpoint": "https://[::]:4318"`, "runtime.telemetry.endpoint"},
		"telemetry short form":      {`"endpoint": "https://otel.example.invalid"`, `"endpoint": "https://127.1:4318"`, "runtime.telemetry.endpoint"},
		"telemetry decimal form":    {`"endpoint": "https://otel.example.invalid"`, `"endpoint": "https://2130706433:4318"`, "runtime.telemetry.endpoint"},
		"telemetry hex form":        {`"endpoint": "https://otel.example.invalid"`, `"endpoint": "https://0x7f.0.0.1:4318"`, "runtime.telemetry.endpoint"},
		"telemetry only a dot":      {`"endpoint": "https://otel.example.invalid"`, `"endpoint": "https://.:4318"`, "runtime.telemetry.endpoint"},
		"page on loopback":          {`"allowed_origin": "https://example-owner.github.example.invalid"`, `"allowed_origin": "https://localhost:3000"`, "runtime.allowed_origin"},
		"rpc url on loopback":       {`"rpc_base_url": "https://bff.example.invalid"`, `"rpc_base_url": "https://127.0.0.1:8080"`, "frontend.rpc_base_url"},
	}
	for name, c := range cases {
		text := strings.Replace(base, c.old, c.new, 1)
		if text == base {
			t.Fatalf("%s: the mutation changed nothing", name)
		}
		_, err := Load(write(t, text))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want a refusal naming %q", name, err, c.want)
		}
	}
}

const (
	previousDigest   = `"previous_digest": "sha256:2222222222222222222222222222222222222222222222222222222222222222"`
	previousRevision = `"previous_revision": "2222222222222222222222222222222222222222"`
)

// The previous digest and its revision come together: the service reports the revision of the digest it serves.
func TestThePreviousDigestNamesItsRevision(t *testing.T) {
	withDigest := strings.Replace(example(t), `"previous_digest": null`, previousDigest, 1)
	sameDigest := strings.ReplaceAll(withDigest, previousDigest, `"previous_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"`)
	for name, text := range map[string]string{
		"a digest without a revision":  withDigest,
		"a revision that is not one":   strings.Replace(withDigest, `"previous_revision": null`, `"previous_revision": "main"`, 1),
		"two revisions for one digest": strings.Replace(sameDigest, `"previous_revision": null`, previousRevision, 1),
	} {
		if _, err := Load(write(t, text)); err == nil || !strings.Contains(err.Error(), "release.previous_revision") {
			t.Errorf("%s: got %v, want a refusal naming release.previous_revision", name, err)
		}
	}
	same := strings.Replace(sameDigest, `"previous_revision": null`, `"previous_revision": "0000000000000000000000000000000000000000"`, 1)
	if _, err := Load(write(t, same)); err != nil {
		t.Errorf("the image's own digest and revision as the previous pair: %v", err)
	}
}

// A sample ratio of zero is present, not absent: it is valid and keeps no trace, as the binary reads it.
func TestASampleRatioOfZeroIsAValue(t *testing.T) {
	for _, zero := range []string{"0", "0.0"} {
		_, _, effective, err := Resolve(write(t, strings.Replace(example(t), `"sample_ratio": 1`, `"sample_ratio": `+zero, 1)))
		if err != nil {
			t.Fatalf("%s: %v", zero, err)
		}
		if !strings.Contains(string(effective), `"sample_ratio": `+zero) {
			t.Errorf("%s did not reach the effective document", zero)
		}
	}
}

// A hosted origin is a name a hosted service can reach, whatever its port.
// TestAnOriginThatIsNotHostedIsRefusedForThatReason holds every field exactOrigin serves to the same hosts, and names
// the reason, so that a host refused for something else entirely cannot stand in for this rule. The fullwidth and
// ideographic spellings are the ones net/http maps through IDNA to localhost and 127.0.0.1 before it dials.
func TestAnOriginThatIsNotHostedIsRefusedForThatReason(t *testing.T) {
	const notHosted, notASCII = "not a hosted origin", "in ASCII"
	origins := map[string]string{
		"https://[::1%25lo]:4318":                                             notHosted,
		"https://[::%25lo]:4318":                                              notHosted,
		"https://[::ffff:127.0.0.1%25lo]:4318":                                notHosted,
		"https://[fe80::1%25eth0]:4318":                                       notHosted,
		"https://[2001:db8::1%25eth0]:4318":                                   notHosted,
		"https://[fe80::1]:4318":                                              notHosted,
		"https://169.254.169.254":                                             notHosted,
		"https://[::ffff:169.254.169.254]":                                    notHosted,
		"https://10.0.0.1:4318":                                               notHosted,
		"https://172.16.0.1:4318":                                             notHosted,
		"https://192.168.1.1:4318":                                            notHosted,
		"https://[::ffff:10.0.0.1]:4318":                                      notHosted,
		"https://[fd00::1]:4318":                                              notHosted,
		"https://224.0.0.1:4318":                                              notHosted,
		"https://[ff02::1]:4318":                                              notHosted,
		"https://[::ffff:224.0.0.1]:4318":                                     notHosted,
		"https://[::ffff:255.255.255.255]:4318":                               notHosted,
		"https://[::ffff:0.0.0.0]:4318":                                       notHosted,
		"https://255.255.255.255:4318":                                        notHosted,
		"https://\uff4c\uff4f\uff43\uff41\uff4c\uff48\uff4f\uff53\uff54:4318": notASCII,
		"https://\uff11\uff12\uff17.\uff10.\uff10.\uff11:4318":                notASCII,
		"https://127\u30020\u30020\u30021:4318":                               notASCII,
		"https://%EF%BD%8Cocalhost:4318":                                      notASCII,
	}
	fields := map[string]string{
		"runtime.telemetry.endpoint": `"https://otel.example.invalid"`,
		"runtime.allowed_origin":     `"https://example-owner.github.example.invalid"`,
		"frontend.rpc_base_url":      `"https://bff.example.invalid"`,
	}
	base := example(t)
	for field, value := range fields {
		for origin, reason := range origins {
			text := strings.Replace(base, value, strconv.Quote(origin), 1)
			if text == base {
				t.Fatalf("%s: the mutation changed nothing", field)
			}
			_, err := Load(write(t, text))
			if err == nil || !strings.Contains(err.Error(), field) || !strings.Contains(err.Error(), reason) {
				t.Errorf("%s = %s: got %v, want a refusal naming the field and %q", field, origin, err, reason)
			}
		}
	}
}

func TestAnExternalHostStaysAccepted(t *testing.T) {
	for _, endpoint := range []string{"https://otel.example.invalid:4318", "https://xn--bcher-kva.example.invalid:4318", "https://203.0.113.7:4318", "https://[2001:db8::1]:4318"} {
		if _, err := Load(write(t, strings.Replace(example(t), `"https://otel.example.invalid"`, `"`+endpoint+`"`, 1))); err != nil {
			t.Errorf("%s: %v", endpoint, err)
		}
	}
}

func TestARollbackServesThePreviousDigest(t *testing.T) {
	text := strings.Replace(example(t), `"previous_digest": null`, previousDigest, 1)
	text = strings.Replace(text, `"previous_revision": null`, previousRevision, 1)
	text = strings.Replace(text, `"serving_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"`, `"serving_digest": "sha256:2222222222222222222222222222222222222222222222222222222222222222"`, 1)
	if _, err := Load(write(t, text)); err != nil {
		t.Fatalf("a rollback to the previous digest was refused: %v", err)
	}
}
