package command

import (
	"errors"
	"strings"
	"testing"
)

type dispatchCase struct {
	name       string
	args       []string
	wantCode   int
	wantStderr string
}

func sampleCommands() map[string]func([]string) (int, error) {
	return map[string]func([]string) (int, error){
		"check":   func(args []string) (int, error) { return len(args), nil },
		"compare": func([]string) (int, error) { return 0, errors.New("could not judge") },
	}
}

func dispatchCases() []dispatchCase {
	return []dispatchCase{
		{"no subcommand", nil, 2, "usage: tool check|compare ..."},
		{"unknown subcommand", []string{"judge"}, 2, `tool: unknown command "judge"`},
		{"the code passes through", []string{"check", "a", "b", "c"}, 3, ""},
		{"a refusal to judge", []string{"compare"}, 2, "tool: could not judge"},
	}
}

func TestDispatchExitCode(t *testing.T) {
	for _, tc := range dispatchCases() {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var stderr strings.Builder
			commands := sampleCommands()

			// Act
			code := Dispatch("tool", tc.args, commands, &stderr)

			// Assert
			if code != tc.wantCode {
				t.Fatalf("code=%d; want %d", code, tc.wantCode)
			}
		})
	}
}

func TestDispatchStderr(t *testing.T) {
	for _, tc := range dispatchCases() {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var stderr strings.Builder
			commands := sampleCommands()

			// Act
			_ = Dispatch("tool", tc.args, commands, &stderr)

			// Assert
			if got := strings.TrimSpace(stderr.String()); got != tc.wantStderr {
				t.Fatalf("stderr=%q; want %q", got, tc.wantStderr)
			}
		})
	}
}
