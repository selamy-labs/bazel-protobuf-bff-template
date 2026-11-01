package command

import (
	"errors"
	"strings"
	"testing"
)

func TestDispatch(t *testing.T) {
	commands := map[string]func([]string) (int, error){
		"check":   func(args []string) (int, error) { return len(args), nil },
		"compare": func([]string) (int, error) { return 0, errors.New("could not judge") },
	}
	cases := map[string]struct {
		args []string
		code int
		text string
	}{
		"no subcommand":           {nil, 2, "usage: tool check|compare ..."},
		"unknown subcommand":      {[]string{"judge"}, 2, `tool: unknown command "judge"`},
		"the code passes through": {[]string{"check", "a", "b", "c"}, 3, ""},
		"a refusal to judge":      {[]string{"compare"}, 2, "tool: could not judge"},
	}
	for name, c := range cases {
		var stderr strings.Builder
		code := Dispatch("tool", c.args, commands, &stderr)
		if code != c.code || strings.TrimSpace(stderr.String()) != c.text {
			t.Errorf("%s: code %d, stderr %q; want %d, %q", name, code, stderr.String(), c.code, c.text)
		}
	}
}
