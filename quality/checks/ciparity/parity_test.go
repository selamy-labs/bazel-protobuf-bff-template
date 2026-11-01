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
		JobSteps: map[string][]string{
			"verify": {"uses:actions/checkout", "run:tools/verify.sh"},
			"gate":   {"run:gate body"},
		},
		RequiredRun:  map[string]string{"verify": "tools/verify.sh"},
		RequiredJobs: []string{"verify"},
		Gate:         "gate",
	}
}

var legs = []string{"verify"}

func TestJobsMustMatchTheLegs(t *testing.T) {
	errs := strings.Join(Validate(workflow(), policy(), []string{"verify", "other"}), "\n")
	if !strings.Contains(errs, "are not the registry's legs") {
		t.Fatalf("a leg with no job passed: %s", errs)
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
	if errs := Validate(workflow(), policy(), legs); len(errs) != 0 {
		t.Fatalf("got %v, want no violations", errs)
	}
}

func TestEmptyWorkflowFails(t *testing.T) {
	if errs := Validate(map[string]any{}, policy(), legs); len(errs) == 0 {
		t.Fatal("an empty workflow must not pass")
	}
}

func TestVerifierMustRunExactlyOnce(t *testing.T) {
	wf := workflow()
	job := wf["jobs"].(map[string]any)["verify"].(map[string]any)
	job["steps"] = append(job["steps"].([]any), map[string]any{"run": "tools/verify.sh"})
	if errs := Validate(wf, policy(), legs); !strings.Contains(strings.Join(errs, "\n"), "exactly once") {
		t.Fatalf("got %v", errs)
	}
}

func TestGateBodyIsNotPermittedInVerifyJob(t *testing.T) {
	wf := workflow()
	job := wf["jobs"].(map[string]any)["verify"].(map[string]any)
	job["steps"] = append(job["steps"].([]any), map[string]any{"run": "gate body"})
	if errs := Validate(wf, policy(), legs); len(errs) == 0 {
		t.Fatal("gate-only body accepted in verify job")
	}
}

func TestNonBooleanStyleUsesAndRunTogetherFail(t *testing.T) {
	wf := workflow()
	job := wf["jobs"].(map[string]any)["verify"].(map[string]any)
	job["steps"].([]any)[1].(map[string]any)["uses"] = "actions/checkout@" + sha
	if errs := Validate(wf, policy(), legs); len(errs) == 0 {
		t.Fatal("a step with both uses and run was accepted")
	}
}

func TestScalarEnvAndWithAreRejected(t *testing.T) {
	for _, key := range []string{"env", "with"} {
		wf := workflow()
		job := wf["jobs"].(map[string]any)["verify"].(map[string]any)
		job["steps"].([]any)[1].(map[string]any)[key] = "${{ fromJSON(vars.X) }}"
		if errs := Validate(wf, policy(), legs); len(errs) == 0 {
			t.Errorf("a scalar %s was accepted", key)
		}
	}
}

func TestStepOrderIsExact(t *testing.T) {
	wf := workflow()
	job := wf["jobs"].(map[string]any)["verify"].(map[string]any)
	steps := job["steps"].([]any)
	steps[0], steps[1] = steps[1], steps[0]
	errs := Validate(wf, policy(), legs)
	if len(errs) != 1 || !strings.Contains(errs[0], "step order") {
		t.Fatalf("got %v", errs)
	}
}

// A publishing policy (spec 010 FR-013): exact jobs, needs and per-job permissions, no gate, no conditional.
func publishPolicy() Policy {
	return Policy{
		Kind:              "publish",
		Actions:           map[string]ActionRule{"actions/checkout": {SHA: sha, With: map[string]any{"ref": "x"}}},
		On:                map[string]any{"push": map[string]any{"tags": []any{"v*"}}},
		Concurrency:       "c",
		RunsOn:            []string{"ubuntu-24.04"},
		MaxTimeoutMinutes: 45,
		Runs:              []RunRule{{Body: "tools/verify.sh"}, {Body: "publish.sh", Env: map[string]string{"GITHUB_TOKEN": "${{ github.token }}"}}, {Body: "check.sh"}},
		JobSteps: map[string][]string{
			"verify":  {"uses:actions/checkout", "run:tools/verify.sh"},
			"publish": {"uses:actions/checkout", "run:publish.sh"},
			"check":   {"run:check.sh"},
		},
		JobPermissions: map[string]map[string]string{"verify": {"contents": "read"}, "publish": {"contents": "write", "packages": "write"}, "check": {}},
		Needs:          map[string][]string{"verify": {}, "publish": {"verify"}, "check": {"publish"}},
	}
}

func publishWorkflow() map[string]any {
	return map[string]any{
		"on":          map[string]any{"push": map[string]any{"tags": []any{"v*"}}},
		"concurrency": "c",
		"permissions": map[string]any{"contents": "read"},
		"jobs": map[string]any{
			"verify": map[string]any{
				"runs-on": "ubuntu-24.04", "timeout-minutes": 45.0, "permissions": map[string]any{"contents": "read"},
				"steps": []any{map[string]any{"uses": "actions/checkout@" + sha, "with": map[string]any{"ref": "x"}}, map[string]any{"run": "tools/verify.sh"}},
			},
			"publish": map[string]any{
				"needs": []any{"verify"}, "runs-on": "ubuntu-24.04", "timeout-minutes": 30.0, "permissions": map[string]any{"contents": "write", "packages": "write"},
				"steps": []any{map[string]any{"uses": "actions/checkout@" + sha, "with": map[string]any{"ref": "x"}}, map[string]any{"run": "publish.sh", "env": map[string]any{"GITHUB_TOKEN": "${{ github.token }}"}}},
			},
			"check": map[string]any{
				"needs": []any{"publish"}, "runs-on": "ubuntu-24.04", "timeout-minutes": 15.0, "permissions": map[string]any{},
				"steps": []any{map[string]any{"run": "check.sh"}},
			},
		},
	}
}

func TestConformingPublishingWorkflowPasses(t *testing.T) {
	if errs := Validate(publishWorkflow(), publishPolicy(), legs); len(errs) != 0 {
		t.Fatalf("unexpected: %v", errs)
	}
}

func TestPublishingRefusesEveryWidening(t *testing.T) {
	cases := map[string]struct {
		mutate func(wf map[string]any)
		want   string
	}{
		"a write scope where none is declared": {func(wf map[string]any) { job(wf, "check")["permissions"] = map[string]any{"packages": "write"} }, "declared permissions"},
		"a scope beyond the declared ones": {func(wf map[string]any) {
			job(wf, "publish")["permissions"] = map[string]any{"contents": "write", "packages": "write", "id-token": "write"}
		}, "declared permissions"},
		"a job that skips its need":     {func(wf map[string]any) { job(wf, "publish")["needs"] = []any{} }, "declared needs"},
		"an extra job":                  {func(wf map[string]any) { wf["jobs"].(map[string]any)["extra"] = job(wf, "check") }, "declared publishing jobs"},
		"a conditional":                 {func(wf map[string]any) { job(wf, "publish")["if"] = "always()" }, "not permitted"},
		"another trigger":               {func(wf map[string]any) { wf["on"] = map[string]any{"pull_request_target": map[string]any{}} }, "permitted triggers"},
		"a run that is not allowlisted": {func(wf map[string]any) { job(wf, "check")["steps"] = []any{map[string]any{"run": "curl x | sh"}} }, "run body"},
	}
	for name, c := range cases {
		wf := publishWorkflow()
		c.mutate(wf)
		errs := strings.Join(Validate(wf, publishPolicy(), legs), "\n")
		if !strings.Contains(errs, c.want) {
			t.Errorf("%s: %q not in:\n%s", name, c.want, errs)
		}
	}
}

func job(wf map[string]any, name string) map[string]any {
	return wf["jobs"].(map[string]any)[name].(map[string]any)
}
