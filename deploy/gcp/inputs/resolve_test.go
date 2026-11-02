package inputs

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

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
	leaves, _, _ := shape()
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

// A struct decode reads null as a zero value, so a required leaf given as null would pass as false, zero or empty
// while the effective document kept the null. Only the two leaves the schema declares nullable may be null.
func TestNullIsNotAValue(t *testing.T) {
	for leaf, old := range map[string]string{
		"runtime.telemetry.sample_ratio": `"sample_ratio": 1`,
		"release.cancel_in_flight":       `"cancel_in_flight": false`,
		"bounds.max_instances":           `"max_instances": 2`,
		"contract.notes_bucket":          `"notes_bucket": "example-bff-hosted-000000-notes"`,
		"runtime.port":                   `"port": 8080`,
	} {
		key, _, _ := strings.Cut(old, ":")
		_, err := Load(write(t, strings.Replace(example(t), old, key+": null", 1)))
		if err == nil || !strings.Contains(err.Error(), leaf+": null") {
			t.Errorf("%s: got %v, want a refusal naming the leaf", leaf, err)
		}
	}
	if _, err := Load(write(t, example(t))); err != nil {
		t.Errorf("the example's nullable leaves are null: %v", err)
	}
}

// Null is judged on the effective value: an overlay may not introduce one, and it may replace one.
func TestNullIsJudgedAfterTheOverlays(t *testing.T) {
	_, _, _, err := Resolve(write(t, example(t)), overlay(t, "operator.json", `{"runtime": {"telemetry": {"sample_ratio": null}}}`))
	if err == nil || !strings.Contains(err.Error(), "runtime.telemetry.sample_ratio: null") || !strings.Contains(err.Error(), "operator.json") {
		t.Errorf("an overlay's null: got %v, want a refusal naming the leaf and the file", err)
	}
	base := write(t, strings.Replace(example(t), `"sample_ratio": 1`, `"sample_ratio": null`, 1))
	in, _, _, err := Resolve(base, overlay(t, "operator.json", `{"runtime": {"telemetry": {"sample_ratio": 0}}}`))
	if err != nil || in.Runtime.Telemetry.SampleRatio != 0 {
		t.Errorf("an overlay replacing a null: %v", err)
	}
}
