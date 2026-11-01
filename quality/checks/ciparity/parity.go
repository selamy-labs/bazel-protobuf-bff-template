// Package ciparity validates a GitHub Actions workflow against an allowlist policy so that CI
// can only run the canonical verifier and report one aggregate verdict (spec 002 T024).
//
// The validator is deliberately closed: every key it does not recognize is an error, so a new
// way to skip, soften or redirect a step fails instead of passing unnoticed.
package ciparity

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Policy is the repository-specific allowlist. Structural rules are fixed in code.
type Policy struct {
	// Actions maps "owner/repo" to the only permitted commit and exact `with:` inputs. Inputs are
	// exact because an action input can redirect a run as easily as a shell can.
	Actions map[string]ActionRule `json:"actions"`
	// On and Concurrency must equal the workflow's values exactly.
	On          any `json:"on"`
	Concurrency any `json:"concurrency"`
	// RunsOn is the set of permitted runner labels.
	RunsOn []string `json:"runs_on"`
	// MaxTimeoutMinutes bounds every job.
	MaxTimeoutMinutes float64 `json:"max_timeout_minutes"`
	// Runs are the only permitted `run:` bodies, compared after trimming surrounding whitespace.
	Runs []RunRule `json:"runs"`
	// RequiredRun must appear exactly once in every job except the gate. {job} expands to the job.
	RequiredRun string `json:"required_run"`
	// JobSteps is each job's exact ordered step list: "uses:<owner/repo>" or "run:<body>".
	JobSteps map[string][]string `json:"job_steps"`
	// RequiredJobs must exist (besides the gate).
	RequiredJobs []string `json:"required_jobs"`
	// Gate is the aggregate job's name.
	Gate string `json:"gate"`
}

// ActionRule pins one action.
type ActionRule struct {
	SHA  string         `json:"sha"`
	With map[string]any `json:"with"`
}

// RunRule is one permitted run body and the exact env it may carry.
type RunRule struct {
	Body     string            `json:"body"`
	Env      map[string]string `json:"env"`
	GateOnly bool              `json:"gate_only"`
}

var (
	topKeys  = set("name", "on", "permissions", "concurrency", "jobs")
	jobKeys  = set("name", "runs-on", "timeout-minutes", "steps", "needs", "if", "permissions")
	stepKeys = set("name", "id", "uses", "with", "run", "env", "if")
	fullSHA  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	always   = regexp.MustCompile(`^(\$\{\{\s*)?always\(\)(\s*\}\})?$`)
)

func set(keys ...string) map[string]bool {
	m := map[string]bool{}
	for _, k := range keys {
		m[k] = true
	}
	return m
}

// Validate returns every violation found, sorted for stable output. An empty result means the
// workflow conforms.
func Validate(wf map[string]any, p Policy, legs []string) []string {
	v := &validator{p: p, legs: legs}
	v.workflow(wf)
	sort.Strings(v.errs)
	return v.errs
}

type validator struct {
	p       Policy
	legs    []string
	current string
	errs    []string
}

func (v *validator) fail(format string, args ...any) {
	v.errs = append(v.errs, fmt.Sprintf(format, args...))
}

func (v *validator) unknownKeys(where string, m map[string]any, allowed map[string]bool) {
	for k := range m {
		if !allowed[k] {
			v.fail("%s: key %q is not permitted (closed allowlist; e.g. continue-on-error, defaults, env, shell, working-directory, container)", where, k)
		}
	}
}

func (v *validator) workflow(wf map[string]any) {
	v.unknownKeys("workflow", wf, topKeys)
	v.readOnlyPermissions("workflow", wf["permissions"], false)
	if !reflect.DeepEqual(wf["on"], v.p.On) {
		v.fail("workflow: on triggers %v are not the permitted triggers", wf["on"])
	}
	if !reflect.DeepEqual(wf["concurrency"], v.p.Concurrency) {
		v.fail("workflow: concurrency %v is not the permitted setting", wf["concurrency"])
	}
	jobs, ok := wf["jobs"].(map[string]any)
	if !ok || len(jobs) == 0 {
		v.fail("workflow: jobs are missing")
		return
	}
	gate, ok := jobs[v.p.Gate].(map[string]any)
	if !ok {
		v.fail("workflow: aggregate job %q is missing", v.p.Gate)
	}
	for _, name := range v.p.RequiredJobs {
		if _, ok := jobs[name]; !ok {
			v.fail("workflow: required job %q (the verify job) is missing", name)
		}
	}
	if !sameSet(v.p.RequiredJobs, v.legs) {
		v.fail("policy: required jobs %v are not the registry's legs %v", v.p.RequiredJobs, v.legs)
	}
	for _, rule := range v.p.Runs {
		if want, ok := rule.Env["EXPECTED_LEGS"]; ok && want != strconv.Itoa(len(v.legs)) {
			v.fail("policy: the gate expects %s results, and the registry has %d legs", want, len(v.legs))
		}
	}
	var others []string
	for name, raw := range jobs {
		job, ok := raw.(map[string]any)
		if !ok {
			v.fail("job %s: not a mapping", name)
			continue
		}
		if name != v.p.Gate {
			others = append(others, name)
		}
		v.job(name, job, name == v.p.Gate)
	}
	if !sameSet(others, v.legs) {
		v.fail("workflow: jobs %v are not the registry's legs %v", others, v.legs)
	}
	if gate != nil {
		v.gateNeeds(gate, others)
	}
}

func sameSet(a, b []string) bool {
	a, b = slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b))
	return slices.Equal(a, b)
}

func (v *validator) readOnlyPermissions(where string, raw any, allowEmpty bool) {
	m, ok := raw.(map[string]any)
	if !ok {
		v.fail("%s: permissions must be an explicit mapping", where)
		return
	}
	if len(m) == 0 && !allowEmpty {
		v.fail("%s: permissions must grant contents: read", where)
	}
	for scope, level := range m {
		if level != "read" && level != "none" {
			v.fail("%s: permissions %s: %v is not read-only", where, scope, level)
		}
	}
}

func (v *validator) job(name string, job map[string]any, isGate bool) {
	v.current = name
	where := "job " + name
	if _, ok := job["uses"]; ok {
		v.fail("%s: reusable workflow jobs (uses) are not permitted", where)
	}
	v.unknownKeys(where, job, jobKeys)
	if cond, ok := job["if"]; ok {
		s, _ := cond.(string)
		if !isGate || !always.MatchString(strings.TrimSpace(s)) {
			v.fail("%s: if %q is not permitted (only the gate may use if: always())", where, s)
		}
	} else if isGate {
		v.fail("%s: the gate must use if: always()", where)
	}
	if perms, ok := job["permissions"]; ok {
		v.readOnlyPermissions(where, perms, true)
	}
	v.runner(where, job)
	steps, _ := job["steps"].([]any)
	if len(steps) == 0 {
		v.fail("%s: has no steps", where)
	}
	required := 0
	var order []string
	for i, raw := range steps {
		step, ok := raw.(map[string]any)
		if !ok {
			v.fail("%s step %d: not a mapping", where, i)
			continue
		}
		if v.step(fmt.Sprintf("%s step %d", where, i), step, isGate) {
			required++
		}
		order = append(order, signature(step))
	}
	if want, ok := v.p.JobSteps[name]; !ok {
		v.fail("%s: has no permitted step order", where)
	} else if !reflect.DeepEqual(order, want) {
		v.fail("%s: step order %q is not the permitted step order %q", where, order, want)
	}
	if !isGate && required != 1 {
		v.fail("%s: must run %q exactly once, found %d", where, v.p.RequiredRun, required)
	}
}

func (v *validator) runner(where string, job map[string]any) {
	label, _ := job["runs-on"].(string)
	permitted := false
	for _, l := range v.p.RunsOn {
		permitted = permitted || l == label
	}
	if !permitted {
		v.fail("%s: runs-on %v is not a permitted runner", where, job["runs-on"])
	}
	if m, ok := job["timeout-minutes"].(float64); !ok || m <= 0 || m > v.p.MaxTimeoutMinutes {
		v.fail("%s: timeout-minutes %v must be a number in (0, %v]", where, job["timeout-minutes"], v.p.MaxTimeoutMinutes)
	}
}

// signature identifies a step for the ordered comparison.
func signature(step map[string]any) string {
	if uses, ok := step["uses"].(string); ok {
		repo, _, _ := strings.Cut(uses, "@")
		return "uses:" + repo
	}
	run, _ := step["run"].(string)
	return "run:" + strings.TrimSpace(run)
}

// step validates one step and reports whether it is the required verifier run.
func (v *validator) step(where string, step map[string]any, isGate bool) bool {
	v.unknownKeys(where, step, stepKeys)
	for _, key := range []string{"env", "with"} {
		if value, ok := step[key]; ok {
			if _, isMap := value.(map[string]any); !isMap {
				v.fail("%s: %s must be a literal mapping, not %v", where, key, value)
			}
		}
	}
	uses, hasUses := step["uses"].(string)
	run, hasRun := step["run"].(string)
	if hasUses == hasRun {
		v.fail("%s: must have exactly one of uses or run", where)
		return false
	}
	if cond, ok := step["if"]; ok {
		s, _ := cond.(string)
		if hasRun || !always.MatchString(strings.TrimSpace(s)) {
			v.fail("%s: if %q is not permitted (only action steps may use if: always())", where, s)
		}
	}
	if hasUses {
		v.action(where, uses, step)
		return false
	}
	return v.run(where, strings.TrimSpace(run), step, isGate)
}

func (v *validator) action(where, uses string, step map[string]any) {
	if _, ok := step["env"]; ok {
		v.fail("%s: env is not permitted on action steps", where)
	}
	repo, ref, ok := strings.Cut(uses, "@")
	if !ok || !fullSHA.MatchString(ref) {
		v.fail("%s: uses %q must be pinned to a full commit SHA", where, uses)
		return
	}
	rule, listed := v.p.Actions[repo]
	if !listed {
		v.fail("%s: uses %q is not an allowlisted action", where, uses)
		return
	}
	if ref != rule.SHA {
		v.fail("%s: uses %q is not the allowlisted commit %s", where, uses, rule.SHA)
	}
	with, _ := step["with"].(map[string]any)
	if len(with) == 0 {
		with = nil
	}
	want := rule.With
	if len(want) == 0 {
		want = nil
	}
	if !reflect.DeepEqual(with, want) {
		v.fail("%s: %s with inputs %v must be exactly %v (exact ref, no persisted credentials)", where, repo, with, want)
	}
}

func (v *validator) run(where, body string, step map[string]any, isGate bool) bool {
	env, _ := step["env"].(map[string]any)
	for _, rule := range v.p.Runs {
		if strings.TrimSpace(rule.Body) != body {
			continue
		}
		if rule.GateOnly != isGate {
			v.fail("%s: run body is only permitted in the %s job", where, map[bool]string{true: "gate", false: "verify"}[rule.GateOnly])
		}
		if len(env) != len(rule.Env) {
			v.fail("%s: env %v is not the permitted env %v", where, env, rule.Env)
		}
		for k, want := range rule.Env {
			if fmt.Sprint(env[k]) != want {
				v.fail("%s: env %s must be %q", where, k, want)
			}
		}
		return body == strings.TrimSpace(strings.ReplaceAll(v.p.RequiredRun, "{job}", v.current))
	}
	first, _, _ := strings.Cut(body, "\n")
	v.fail("%s: run body is not allowlisted (first line %q)", where, first)
	return false
}

func (v *validator) gateNeeds(gate map[string]any, others []string) {
	have := map[string]bool{}
	switch n := gate["needs"].(type) {
	case []any:
		for _, x := range n {
			s, _ := x.(string)
			have[s] = true
		}
	case string:
		have[n] = true
	}
	for _, name := range others {
		if !have[name] {
			v.fail("job %s: gate needs must include job %q", v.p.Gate, name)
		}
	}
	for name := range have {
		found := false
		for _, o := range others {
			found = found || o == name
		}
		if !found {
			v.fail("job %s: needs %q names no job", v.p.Gate, name)
		}
	}
}
