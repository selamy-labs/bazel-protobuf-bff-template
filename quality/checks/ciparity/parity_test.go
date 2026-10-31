package ciparity

import (
	"strings"
	"testing"
)

const sha = "0123456789abcdef0123456789abcdef01234567"

func policy() Policy {
	return Policy{
		Actions:           map[string]ActionRule{"actions/checkout": {SHA: sha, With: map[string]any{"ref": "x"}}},
		On:                "push",
		Concurrency:       "c",
		RunsOn:            []string{"ubuntu-24.04"},
		MaxTimeoutMinutes: 45,
		Runs: []RunRule{
			{Body: "tools/verify.sh"},
			{Body: "gate body", GateOnly: true},
		},
		RequiredRun:  "tools/verify.sh",
		RequiredJobs: []string{"verify"},
		Gate:         "gate",
	}
}

func workflow() map[string]any {
	return map[string]any{
		"on":          "push",
		"concurrency": "c",
		"permissions": map[string]any{"contents": "read"},
		"jobs": map[string]any{
			"verify": map[string]any{
				"runs-on":         "ubuntu-24.04",
				"timeout-minutes": 45.0,
				"steps": []any{
					map[string]any{"uses": "actions/checkout@" + sha, "with": map[string]any{"ref": "x"}},
					map[string]any{"run": "tools/verify.sh\n"},
				},
			},
			"gate": map[string]any{
				"if":              "always()",
				"needs":           []any{"verify"},
				"runs-on":         "ubuntu-24.04",
				"timeout-minutes": 5.0,
				"permissions":     map[string]any{},
				"steps":           []any{map[string]any{"run": "gate body"}},
			},
		},
	}
}

func TestConformingWorkflowPasses(t *testing.T) {
	if errs := Validate(workflow(), policy()); len(errs) != 0 {
		t.Fatalf("got %v, want no violations", errs)
	}
}

func TestEmptyWorkflowFails(t *testing.T) {
	if errs := Validate(map[string]any{}, policy()); len(errs) == 0 {
		t.Fatal("an empty workflow must not pass")
	}
}

func TestVerifierMustRunExactlyOnce(t *testing.T) {
	wf := workflow()
	job := wf["jobs"].(map[string]any)["verify"].(map[string]any)
	job["steps"] = append(job["steps"].([]any), map[string]any{"run": "tools/verify.sh"})
	errs := Validate(wf, policy())
	if len(errs) != 1 || !strings.Contains(errs[0], "exactly once") {
		t.Fatalf("got %v", errs)
	}
}

func TestGateBodyIsNotPermittedInVerifyJob(t *testing.T) {
	wf := workflow()
	job := wf["jobs"].(map[string]any)["verify"].(map[string]any)
	job["steps"] = append(job["steps"].([]any), map[string]any{"run": "gate body"})
	if errs := Validate(wf, policy()); len(errs) == 0 {
		t.Fatal("gate-only body accepted in verify job")
	}
}

func TestNonBooleanStyleUsesAndRunTogetherFail(t *testing.T) {
	wf := workflow()
	job := wf["jobs"].(map[string]any)["verify"].(map[string]any)
	job["steps"].([]any)[1].(map[string]any)["uses"] = "actions/checkout@" + sha
	if errs := Validate(wf, policy()); len(errs) == 0 {
		t.Fatal("a step with both uses and run was accepted")
	}
}
