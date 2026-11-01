// Command profile validates the local demo's typed profile once, at the top boundary (spec 012 FR-020, spec 011
// FR-016), and prints the effective values as shell assignments with their provenance. The profile file holds
// the defaults; an environment variable named in the schema overrides one value; nothing below this command
// carries a default of its own. A missing, unknown, malformed or out-of-range value is refused by name, before
// anything starts.
//
//	profile --file tools/dev/profile.json [--env]   # --env lets BFF_PORT, WEB_PORT, BFF_DATA_DIR, BFF_STORE override
//
// Exit 0 with one `name=value # source` line per field; 2 with the field named on stderr.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Profile is the local demo's typed configuration. Every field is required in the file.
type Profile struct {
	// Version of this schema; the only value this command understands is 1.
	Version int `json:"version"`
	// BffPort and WebPort are loopback ports, 1 to 65535, distinct.
	BffPort int `json:"bff_port"`
	WebPort int `json:"web_port"`
	// Store is "memory" (notes live until the demo stops) or "local" (notes live in DataDir until a reset).
	Store string `json:"store"`
	// DataDir is where the local store keeps notes, relative to the repository root or absolute.
	DataDir string `json:"data_dir"`
	// Identity is the local sign-in: "test-identity" is the development identity on loopback, any subject.
	Identity string `json:"identity"`
	// Seed is the synthetic subject the demo and its recording use.
	Seed string `json:"seed_subject"`
}

type field struct {
	name, env string
	set       func(p *Profile, raw string) error
}

var fields = []field{
	{"bff_port", "BFF_PORT", func(p *Profile, raw string) (err error) { p.BffPort, err = port(raw); return }},
	{"web_port", "WEB_PORT", func(p *Profile, raw string) (err error) { p.WebPort, err = port(raw); return }},
	{"store", "BFF_STORE", func(p *Profile, raw string) error { p.Store = raw; return nil }},
	{"data_dir", "BFF_DATA_DIR", func(p *Profile, raw string) error { p.DataDir = raw; return nil }},
}

func port(raw string) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number", raw)
	}
	return n, nil
}

func main() {
	file := flag.String("file", "", "the profile file")
	useEnv := flag.Bool("env", false, "let the schema's environment variables override the file")
	flag.Parse()
	if *file == "" {
		fmt.Fprintln(os.Stderr, "profile: --file is required")
		os.Exit(2)
	}
	profile, sources, err := load(*file, *useEnv, os.Getenv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "profile: %v\n", err)
		os.Exit(2)
	}
	for _, line := range render(profile, sources) {
		fmt.Println(line)
	}
}

// load reads the file, applies the environment overrides, validates, and records where each value came from.
func load(path string, useEnv bool, getenv func(string) string) (Profile, map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var profile Profile
	if err := decoder.Decode(&profile); err != nil {
		return Profile{}, nil, fmt.Errorf("%s: %w", path, err)
	}
	sources := map[string]string{}
	for _, name := range []string{"bff_port", "web_port", "store", "data_dir", "identity", "seed_subject", "version"} {
		sources[name] = filepath.Base(path)
	}
	if useEnv {
		for _, f := range fields {
			if raw := getenv(f.env); raw != "" {
				if err := f.set(&profile, raw); err != nil {
					return Profile{}, nil, fmt.Errorf("%s: %v", f.env, err)
				}
				sources[f.name] = f.env
			}
		}
	}
	return profile, sources, validate(profile)
}

func validate(p Profile) error {
	if p.Version != 1 {
		return fmt.Errorf("version: %d is not the supported schema version 1", p.Version)
	}
	for _, port := range []struct {
		name  string
		value int
	}{{"bff_port", p.BffPort}, {"web_port", p.WebPort}} {
		if port.value < 1 || port.value > 65535 {
			return fmt.Errorf("%s: %d is not a port between 1 and 65535", port.name, port.value)
		}
	}
	if p.BffPort == p.WebPort {
		return fmt.Errorf("bff_port and web_port: both %d; the server and the page need distinct ports", p.BffPort)
	}
	switch p.Store {
	case "memory", "local":
	default:
		return fmt.Errorf("store: %q is neither \"memory\" nor \"local\"", p.Store)
	}
	if p.Store == "local" && strings.TrimSpace(p.DataDir) == "" {
		return errors.New("data_dir: required when store is \"local\"")
	}
	if p.Identity != "test-identity" {
		return fmt.Errorf("identity: %q is not \"test-identity\", the only local sign-in", p.Identity)
	}
	if strings.TrimSpace(p.Seed) == "" {
		return errors.New("seed_subject: required")
	}
	return nil
}

// render prints each effective value with its source, for the launcher to eval and for a reader to see.
func render(p Profile, sources map[string]string) []string {
	values := map[string]string{
		"bff_port": strconv.Itoa(p.BffPort), "web_port": strconv.Itoa(p.WebPort), "store": p.Store,
		"data_dir": p.DataDir, "identity": p.Identity, "seed_subject": p.Seed,
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	var lines []string
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("%s=%s # from %s", name, strconv.Quote(values[name]), sources[name]))
	}
	return lines
}
