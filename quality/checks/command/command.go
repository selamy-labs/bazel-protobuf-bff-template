// Package command is the one entry point of a check binary with subcommands: the subcommand named first, its
// arguments passed on, its exit code returned, and a refusal to judge reported as exit 2 with the cause.
// The complexity and coverage commands share it, so neither carries its own copy (spec 011 FR-006).
package command

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// Dispatch runs the subcommand args[0] names from commands and returns its exit code. No subcommand, an unknown
// one or one that could not judge is exit 2, with the cause written to stderr under the binary's name.
func Dispatch(name string, args []string, commands map[string]func([]string) (int, error), stderr io.Writer) int {
	names := make([]string, 0, len(commands))
	for command := range commands {
		names = append(names, command)
	}
	sort.Strings(names)
	if len(args) < 1 {
		fmt.Fprintf(stderr, "usage: %s %s ...\n", name, strings.Join(names, "|"))
		return 2
	}
	run, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(stderr, "%s: unknown command %q\n", name, args[0])
		return 2
	}
	code, err := run(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 2
	}
	return code
}
