package runner

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// runBazelStreams owns process waiting separately from output forwarding. abortOutput must release
// writes to both destinations; native channels do so by closing their registered connection.
func runBazelStreams(ctx context.Context, bazel string, args []string, dir string, env []string,
	stdout, stderr io.Writer, start func(*exec.Cmd) error, abortOutput func()) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	command := exec.Command(bazel, args...)
	command.Dir, command.Env = dir, env
	pipes, err := newBazelPipes(stdout, stderr)
	if err != nil {
		return 0, err
	}
	defer pipes.close()
	command.Stdout, command.Stderr = pipes[0].write, pipes[1].write
	if err := start(command); err != nil {
		return 0, err
	}
	for _, p := range pipes {
		_ = p.write.Close()
	}
	clientCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var once sync.Once
	abort := func() {
		once.Do(func() {
			abortOutput()
			pipes.closeReaders()
		})
	}
	watchDone := make(chan struct{})
	stopWatch := context.AfterFunc(clientCtx, func() { abort(); close(watchDone) })
	defer func() {
		if !stopWatch() {
			<-watchDone
		}
	}()
	copied := make(chan error, len(pipes))
	for _, p := range pipes {
		go func() {
			_, err := io.Copy(p.out, p.read)
			if err != nil {
				cancel()
				abort()
			}
			copied <- err
		}()
	}
	code, waitErr := waitBazelClient(clientCtx, command)
	return code, errors.Join(waitErr, drainBazelPipes(ctx, copied, len(pipes), abort))
}

type bazelPipe struct {
	read, write *os.File
	out         io.Writer
}
type bazelPipes []bazelPipe

func newBazelPipes(stdout, stderr io.Writer) (bazelPipes, error) {
	var pipes bazelPipes
	for _, out := range []io.Writer{stdout, stderr} {
		read, write, err := os.Pipe()
		if err != nil {
			pipes.close()
			return nil, err
		}
		pipes = append(pipes, bazelPipe{read: read, write: write, out: out})
	}
	return pipes, nil
}

func (pipes bazelPipes) close() {
	for _, p := range pipes {
		_ = p.read.Close()
		_ = p.write.Close()
	}
}

func (pipes bazelPipes) closeReaders() {
	for _, p := range pipes {
		_ = p.read.Close()
	}
}

func drainBazelPipes(ctx context.Context, copied <-chan error, remaining int, abort func()) error {
	var errs []error
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	deadline := timer.C
	ended := ctx.Done()
	for remaining > 0 {
		select {
		case err := <-copied:
			errs = append(errs, err)
			remaining--
		case <-deadline:
			errs = append(errs, exec.ErrWaitDelay)
			abort()
			deadline = nil
		case <-ended:
			errs = append(errs, ctx.Err())
			abort()
			ended = nil
		}
	}
	return errors.Join(errs...)
}
