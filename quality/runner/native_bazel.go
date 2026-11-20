package runner

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// BazelClientArg dispatches the private current-check Bazel client entrypoint.
const BazelClientArg = "__native_bazel_client"

// Transport bounds limit allocation; the host's exec admission still applies.
const (
	nativeArgumentBytes = 16 << 20
	nativeArgumentCount = 1 << 20
	nativeFrameBytes    = 32 << 10
	nativeFrameCount    = 1 << 20
	nativeStdout        = 1
	nativeStderr        = 2
	nativeExit          = 3
	nativeAck           = 4
)

type nativeBazelClient struct {
	ctx              context.Context
	cancel           context.CancelFunc
	listener         net.Listener
	dir, path        string
	done             chan struct{}
	stopWatch        func() bool
	options          Options
	mu               sync.Mutex
	closed, complete bool
	conn             net.Conn
	err              error
}

func newNativeBazelClient(ctx context.Context, options Options) (*nativeBazelClient, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// AF_UNIX paths are short; do not inherit an arbitrarily long TMPDIR.
	dir, err := os.MkdirTemp("/tmp", "quality-bazel-")
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*nativeBazelClient, error) { _ = os.RemoveAll(dir); return nil, err }
	listener, err := net.Listen("unix", filepath.Join(dir, "socket"))
	if err != nil {
		return fail(err)
	}
	self, err := os.Executable()
	if err != nil {
		_ = listener.Close()
		return fail(err)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	path := filepath.Join(dir, "bazel")
	script := "#!/bin/sh\nexec " + quote(self) + " " + quote(BazelClientArg) + " " + quote(listener.Addr().String()) + " \"$@\"\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		_ = listener.Close()
		return fail(err)
	}
	owned, cancel := context.WithCancel(ctx)
	b := &nativeBazelClient{ctx: owned, cancel: cancel, listener: listener, dir: dir, path: path, done: make(chan struct{}), options: options}
	b.stopWatch = context.AfterFunc(owned, b.stop)
	go b.serve()
	return b, nil
}

func (b *nativeBazelClient) problem(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err == nil {
		b.err = err
	}
}

// stop is the terminal admission transition, shared with registration and Start.
func (b *nativeBazelClient) stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	if b.conn != nil && !b.complete && b.err == nil {
		b.err = errors.New("native Bazel invocation incomplete at helper completion")
	}
	b.cancel()
	_ = b.listener.Close()
	if b.conn != nil {
		_ = b.conn.Close()
	}
}

func (b *nativeBazelClient) close() error {
	b.stop()
	<-b.done
	b.stopWatch()
	b.mu.Lock()
	err := b.err
	b.mu.Unlock()
	return errors.Join(err, os.RemoveAll(b.dir))
}

func (b *nativeBazelClient) admit(ctx context.Context, start func() error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return start()
}

func (b *nativeBazelClient) register(conn net.Conn) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.ctx.Err() != nil {
		if b.err == nil {
			b.err = errors.New("native Bazel connection arrived after terminal admission")
		}
		return false
	}
	b.conn, b.complete = conn, false
	return true
}

func (b *nativeBazelClient) serve() {
	defer close(b.done)
	for {
		conn, err := b.listener.Accept()
		if err != nil {
			b.mu.Lock()
			closed := b.closed
			b.mu.Unlock()
			if !closed {
				b.problem(fmt.Errorf("native Bazel accept: %w", err))
			}
			return
		}
		if !b.register(conn) {
			_ = conn.Close()
			return
		}
		// At most one accepted request/copy pair per check; other checks retain their pools.
		b.handle(conn)
		b.mu.Lock()
		b.conn = nil
		b.mu.Unlock()
	}
}

// nativePeer observes a silent peer independently of client output.
type nativePeer struct {
	mu       sync.Mutex
	terminal bool
}

func (p *nativePeer) allowTerminal() { p.mu.Lock(); p.terminal = true; p.mu.Unlock() }
func (p *nativePeer) acknowledgement(conn net.Conn) error {
	var ack [1]byte
	if _, err := io.ReadFull(conn, ack[:]); err != nil {
		return fmt.Errorf("native Bazel acknowledgement: %w", err)
	}
	p.mu.Lock()
	allowed := p.terminal
	p.mu.Unlock()
	if !allowed || ack[0] != nativeAck {
		return errors.New("unexpected native Bazel request data")
	}
	// Only the final acknowledgement may half-close. EOF before it cancels a silent client.
	n, err := conn.Read(ack[:])
	if n != 0 || !errors.Is(err, io.EOF) {
		return errors.New("invalid native Bazel acknowledgement terminator")
	}
	return nil
}

func (b *nativeBazelClient) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	args, err := readNativeArguments(conn)
	if err != nil {
		b.problem(fmt.Errorf("native Bazel request: %w", err))
		return
	}
	ctx, cancel := context.WithCancel(b.ctx)
	defer cancel()
	peer := &nativePeer{}
	acknowledged := make(chan error, 1)
	peerDone := make(chan struct{})
	go func() {
		err := peer.acknowledgement(conn)
		if err != nil {
			b.problem(err)
			cancel()
			_ = conn.Close()
		}
		acknowledged <- err
		close(peerDone)
	}()
	defer func() { _ = conn.Close(); <-peerDone }()
	frames := &nativeFrames{out: conn}
	args = append(slices.Clone(b.options.BazelStartup), args...)
	code, runErr := runBazelStreams(ctx, b.options.Bazel, args, b.options.Root, b.options.Env,
		nativeOutput{frames, nativeStdout}, nativeOutput{frames, nativeStderr},
		func(command *exec.Cmd) error { return b.admit(ctx, command.Start) },
		func() { _ = conn.Close() })
	if runErr != nil {
		b.problem(fmt.Errorf("native Bazel execution/output: %w", runErr))
		code = ExitConfig
		if ctx.Err() != nil {
			code = ExitInterrupted
		}
		_, _ = (nativeOutput{frames, nativeStderr}).Write([]byte("native Bazel client: " + runErr.Error() + "\n"))
	}
	// Wait/copy completion precedes this phase. The same writer owns whole frames and status.
	peer.allowTerminal()
	var status [4]byte
	binary.BigEndian.PutUint32(status[:], uint32(code))
	if err := frames.write(nativeExit, status[:]); err != nil {
		b.problem(err)
		return
	}
	if err := <-acknowledged; err != nil {
		return
	}
	b.mu.Lock()
	b.complete = true
	b.mu.Unlock()
	// Mark complete before EOF: a healthy proxy cannot return to its helper sooner.
}

func writeNativeBytes(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 || n > len(data) {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func writeNativeArguments(w io.Writer, args []string) error {
	if len(args) > nativeArgumentCount {
		return errors.New("too many native Bazel arguments")
	}
	total := uint64(len(args)) * 4
	for _, arg := range args {
		total += uint64(len(arg))
	}
	if total > nativeArgumentBytes {
		return errors.New("native Bazel arguments exceed transport bound")
	}
	if err := binary.Write(w, binary.BigEndian, uint32(len(args))); err != nil {
		return err
	}
	for _, arg := range args {
		if err := binary.Write(w, binary.BigEndian, uint32(len(arg))); err != nil {
			return err
		}
		if err := writeNativeBytes(w, []byte(arg)); err != nil {
			return err
		}
	}
	return nil
}

func readNativeArguments(r io.Reader) ([]string, error) {
	var count uint32
	if err := binary.Read(r, binary.BigEndian, &count); err != nil {
		return nil, err
	}
	if count > nativeArgumentCount {
		return nil, errors.New("too many native Bazel arguments")
	}
	total := uint64(count) * 4
	args := make([]string, 0, count)
	for range count {
		var size uint32
		if err := binary.Read(r, binary.BigEndian, &size); err != nil {
			return nil, err
		}
		total += uint64(size)
		if total > nativeArgumentBytes {
			return nil, errors.New("native Bazel arguments exceed transport bound")
		}
		data := make([]byte, size)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, err
		}
		args = append(args, string(data))
	}
	return args, nil
}

type nativeFrames struct {
	out   io.Writer
	mu    sync.Mutex
	count int
	err   error
}

func (f *nativeFrames) write(kind byte, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	if f.count >= nativeFrameCount || len(data) > nativeFrameBytes {
		f.err = errors.New("native Bazel frame bound exceeded")
		return f.err
	}
	f.count++
	var header [5]byte
	header[0] = kind
	binary.BigEndian.PutUint32(header[1:], uint32(len(data)))
	f.err = writeNativeBytes(f.out, header[:])
	if f.err == nil {
		f.err = writeNativeBytes(f.out, data)
	}
	return f.err
}

type nativeOutput struct {
	frames *nativeFrames
	kind   byte
}

func (out nativeOutput) Write(data []byte) (int, error) {
	written := 0
	for len(data) > 0 {
		size := min(len(data), nativeFrameBytes)
		if err := out.frames.write(out.kind, data[:size]); err != nil {
			return written, err
		}
		written += size
		data = data[size:]
	}
	return written, nil
}
func readNativeFrame(r io.Reader) (byte, []byte, error) {
	var header [5]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	size := binary.BigEndian.Uint32(header[1:])
	if size > nativeFrameBytes {
		return 0, nil, errors.New("native Bazel frame bound exceeded")
	}
	data := make([]byte, size)
	_, err := io.ReadFull(r, data)
	return header[0], data, err
}

// BazelClient returns command status only after local output, acknowledgement and parent EOF.
func BazelClient(args []string, stdout, stderr io.Writer) int {
	var conn net.Conn
	fail := func(err error) int {
		// Release the parent before any potentially blocked diagnostic write.
		if conn != nil {
			_ = conn.Close()
		}
		_, _ = fmt.Fprintf(stderr, "native Bazel client: %v\n", err)
		return ExitConfig
	}
	if len(args) == 0 {
		return fail(errors.New("missing native Bazel channel"))
	}
	var err error
	conn, err = net.Dial("unix", args[0])
	if err != nil {
		return fail(err)
	}
	defer func() { _ = conn.Close() }()
	if err := writeNativeArguments(conn, args[1:]); err != nil {
		return fail(err)
	}
	code, err := readNativeResponse(conn, stdout, stderr)
	if err != nil {
		return fail(err)
	}
	return code
}

// readNativeResponse keeps ordered stream copying separate from connection preparation.
func readNativeResponse(conn net.Conn, stdout, stderr io.Writer) (int, error) {
	for range nativeFrameCount {
		kind, data, err := readNativeFrame(conn)
		if err != nil {
			return 0, err
		}
		switch kind {
		case nativeStdout:
			err = writeNativeBytes(stdout, data)
		case nativeStderr:
			err = writeNativeBytes(stderr, data)
		case nativeExit:
			return acknowledgeNativeStatus(conn, data)
		default:
			return 0, errors.New("invalid native Bazel stream")
		}
		if err != nil {
			return 0, err
		}
	}
	return 0, errors.New("native Bazel frame count exceeded")
}

// acknowledgeNativeStatus follows all local output writes and waits for parent completion EOF.
func acknowledgeNativeStatus(conn net.Conn, data []byte) (int, error) {
	if len(data) != 4 || binary.BigEndian.Uint32(data) > 255 {
		return 0, errors.New("invalid native Bazel exit status")
	}
	if err := writeNativeBytes(conn, []byte{nativeAck}); err != nil {
		return 0, err
	}
	if err := conn.(*net.UnixConn).CloseWrite(); err != nil {
		return 0, err
	}
	var extra [1]byte
	n, end := conn.Read(extra[:])
	if n != 0 || !errors.Is(end, io.EOF) {
		return 0, errors.New("native Bazel status was not terminal")
	}
	return int(binary.BigEndian.Uint32(data)), nil
}
