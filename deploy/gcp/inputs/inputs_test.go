package inputs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
	for _, value := range []string{in.Contract.ProjectID, in.Contract.RuntimeIdentity, in.Project.OwnerGroup, in.Runtime.Verifier.Issuer, in.Frontend.RPCBaseURL} {
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
		"same identities":           {`"build_identity": "bff-build@example-bff-hosted-000000.iam.example.invalid"`, `"build_identity": "bff-runtime@example-bff-hosted-000000.iam.example.invalid"`, "runtime must hold no build"},
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
		"bad runtime identity":      {`"runtime_identity": "bff-runtime@example-bff-hosted-000000.iam.example.invalid"`, `"runtime_identity": "not-an-identity"`, "contract.runtime_identity"},
		"bad build identity":        {`"build_identity": "bff-build@example-bff-hosted-000000.iam.example.invalid"`, `"build_identity": "nope"`, "contract.build_identity"},
		"missing registry":          {`"artifact_registry": "example-region1-docker.example.invalid/example-bff-hosted-000000/bff"`, `"artifact_registry": ""`, "contract.artifact_registry"},
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

func TestARollbackServesThePreviousDigest(t *testing.T) {
	text := strings.Replace(example(t), `"previous_digest": null`, `"previous_digest": "sha256:2222222222222222222222222222222222222222222222222222222222222222"`, 1)
	text = strings.Replace(text, `"serving_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"`, `"serving_digest": "sha256:2222222222222222222222222222222222222222222222222222222222222222"`, 1)
	if _, err := Load(write(t, text)); err != nil {
		t.Fatalf("a rollback to the previous digest was refused: %v", err)
	}
}
