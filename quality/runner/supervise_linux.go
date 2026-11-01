package runner

import (
	"bytes"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const prSetChildSubreaper = 36

func becomeSubreaper() error {
	_, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetChildSubreaper, 1, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

// groupAttributes puts the command in its own group. The command also dies with its supervisor, so
// a killed supervisor cannot leave it running.
func groupAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}

// children lists this process's direct children from /proc. After the supervised command is gone,
// every one of them is an adopted descendant.
func children() []int {
	self := os.Getpid()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var found []int
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		stat, err := os.ReadFile("/proc/" + entry.Name() + "/stat")
		if err != nil {
			continue
		}
		// The command name may hold spaces and parentheses, so fields start after the last ')'.
		index := bytes.LastIndexByte(stat, ')')
		if index < 0 {
			continue
		}
		fields := strings.Fields(string(stat[index+1:]))
		if len(fields) > 1 && fields[1] == strconv.Itoa(self) {
			found = append(found, pid)
		}
	}
	return found
}

// reapAdopted collects adopted orphans as they exit, so a process that waits for its own daemon to
// go, as a Bazel client does, does not see a zombie still answering. The command itself is left to
// its own Wait.
func reapAdopted(command int) (stop func()) {
	exited := make(chan os.Signal, 1)
	signal.Notify(exited, syscall.SIGCHLD)
	quit := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for {
			select {
			case <-quit:
				return
			case <-exited:
			}
			for _, pid := range children() {
				if pid != command {
					var status syscall.WaitStatus
					_, _ = syscall.Wait4(pid, &status, syscall.WNOHANG, nil)
				}
			}
		}
	}()
	return func() {
		signal.Stop(exited)
		close(quit)
		<-finished
	}
}

// removeDescendants stops every adopted process, reaps it, and repeats until none are left, because
// a stopping process hands its own children on to this one. It is bounded by two grace periods.
func removeDescendants() {
	deadline := time.Now().Add(2 * termGrace)
	sent := map[int]bool{}
	for time.Now().Before(deadline) {
		var status syscall.WaitStatus
		for {
			pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
			if pid <= 0 || err != nil {
				break
			}
		}
		left := children()
		if len(left) == 0 {
			return
		}
		late := time.Now().After(deadline.Add(-termGrace))
		for _, pid := range left {
			if late {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			} else if !sent[pid] {
				_ = syscall.Kill(pid, syscall.SIGTERM)
				sent[pid] = true
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, pid := range children() {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}
