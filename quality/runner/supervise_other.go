//go:build !linux

package runner

import "syscall"

// No subreaper exists here, so only the process group is owned. This path is unverified.
func becomeSubreaper() error { return nil }

func groupAttributes() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }

func removeDescendants() {}
