package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const good = `{"version": 1, "bff_port": 8080, "web_port": 5173, "store": "local", "data_dir": ".local/notes", "identity": "test-identity", "seed_subject": "ana"}`

func write(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profile.json")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func env(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func TestTheFileIsTheOneDefaultSiteAndTheEnvironmentOverridesByName(t *testing.T) {
	profile, sources, err := load(write(t, good), true, env(map[string]string{"BFF_PORT": "9090"}))
	if err != nil {
		t.Fatal(err)
	}
	if profile.BffPort != 9090 || sources["bff_port"] != "BFF_PORT" || profile.WebPort != 5173 || sources["web_port"] != "profile.json" {
		t.Errorf("%+v %v", profile, sources)
	}
	lines := strings.Join(render(profile, sources), "\n")
	for _, want := range []string{`bff_port="9090" # from BFF_PORT`, `web_port="5173" # from profile.json`, `store="local" # from profile.json`, `seed_subject="ana" # from profile.json`} {
		if !strings.Contains(lines, want) {
			t.Errorf("missing %q in:\n%s", want, lines)
		}
	}
	if _, _, err := load(write(t, good), false, env(map[string]string{"BFF_PORT": "9090"})); err != nil {
		t.Fatal(err)
	} else if p, _, _ := load(write(t, good), false, env(map[string]string{"BFF_PORT": "9090"})); p.BffPort != 8080 {
		t.Errorf("without --env the environment must not override: %d", p.BffPort)
	}
}

func TestEveryRefusalNamesTheField(t *testing.T) {
	cases := map[string]struct {
		text string
		env  map[string]string
		want string
	}{
		"unknown field":       {strings.Replace(good, `"seed_subject"`, `"extra": 1, "seed_subject"`, 1), nil, "extra"},
		"missing field":       {strings.Replace(good, `, "seed_subject": "ana"`, "", 1), nil, "seed_subject"},
		"wrong version":       {strings.Replace(good, `"version": 1`, `"version": 2`, 1), nil, "version"},
		"port zero":           {strings.Replace(good, `"bff_port": 8080`, `"bff_port": 0`, 1), nil, "bff_port"},
		"port too large":      {strings.Replace(good, `"web_port": 5173`, `"web_port": 70000`, 1), nil, "web_port"},
		"equal ports":         {strings.Replace(good, `"web_port": 5173`, `"web_port": 8080`, 1), nil, "distinct"},
		"unknown store":       {strings.Replace(good, `"store": "local"`, `"store": "cloud"`, 1), nil, "store"},
		"local without a dir": {strings.Replace(good, `"data_dir": ".local/notes"`, `"data_dir": " "`, 1), nil, "data_dir"},
		"other identity":      {strings.Replace(good, `"identity": "test-identity"`, `"identity": "production"`, 1), nil, "identity"},
		"empty seed":          {strings.Replace(good, `"seed_subject": "ana"`, `"seed_subject": ""`, 1), nil, "seed_subject"},
		"env not a number":    {good, map[string]string{"WEB_PORT": "five"}, "WEB_PORT"},
		"env out of range":    {good, map[string]string{"BFF_PORT": "65536"}, "bff_port"},
		"not json":            {"{", nil, "profile.json"},
	}
	for name, c := range cases {
		_, _, err := load(write(t, c.text), true, env(c.env))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want a refusal naming %q", name, err, c.want)
		}
	}
}

func TestAMemoryStoreNeedsNoDirectory(t *testing.T) {
	text := strings.Replace(strings.Replace(good, `"store": "local"`, `"store": "memory"`, 1), `"data_dir": ".local/notes"`, `"data_dir": ""`, 1)
	if _, _, err := load(write(t, text), false, env(nil)); err != nil {
		t.Fatal(err)
	}
}
