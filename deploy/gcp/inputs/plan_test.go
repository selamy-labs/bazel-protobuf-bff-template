package inputs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

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
