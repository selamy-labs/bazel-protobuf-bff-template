package inputs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func checkEffectiveBounds(t *testing.T, effectiveJSON []byte, in *Inputs) {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal(effectiveJSON, &raw); err != nil {
		t.Fatal(err)
	}
	bounds, ok := raw["bounds"].(map[string]any)
	if !ok {
		t.Fatal("bounds section missing from effective JSON")
	}
	for _, consumed := range []string{"max_instances", "concurrency", "request_timeout_seconds", "budget_alert_amount", "budget_alert_currency"} {
		if _, ok := bounds[consumed]; !ok {
			t.Errorf("effective JSON does not contain bounds.%s", consumed)
		}
	}
	if in.Bounds.MaxInstances <= 0 || in.Bounds.Concurrency <= 0 || in.Bounds.RequestTimeoutSeconds <= 0 {
		t.Errorf("invalid bounds in resolved inputs: %+v", in.Bounds)
	}
}

// The cost bounds are typed inputs, enumerated in the plan inputs contract (spec 012 FR-010): every bound the service consumes
// is resolved from the funnel, the budget alert is the infrastructure owner's and appears in the schema, and
// the schema itself says the alert is never a cap. Module wiring and bounds are tested natively in service.tftest.hcl.
func TestTheCostBoundsAreEnumeratedInTheInputsContract(t *testing.T) {
	var fields []string
	typ := reflect.TypeOf(Bounds{})
	for i := 0; i < typ.NumField(); i++ {
		fields = append(fields, typ.Field(i).Tag.Get("json"))
	}
	want := []string{"max_instances", "concurrency", "request_timeout_seconds", "budget_alert_amount", "budget_alert_currency"}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("the bounds are %v, the enumeration knows %v", fields, want)
	}

	in, _, effectiveJSON, err := Resolve(findTestFile("EXAMPLE", "example.inputs.json", filepath.Join("deploy", "gcp", "inputs", "example.inputs.json")))
	if err != nil {
		t.Fatal(err)
	}
	checkEffectiveBounds(t, effectiveJSON, in)

	schema, err := os.ReadFile(findTestFile("SCHEMA", "inputs.schema.json", filepath.Join("deploy", "gcp", "inputs", "inputs.schema.json")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), "never a cap") {
		t.Error("the schema does not say the budget alert is never a cap")
	}
}

// The identities contract is validated from the funnel (spec 012 FR-004): the contract specifies
// distinct runtime and build identities, so the runtime identity holds no build or deploy permission.
// Module-level IAM bindings and resource inventories are verified natively in service.tftest.hcl and offline.sh.
func TestTheIdentitiesAreEnumeratedInTheContract(t *testing.T) {
	in, _, effectiveJSON, err := Resolve(findTestFile("EXAMPLE", "example.inputs.json", filepath.Join("deploy", "gcp", "inputs", "example.inputs.json")))
	if err != nil {
		t.Fatal(err)
	}
	if in.Contract.RuntimeIdentity == "" {
		t.Error("the runtime identity must be present in the contract")
	}
	if in.Contract.BuildIdentity == "" {
		t.Error("the build identity must be present in the contract")
	}
	if in.Contract.RuntimeIdentity == in.Contract.BuildIdentity {
		t.Error("the runtime identity and build identity must be distinct; runtime must hold no build or deploy permission")
	}

	var raw map[string]any
	if err := json.Unmarshal(effectiveJSON, &raw); err != nil {
		t.Fatal(err)
	}
	contract, ok := raw["contract"].(map[string]any)
	if !ok {
		t.Fatal("the contract section is missing from effective JSON")
	}
	if contract["runtime_identity"] != in.Contract.RuntimeIdentity {
		t.Errorf("contract runtime_identity in JSON = %v, want %s", contract["runtime_identity"], in.Contract.RuntimeIdentity)
	}
	if contract["build_identity"] != in.Contract.BuildIdentity {
		t.Errorf("contract build_identity in JSON = %v, want %s", contract["build_identity"], in.Contract.BuildIdentity)
	}
}

func verifyPublishedContract(t *testing.T, raw []byte) {
	t.Helper()
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
}

func verifyInputsContractRef(t *testing.T, inputs []byte) {
	t.Helper()
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

// The contract the infrastructure owner publishes is the one the funnel consumes (spec 012 T002, R1): the
// published schema's fields are the struct's, the inputs schema references it rather than restating it, and
// the version is the one supported.
func TestTheContractSchemaIsTheOneTheFunnelConsumes(t *testing.T) {
	raw, err := os.ReadFile(findTestFile("CONTRACT_SCHEMA", "contract.schema.json", filepath.Join("deploy", "gcp", "inputs", "contract.schema.json")))
	if err != nil {
		t.Fatal(err)
	}
	verifyPublishedContract(t, raw)

	inputs, err := os.ReadFile(findTestFile("SCHEMA", "inputs.schema.json", filepath.Join("deploy", "gcp", "inputs", "inputs.schema.json")))
	if err != nil {
		t.Fatal(err)
	}
	verifyInputsContractRef(t, inputs)
}
