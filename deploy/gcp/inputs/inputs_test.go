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

func overlay(t *testing.T, name, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The later file wins per leaf, every other leaf keeps the base's value, and each leaf names the file that gave it.
func TestAnOverlayWinsPerLeafAndEveryLeafNamesItsSource(t *testing.T) {
	base := write(t, example(t))
	first := overlay(t, "operator.json", `{"bounds": {"max_instances": 3}, "release": {"cancel_in_flight": true}}`)
	second := overlay(t, "contract.json", `{"bounds": {"max_instances": 4}}`)
	in, provenance, effective, err := Resolve(base, first, second)
	if err != nil {
		t.Fatal(err)
	}
	if in.Bounds.MaxInstances != 4 || in.Bounds.Concurrency != 20 || !in.Release.CancelInFlight {
		t.Errorf("%+v %+v", in.Bounds, in.Release)
	}
	for path, want := range map[string]string{
		"bounds.max_instances": "contract.json", "release.cancel_in_flight": "operator.json",
		"bounds.concurrency": "inputs.json", "release.previous_digest": "inputs.json", "contract.project_id": "inputs.json",
	} {
		if provenance[path] != want {
			t.Errorf("%s: from %q, want %q", path, provenance[path], want)
		}
	}
	leaves, _ := shape()
	if len(provenance) != len(leaves) {
		t.Errorf("%d leaves with a source, the schema has %d", len(provenance), len(leaves))
	}
	// The effective document is the merged one, canonical, and reads back as the same inputs.
	again, err := Load(write(t, string(effective)))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, in) {
		t.Errorf("the effective document reads back differently:\n%+v\n%+v", again, in)
	}
	lines, err := Render(effective, provenance)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != len(leaves) || !contains(lines, `bounds.max_instances=4 # from contract.json`) ||
		!contains(lines, `release.previous_digest=null # from inputs.json`) ||
		!contains(lines, `contract.project_id="example-bff-hosted-000000" # from inputs.json`) {
		t.Errorf("rendered:\n%s", strings.Join(lines, "\n"))
	}
}

func contains(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

// A leaf given by no file is refused by name: a struct decode would have read its zero value silently.
func TestALeafGivenByNoFileIsRefusedByName(t *testing.T) {
	text := strings.Replace(example(t), ",\n    \"cancel_in_flight\": false", "", 1)
	if text == example(t) {
		t.Fatal("the mutation changed nothing")
	}
	_, err := Load(write(t, text))
	if err == nil || !strings.Contains(err.Error(), "release.cancel_in_flight: given by no file") {
		t.Errorf("got %v", err)
	}
	// Given by the overlay, it is complete again.
	in, provenance, _, err := Resolve(write(t, text), overlay(t, "operator.json", `{"release": {"cancel_in_flight": true}}`))
	if err != nil || !in.Release.CancelInFlight || provenance["release.cancel_in_flight"] != "operator.json" {
		t.Errorf("%v %+v %v", err, in, provenance)
	}
}

// A file is read in the schema's shape only, and each refusal names the file and the key.
func TestAnOverlayOutsideTheShapeIsRefusedNamingTheFile(t *testing.T) {
	base := write(t, example(t))
	cases := map[string]struct{ text, want string }{
		"a key outside the schema":   {`{"bounds": {"waiver": true}}`, "operator.json: bounds.waiver: not in the schema"},
		"a section given as a value": {`{"bounds": 5}`, "operator.json: bounds: is a section of the schema, not a value"},
		"a value given as a section": {`{"region": {"name": "x"}}`, "operator.json: region: is a value of the schema, not a section"},
		"not an object":              {`[1]`, "operator.json"},
		"not json":                   {`{`, "operator.json"},
	}
	for name, c := range cases {
		_, _, _, err := Resolve(base, overlay(t, "operator.json", c.text))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want a refusal naming %q", name, err, c.want)
		}
	}
	if _, _, _, err := Resolve(base, filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Error("an absent overlay was accepted")
	}
	// A wrong type reaches the struct decode, which names the leaf; validation then sees the merged document.
	_, _, _, err := Resolve(base, overlay(t, "operator.json", `{"bounds": {"max_instances": "two"}}`))
	if err == nil || !strings.Contains(err.Error(), "max_instances") {
		t.Errorf("got %v", err)
	}
	_, _, _, err = Resolve(base, overlay(t, "operator.json", `{"bounds": {"max_instances": 50}}`))
	if err == nil || !strings.Contains(err.Error(), "bounds.max_instances: 50") {
		t.Errorf("got %v", err)
	}
}

func TestRenderRefusesWhatIsNotADocument(t *testing.T) {
	if _, err := Render([]byte("{"), nil); err == nil {
		t.Error("accepted")
	}
}

func mainTF(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv("MAIN_TF")))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// The cost bounds are typed inputs, enumerated in the plan (spec 012 FR-010): every bound the service consumes
// is read from the funnel, the budget alert is the infrastructure owner's and appears in no resource here, and
// the schema itself says the alert is never a cap.
func TestTheCostBoundsAreEnumeratedInThePlan(t *testing.T) {
	var fields []string
	typ := reflect.TypeOf(Bounds{})
	for i := 0; i < typ.NumField(); i++ {
		fields = append(fields, typ.Field(i).Tag.Get("json"))
	}
	want := []string{"max_instances", "concurrency", "request_timeout_seconds", "budget_alert_amount", "budget_alert_currency"}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("the bounds are %v, the enumeration knows %v", fields, want)
	}
	plan := mainTF(t)
	for _, consumed := range []string{"max_instances", "concurrency", "request_timeout_seconds"} {
		if !strings.Contains(plan, "local.inputs.bounds."+consumed) {
			t.Errorf("the plan does not read bounds.%s from the funnel", consumed)
		}
	}
	for _, owner := range []string{"budget_alert_amount", "budget_alert_currency"} {
		if strings.Contains(plan, owner) {
			t.Errorf("the plan consumes %s; the budget alert is the infrastructure owner's, declared against the contract", owner)
		}
	}
	if strings.Contains(plan, "cap") {
		t.Error("the plan speaks of a cap; no bound here is a monetary cap")
	}
	schema, err := os.ReadFile(filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv("SCHEMA")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), "never a cap") {
		t.Error("the schema does not say the budget alert is never a cap")
	}
}

// The identities and bindings are enumerated (spec 012 FR-004): the service runs as the contract's runtime
// identity, the one binding is the public invoker, no other role or member is granted, and the build identity
// is granted nothing here, so the runtime identity holds no deploy or build permission from this module.
func TestTheIdentitiesAndBindingsAreEnumerated(t *testing.T) {
	plan := mainTF(t)
	if strings.Count(plan, "\nresource \"") != 2 {
		t.Fatalf("the plan declares %d resources, not the two enumerated", strings.Count(plan, "\nresource \""))
	}
	if !strings.Contains(plan, "service_account                  = local.inputs.contract.runtime_identity") {
		t.Error("the service does not run as the contract's runtime identity")
	}
	for _, line := range strings.Split(plan, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "role "):
			if trimmed != `role     = "roles/run.invoker"` {
				t.Errorf("an unenumerated role: %s", trimmed)
			}
		case strings.HasPrefix(trimmed, "member "):
			if trimmed != `member   = "allUsers"` {
				t.Errorf("an unenumerated member: %s", trimmed)
			}
		}
	}
	bindings := 0
	for _, line := range strings.Split(plan, "\n") {
		if strings.HasPrefix(line, "resource \"") && strings.Contains(line, "iam") {
			bindings++
			if line != `resource "google_cloud_run_v2_service_iam_member" "public_invoker" {` {
				t.Errorf("an unenumerated binding: %s", line)
			}
		}
	}
	if bindings != 1 {
		t.Errorf("%d bindings declared, not the one public invoker", bindings)
	}
	for _, absent := range []string{"build_identity", "roles/run.admin", "roles/owner", "roles/editor", "serviceAccountUser", "iam_binding", "iam_policy"} {
		if strings.Contains(plan, absent) {
			t.Errorf("the plan mentions %s; the module grants nothing beyond the public invoker", absent)
		}
	}
}

// The contract the infrastructure owner publishes is the one the funnel consumes (spec 012 T002, R1): the
// published schema's fields are the struct's, the inputs schema references it rather than restating it, and
// the version is the one supported.
func TestTheContractSchemaIsTheOneTheFunnelConsumes(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv("CONTRACT_SCHEMA")))
	if err != nil {
		t.Fatal(err)
	}
	var published struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &published); err != nil {
		t.Fatal(err)
	}
	var fields []string
	typ := reflect.TypeOf(Contract{})
	for i := 0; i < typ.NumField(); i++ {
		fields = append(fields, typ.Field(i).Tag.Get("json"))
	}
	if !reflect.DeepEqual(fields, published.Required) {
		t.Errorf("struct fields %v, the published contract requires %v", fields, published.Required)
	}
	for _, field := range fields {
		if _, ok := published.Properties[field]; !ok {
			t.Errorf("the published contract lacks %s", field)
		}
	}
	if !strings.Contains(string(published.Properties["version"]), `"const": 1`) {
		t.Error("the published contract's version is not the one supported")
	}
	inputs, err := os.ReadFile(filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv("SCHEMA")))
	if err != nil {
		t.Fatal(err)
	}
	var funnel struct {
		Properties struct {
			Contract struct {
				Ref string `json:"$ref"`
			} `json:"contract"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(inputs, &funnel); err != nil {
		t.Fatal(err)
	}
	if funnel.Properties.Contract.Ref != "contract.schema.json" {
		t.Errorf("the inputs schema restates the contract instead of referencing it: %q", funnel.Properties.Contract.Ref)
	}
}
