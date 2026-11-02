package inputs

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// serviceEnvironment is what the module declares, by the funnel's values: the mock tests pin each expression, and
// TestTheDeclaredEnvironmentIsTheOneThisWitnessStarts keeps this table equal to the module's names.
func serviceEnvironment(in *Inputs) map[string]string {
	return map[string]string{
		"BFF_ALLOWED_ORIGIN":    in.Runtime.AllowedOrigin,
		"BFF_VERIFIER_ISSUER":   in.Runtime.Verifier.Issuer,
		"BFF_VERIFIER_AUDIENCE": in.Runtime.Verifier.Audience,
		"BFF_VERIFIER_JWKS_URL": in.Runtime.Verifier.JWKSURL,
		"BFF_STORE":             "object",
		"BFF_OBJECT_BUCKET":     in.Contract.NotesBucket,
		"BFF_OTEL_ENDPOINT":     in.Runtime.Telemetry.Endpoint,
		"BFF_RELEASE_REVISION":  in.Image.SourceRevision,
		"BFF_OTEL_ENVIRONMENT":  in.Runtime.Telemetry.Environment,
		"BFF_OTEL_SAMPLE_RATIO": fmt.Sprint(in.Runtime.Telemetry.SampleRatio),
	}
}

func declaredNames(t *testing.T) []string {
	t.Helper()
	var names []string
	for _, match := range regexp.MustCompile(`name\s+=\s+"(BFF_[A-Z_]+)"`).FindAllStringSubmatch(mainTF(t), -1) {
		names = append(names, match[1])
	}
	slices.Sort(names)
	return names
}

func TestTheDeclaredEnvironmentIsTheOneThisWitnessStarts(t *testing.T) {
	in, err := Load(write(t, example(t)))
	if err != nil {
		t.Fatal(err)
	}
	var known []string
	for name := range serviceEnvironment(in) {
		known = append(known, name)
	}
	slices.Sort(known)
	if declared := declaredNames(t); !slices.Equal(declared, known) {
		t.Fatalf("the module declares %v, this witness starts %v", declared, known)
	}
}

// keySet serves one synthetic signing key on a loopback address, which is what the binary fetches before it
// composes telemetry; the example's issuer is synthetic and resolves to nothing.
func keySet(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	encode := base64.RawURLEncoding.EncodeToString
	exponent := big.NewInt(int64(key.E)).Bytes()
	body := fmt.Sprintf(`{"keys":[{"kty":"RSA","kid":"start","use":"sig","alg":"RS256","n":%q,"e":%q}]}`, encode(key.N.Bytes()), encode(exponent))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

type started struct {
	ready    bool
	exited   bool
	stderr   string
	exitCode int
}

// start runs the real production binary with the environment given and nothing ambient, until it reports ready or
// exits. The address is the one change a hosted run does not need: PORT would bind every interface.
func start(t *testing.T, environment map[string]string) started {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv("BFF_BIN")))
	command.Env = []string{"BFF_ADDR=127.0.0.1:0"}
	for name, value := range environment {
		command.Env = append(command.Env, name+"="+value)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	result := started{}
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err == nil && strings.Contains(line, `"addr"`) {
		result.ready = true
		_ = command.Process.Kill()
	}
	waited := command.Wait()
	result.exited = !result.ready
	result.stderr = stderr.String()
	if exit, ok := waited.(*exec.ExitError); ok {
		result.exitCode = exit.ExitCode()
	}
	return result
}

type telemetryReport struct {
	Event  string
	Config struct {
		Endpoint, ServiceVersion, Environment string
		SampleRatio                           float64
	}
	Sources map[string]string
}

// resolved reads the binary's own report of what it resolved and from where.
func resolved(t *testing.T, result started) telemetryReport {
	t.Helper()
	if !result.ready {
		t.Fatalf("the binary did not start: exit %d: %s", result.exitCode, result.stderr)
	}
	for _, line := range strings.Split(result.stderr, "\n") {
		if strings.Contains(line, "telemetry_config") {
			var report telemetryReport
			if err := json.Unmarshal([]byte(line), &report); err != nil {
				t.Fatalf("%v: %s", err, line)
			}
			return report
		}
	}
	t.Fatalf("no configuration report: %s", result.stderr)
	return telemetryReport{}
}

// A ratio of zero keeps no trace, and the binary's empty-value default is one, so a zero that arrived empty would
// keep every trace. It reaches the binary as the value it is, read from the environment.
func TestAZeroSampleRatioReachesTheBinaryAsZero(t *testing.T) {
	in, err := Load(write(t, strings.Replace(example(t), `"sample_ratio": 1`, `"sample_ratio": 0`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	environment := serviceEnvironment(in)
	environment["BFF_VERIFIER_JWKS_URL"] = keySet(t)
	report := resolved(t, start(t, environment))
	if report.Config.SampleRatio != 0 || report.Sources["BFF_OTEL_SAMPLE_RATIO"] != "environment" {
		t.Errorf("the binary resolved %v from %q", report.Config.SampleRatio, report.Sources["BFF_OTEL_SAMPLE_RATIO"])
	}
}

// The module's variable names, with the funnel's values, start the real binary (spec 011 G09, spec 012 FR-008). The
// table above is this test's own, kept equal to the module's names; the mock tests pin the module's expressions.
func TestTheDeclaredEnvironmentStartsTheProductionBinary(t *testing.T) {
	in, err := Load(write(t, example(t)))
	if err != nil {
		t.Fatal(err)
	}
	environment := serviceEnvironment(in)
	environment["BFF_VERIFIER_JWKS_URL"] = keySet(t)
	report := resolved(t, start(t, environment))
	want := in.Runtime.Telemetry
	if report.Config.Endpoint != want.Endpoint || report.Config.Environment != want.Environment || report.Config.SampleRatio != want.SampleRatio || report.Config.ServiceVersion != in.Image.SourceRevision {
		t.Errorf("the binary resolved %+v, the funnel gave %+v at %s", report.Config, want, in.Image.SourceRevision)
	}
	for _, name := range []string{"BFF_OTEL_ENDPOINT", "BFF_RELEASE_REVISION", "BFF_OTEL_ENVIRONMENT", "BFF_OTEL_SAMPLE_RATIO"} {
		if report.Sources[name] != "environment" {
			t.Errorf("%s came from %q, not from the environment the module declares", name, report.Sources[name])
		}
	}
}

// Each required telemetry input is required by the real binary: this is why the module must pass them. These are
// the planted defects of the positive control above, with the same environment otherwise.
func TestTheBinaryRefusesWithoutARequiredTelemetryInput(t *testing.T) {
	in, err := Load(write(t, example(t)))
	if err != nil {
		t.Fatal(err)
	}
	jwks := keySet(t)
	for name, want := range map[string]string{
		"BFF_OTEL_ENDPOINT":    "telemetry.endpoint",
		"BFF_RELEASE_REVISION": "telemetry.service_version",
	} {
		environment := serviceEnvironment(in)
		environment["BFF_VERIFIER_JWKS_URL"] = jwks
		delete(environment, name)
		result := start(t, environment)
		if result.ready || !result.exited || result.exitCode != 1 || !strings.Contains(result.stderr, want) {
			t.Errorf("without %s: ready %v, exit %d, stderr %q, want a refusal naming %s", name, result.ready, result.exitCode, result.stderr, want)
		}
	}
}
