package inputs

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// validateTelemetry is stricter than the binary, which also accepts a loopback endpoint: this module declares no
// collector beside the service, so one would export to nothing. exactOrigin refuses it.
func (in *Inputs) validateTelemetry() error {
	t := in.Runtime.Telemetry
	if err := exactOrigin("runtime.telemetry.endpoint", t.Endpoint); err != nil {
		return err
	}
	if t.Environment != "dev" && t.Environment != "staging" && t.Environment != "prod" {
		return fmt.Errorf("runtime.telemetry.environment: %q is not dev, staging or prod", t.Environment)
	}
	if t.SampleRatio < 0 || t.SampleRatio > 1 {
		return fmt.Errorf("runtime.telemetry.sample_ratio: %v is not between 0 and 1", t.SampleRatio)
	}
	return nil
}

func (in *Inputs) validateRelease() error {
	r := in.Release
	if !digest.MatchString(r.ServingDigest) {
		return fmt.Errorf("release.serving_digest: %q is not a sha256 digest", r.ServingDigest)
	}
	if r.PreviousDigest != nil && !digest.MatchString(*r.PreviousDigest) {
		return fmt.Errorf("release.previous_digest: %q is not a sha256 digest", *r.PreviousDigest)
	}
	if r.ServingDigest != in.Image.Digest && (r.PreviousDigest == nil || r.ServingDigest != *r.PreviousDigest) {
		return fmt.Errorf("release.serving_digest: %s is neither the image digest nor the previous digest; nothing else is ever deployed", r.ServingDigest)
	}
	return nil
}

// validatePreviousRevision keeps a rollback's telemetry truthful: the served image reports the revision it was
// built from, so the previous digest must name its revision, and one digest cannot have two revisions.
func (in *Inputs) validatePreviousRevision() error {
	previous, named := in.Release.PreviousDigest, in.Release.PreviousRevision
	switch {
	case (previous == nil) != (named == nil):
		return errors.New("release.previous_revision: given exactly when release.previous_digest is")
	case named == nil:
		return nil
	case !revision.MatchString(*named):
		return fmt.Errorf("release.previous_revision: %q is not a full revision", *named)
	case *previous == in.Image.Digest && *named != in.Image.SourceRevision:
		return errors.New("release.previous_revision: differs from image.source_revision for the same digest")
	}
	return nil
}

// exactOrigin accepts one https origin with a host and nothing else: no path, query, user or fragment. It is an
// origin a hosted service reaches, so a host that names the machine it runs on is refused too: https on loopback
// passes every other rule and exports to nothing.
func exactOrigin(name, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.Path != "" || u.RawQuery != "" || u.User != nil || u.Fragment != "" || strings.Contains(u.Host, "*") {
		return fmt.Errorf("%s: %q must be one exact https origin with no path", name, raw)
	}
	if local(u.Hostname()) {
		return fmt.Errorf("%s: %q names this machine, not a hosted origin", name, raw)
	}
	return nil
}

// numericLabel is a final label a resolver reads as an IPv4 address however it is spelled: 127.1, 2130706433 and
// 0x7f.0.0.1 are not names, and net.ParseIP knows none of them.
var numericLabel = regexp.MustCompile(`^(0x[0-9a-f]*|[0-9]+)$`)

// local reports a host that is loopback or unspecified, in any spelling a URL can carry.
func local(host string) bool {
	name := strings.TrimSuffix(strings.ToLower(host), ".")
	if name == "" || name == "localhost" || strings.HasSuffix(name, ".localhost") {
		return true
	}
	if ip := net.ParseIP(name); ip != nil {
		return ip.IsLoopback() || ip.IsUnspecified()
	}
	return numericLabel.MatchString(name[strings.LastIndex(name, ".")+1:])
}
