package runner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

// SuperviseArg is the hidden first argument that turns the runner into one check's supervisor.
// Only a supervisor adopts orphans. If the runner itself did, it would adopt a Bazel server it
// happened to start, and its cleanup would kill it (spec 006 research R5).
const SuperviseArg = "__supervise"

// termGrace is how long a process gets between SIGTERM and SIGKILL.
const termGrace = 2 * time.Second

// Supervise runs one command in its own process group and leaves nothing behind: when the command
// ends or SIGTERM arrives, the group and every adopted descendant are stopped and reaped. It returns
// the command's exit code.
func Supervise(argv []string) int {
	if err := becomeSubreaper(); err != nil {
		fmt.Fprintf(os.Stderr, "supervise: %v\n", err)
		return 125
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	command := exec.Command(argv[0], argv[1:]...)
	command.Stdin = nil
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	command.SysProcAttr = groupAttributes()
	if err := command.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "supervise: %v\n", err)
		return 125
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	stopReaping := reapAdopted(command.Process.Pid)
	defer stopReaping()
	code := 0
	select {
	case err := <-done:
		code = exitCode(err)
	case <-stop:
		code = 143
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(termGrace):
		}
	}
	_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	removeDescendants()
	return code
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal())
		}
		return exit.ExitCode()
	}
	return 125
}
