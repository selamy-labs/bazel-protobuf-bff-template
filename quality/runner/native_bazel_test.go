package runner

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/proto/registrypb"
)

func TestNativeArgumentWirePreservesBytesAndBounds(t *testing.T) {
	original := []string{"", "space and ' quote", "$(false)\nline", string([]byte{0xff, 0xfe}), "cquery"}
	var wire bytes.Buffer
	if err := writeNativeArguments(&wire, original); err != nil {
		t.Fatal(err)
	}
	decoded, err := readNativeArguments(bytes.NewReader(wire.Bytes()))
	if err != nil || !slices.Equal(decoded, original) {
		t.Fatalf("argv %q, %v; want %q", decoded, err, original)
	}
	for n := 0; n < wire.Len(); n++ {
		if _, err := readNativeArguments(bytes.NewReader(wire.Bytes()[:n])); err == nil {
			t.Fatalf("accepted request truncated at %d", n)
		}
	}
	var count [4]byte
	binary.BigEndian.PutUint32(count[:], nativeArgumentCount+1)
	if _, err := readNativeArguments(bytes.NewReader(count[:])); err == nil {
		t.Fatal("accepted excessive argument count")
	}
	limit := []string{strings.Repeat("x", nativeArgumentBytes-4)}
	wire.Reset()
	if err := writeNativeArguments(&wire, limit); err != nil {
		t.Fatal(err)
	}
	got, err := readNativeArguments(&wire)
	if err != nil || !slices.Equal(got, limit) {
		t.Fatalf("exact byte bound rejected: %v", err)
	}
	if err := writeNativeArguments(io.Discard, []string{limit[0] + "x"}); err == nil {
		t.Fatal("accepted excessive argument bytes")
	}
	var oversized bytes.Buffer
	_ = binary.Write(&oversized, binary.BigEndian, uint32(1))
	_ = binary.Write(&oversized, binary.BigEndian, uint32(nativeArgumentBytes))
	if _, err := readNativeArguments(&oversized); err == nil {
		t.Fatal("accepted oversized declared argument")
	}
}

type nativeYieldWriter struct {
	mu sync.Mutex
	bytes.Buffer
}

func (w *nativeYieldWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	_ = w.Buffer.WriteByte(p[0])
	w.mu.Unlock()
	runtime.Gosched()
	return 1, nil
}

type nativeQuotaWriter struct {
	bytes.Buffer
	remaining int
}

func (w *nativeQuotaWriter) Write(p []byte) (int, error) {
	n := min(len(p), w.remaining)
	_, _ = w.Buffer.Write(p[:n])
	w.remaining -= n
	if n < len(p) {
		return n, io.ErrClosedPipe
	}
	return n, nil
}

func TestNativeFramesSerializeWholeRecordsAndLatchFailure(t *testing.T) {
	out := &nativeYieldWriter{}
	frames := &nativeFrames{out: out}
	var wg sync.WaitGroup
	for n := range 16 {
		wg.Go(func() {
			for i := range 30 {
				if err := frames.write(nativeStdout, fmt.Appendf(nil, "%d:%d", n, i)); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	seen := map[string]bool{}
	for i := 0; i < 16*30; i++ {
		kind, data, err := readNativeFrame(&out.Buffer)
		if err != nil || kind != nativeStdout {
			t.Fatalf("interleaved frame %d: %d %q %v", i, kind, data, err)
		}
		if seen[string(data)] {
			t.Fatalf("duplicate/corrupt record %q", data)
		}
		seen[string(data)] = true
	}
	if out.Len() != 0 || len(seen) != 480 {
		t.Fatalf("record inventory %d, trailing %d", len(seen), out.Len())
	}
	short := &nativeQuotaWriter{remaining: 7}
	broken := &nativeFrames{out: short}
	if err := broken.write(nativeStdout, []byte("payload")); err == nil {
		t.Fatal("partial frame succeeded")
	}
	size := short.Len()
	if err := broken.write(nativeExit, make([]byte, 4)); err == nil || short.Len() != size {
		t.Fatal("status certified a failed frame")
	}
	var chunks bytes.Buffer
	payload := bytes.Repeat([]byte("a"), 2*nativeFrameBytes+3)
	if n, err := (nativeOutput{&nativeFrames{out: &chunks}, nativeStderr}).Write(payload); err != nil || n != len(payload) {
		t.Fatalf("split output %d %v", n, err)
	}
	var combined []byte
	for range 3 {
		kind, data, err := readNativeFrame(&chunks)
		if err != nil || kind != nativeStderr {
			t.Fatal("bad split frame", err)
		}
		combined = append(combined, data...)
	}
	if !bytes.Equal(combined, payload) || chunks.Len() != 0 {
		t.Fatal("chunked output differs")
	}
	bounded := &nativeFrames{out: io.Discard, count: nativeFrameCount - 1}
	if err := bounded.write(nativeStdout, nil); err != nil {
		t.Fatal(err)
	}
	if err := bounded.write(nativeExit, make([]byte, 4)); err == nil {
		t.Fatal("frame inventory bound ignored")
	}
}

func nativeProtocolServer(t *testing.T, serve func(net.Conn)) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "quality-wire-test-")
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(dir, "socket"))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		if _, err := readNativeArguments(conn); err == nil {
			serve(conn)
		}
	}()
	t.Cleanup(func() { _ = listener.Close(); <-done; _ = os.RemoveAll(dir) })
	return listener.Addr().String()
}

func TestNativeProxyRequiresCompleteOutputAndTerminalProtocol(t *testing.T) {
	cases := []struct {
		name        string
		send        func(net.Conn)
		out, errOut io.Writer
		want        int
	}{
		{"healthy", func(c net.Conn) {
			f := &nativeFrames{out: c}
			_ = f.write(nativeStdout, []byte("path\n"))
			_ = f.write(nativeStderr, []byte("diagnostic\n"))
			_ = f.write(nativeExit, make([]byte, 4))
			p := &nativePeer{terminal: true}
			_ = p.acknowledgement(c)
		}, io.Discard, io.Discard, 0},
		{"legitimate nonzero", func(c net.Conn) {
			f := &nativeFrames{out: c}
			_ = f.write(nativeExit, []byte{0, 0, 0, 7})
			p := &nativePeer{terminal: true}
			_ = p.acknowledgement(c)
		}, io.Discard, io.Discard, 7},
		{"missing status", func(c net.Conn) { _ = (&nativeFrames{out: c}).write(nativeStdout, []byte("data")) }, io.Discard, io.Discard, ExitConfig},
		{"partial payload", func(c net.Conn) { _, _ = c.Write([]byte{nativeStdout, 0, 0, 0, 4, 'a'}) }, io.Discard, io.Discard, ExitConfig},
		{"unknown stream", func(c net.Conn) { _ = (&nativeFrames{out: c}).write(99, nil) }, io.Discard, io.Discard, ExitConfig},
		{"oversized frame", func(c net.Conn) { _, _ = c.Write([]byte{nativeStdout, 0xff, 0xff, 0xff, 0xff}) }, io.Discard, io.Discard, ExitConfig},
		{"short status", func(c net.Conn) { _ = (&nativeFrames{out: c}).write(nativeExit, []byte{0, 0, 0}) }, io.Discard, io.Discard, ExitConfig},
		{"invalid status", func(c net.Conn) { _ = (&nativeFrames{out: c}).write(nativeExit, []byte{0, 0, 1, 0}) }, io.Discard, io.Discard, ExitConfig},
		{"failed stdout", func(c net.Conn) {
			f := &nativeFrames{out: c}
			_ = f.write(nativeStdout, []byte("data"))
			_ = f.write(nativeExit, make([]byte, 4))
		}, &nativeQuotaWriter{}, io.Discard, ExitConfig},
		{"failed stderr", func(c net.Conn) {
			f := &nativeFrames{out: c}
			_ = f.write(nativeStderr, []byte("data"))
			_ = f.write(nativeExit, make([]byte, 4))
		}, io.Discard, &nativeQuotaWriter{}, ExitConfig},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			socket := nativeProtocolServer(t, c.send)
			if code := BazelClient([]string{socket, "cquery"}, c.out, c.errOut); code != c.want {
				t.Fatalf("proxy exit %d, want %d", code, c.want)
			}
		})
	}
}

func nativeEventually(t *testing.T, what string, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for " + what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestNativeChannelStopsPendingDecodeAndLateAdmission(t *testing.T) {
	for _, mode := range []string{"completion", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			h := newHarness(t)
			marker := filepath.Join(t.TempDir(), "must-not-start")
			fakeBazel(h, "touch "+marker)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			b, err := newNativeBazelClient(ctx, h.options(native("client", "true"), registrypb.Stage_STAGE_FULL))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = b.close() })
			conn, err := net.Dial("unix", b.listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close() }()
			_, _ = conn.Write([]byte{0})
			nativeEventually(t, "accepted partial request", func() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.conn != nil })
			if mode == "deadline" {
				cancel()
			}
			began := time.Now()
			if err := b.close(); err == nil {
				t.Fatal("unfinished request not retained as parent error")
			}
			if time.Since(began) > 2*time.Second {
				t.Fatal("partial request blocked teardown")
			}
			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("pending decode started a client")
			}
			if code := BazelClient([]string{filepath.Join(b.dir, "socket"), "info"}, io.Discard, io.Discard); code == 0 {
				t.Fatal("late invocation succeeded")
			}
			called := false
			if err := b.admit(context.Background(), func() error { called = true; return nil }); err == nil || called {
				t.Fatal("terminal admission started a client")
			}
		})
	}
}

func TestNativeAdmissionSerializesStartAndTerminalStop(t *testing.T) {
	h := newHarness(t)
	b, err := newNativeBazelClient(context.Background(), h.options(native("client", "true"), registrypb.Stage_STAGE_FULL))
	if err != nil {
		t.Fatal(err)
	}
	entered, release, admitted, stopped := make(chan struct{}), make(chan struct{}), make(chan error, 1), make(chan struct{})
	go func() {
		admitted <- b.admit(context.Background(), func() error { close(entered); <-release; return nil })
	}()
	<-entered
	go func() { b.stop(); close(stopped) }()
	select {
	case <-stopped:
		close(release)
		t.Fatal("stop passed an unregistered Start")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-admitted; err != nil {
		t.Fatal(err)
	}
	<-stopped
	called := false
	if err := b.admit(context.Background(), func() error { called = true; return nil }); err == nil || called {
		t.Fatal("Start won after terminal stop")
	}
	if err := b.close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	marker := filepath.Join(t.TempDir(), "spent")
	log, err := os.CreateTemp(t.TempDir(), "log")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()
	if _, err := RunBazel(ctx, "/bin/sh", []string{"-c", "touch " + marker}, h.dir, gitEnv(), log); err == nil {
		t.Fatal("spent context admitted process")
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("spent context started process")
	}
}

func TestNativeStreamsBoundDrainAfterZeroExit(t *testing.T) {
	var held *os.File
	aborted := false
	start := func(c *exec.Cmd) error {
		fd, err := syscall.Dup(int(c.Stdout.(*os.File).Fd()))
		if err != nil {
			return err
		}
		held = os.NewFile(uintptr(fd), "held-output")
		return c.Start()
	}
	began := time.Now()
	code, err := runBazelStreams(context.Background(), "/bin/true", nil, "", gitEnv(), io.Discard, io.Discard, start, func() { aborted = true })
	if held != nil {
		_ = held.Close()
	}
	if code != 0 || !errors.Is(err, exec.ErrWaitDelay) || !aborted {
		t.Fatalf("zero-exit/incomplete output became %d,%v abort=%v", code, err, aborted)
	}
	if elapsed := time.Since(began); elapsed < time.Second || elapsed > 3*time.Second {
		t.Fatalf("unbounded/omitted drain timeout: %s", elapsed)
	}
}

func nativeClientOptions(h *harness, script string) Options {
	options := h.options(native("native-client", script), registrypb.Stage_STAGE_FULL)
	options.Registry.Checks[0].Command = append(options.Registry.Checks[0].GetCommand(), "native-helper", "{bazel_client}")
	return options
}

func TestNativeClientSeparateStreamsAndRepeatedInterpretedExit(t *testing.T) {
	h := newHarness(t)
	fakeBazel(h, `if [ "$1" = cquery ]; then
 (i=0; while [ "$i" -lt 100 ]; do printf 'diagnostic\n' >&2; i=$((i+1)); done) &
 printf 'artifact path\n'; wait
else exit 7; fi`)
	errPath := filepath.Join(t.TempDir(), "stderr")
	script := fmt.Sprintf(`value=$("$1" cquery 2>%q) || exit 31
[ "$value" = 'artifact path' ] || exit 32
[ "$(wc -l <%q)" -eq 100 ] || exit 33
"$1" build
[ "$?" -eq 7 ]`, errPath, errPath)
	if code := h.run(nativeClientOptions(h, script)); code != ExitOK {
		t.Fatalf("split/repeated client exit %d\n%s", code, h.out.String())
	}
}

func TestNativeClientParentRejectsSuppressedOutputFailure(t *testing.T) {
	for _, stream := range []string{"stdout", "stderr"} {
		t.Run(stream, func(t *testing.T) {
			h := newHarness(t)
			redirect := ""
			if stream == "stderr" {
				redirect = " >&2"
			}
			fakeBazel(h, "head -c 1048576 /dev/zero"+redirect)
			if code := h.run(nativeClientOptions(h, `"$1" build >/dev/null 2>/dev/null`)); code != ExitOK {
				t.Fatalf("healthy %s forwarding control failed %d\n%s", stream, code, h.out.String())
			}
			finished := filepath.Join(t.TempDir(), "helper-zero")
			script := `"$1" build | head -c 0`
			if stream == "stderr" {
				script = `("$1" build 3>&1 1>/dev/null 2>&3) | head -c 0`
			}
			script += fmt.Sprintf("\nprintf completed >%q\nexit 0", finished)
			if code := h.run(nativeClientOptions(h, script)); code == ExitOK {
				t.Fatalf("helper0 erased failed %s forwarding\n%s", stream, h.out.String())
			}
			if data, err := os.ReadFile(finished); err != nil || string(data) != "completed" {
				t.Fatalf("helper did not actually complete0: %q %v", data, err)
			}
			if !strings.Contains(h.out.String(), "native Bazel channel") {
				t.Fatalf("failure was not parent-owned channel failure:\n%s", h.out.String())
			}
		})
	}
}

func TestNativeClientSilentDisconnectCancelsBeforeHelperCompletion(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "client.pid")
	signals := filepath.Join(dir, "signals")
	finished := filepath.Join(dir, "helper-alive")
	fakeBazel(h, fmt.Sprintf(`trap 'printf "INT\n" >>%q; exit 0' INT
echo $$ >%q
while :; do sleep 0.01; done`, signals, pidFile))
	script := fmt.Sprintf(`"$1" info >/dev/null 2>&1 & proxy=$!
i=0; while [ ! -s %q ]; do i=$((i+1)); [ "$i" -lt 200 ] || exit 41; sleep 0.01; done
printf "proxy=%%s client=%%s before guaranteed disconnect\n" "$proxy" "$(cat %q)" >&2
# The Go test wrapper consumes TERM. Kill only this proxy to force a real disconnect.
kill -KILL "$proxy"; wait "$proxy" 2>/dev/null || :
printf "proxy wait ended\n" >&2
client=$(cat %q); i=0
while kill -0 "$client" 2>/dev/null; do i=$((i+1)); [ "$i" -lt 200 ] || exit 42; sleep 0.01; done
printf 'helper stayed alive' >%q
exit 0`, pidFile, pidFile, pidFile, finished)
	began := time.Now()
	if code := h.run(nativeClientOptions(h, script)); code == ExitOK {
		t.Fatal("silent disconnect erased by helper0")
	}
	if time.Since(began) > 5*time.Second {
		t.Fatalf("silent client waited for distant check deadline\n%s", h.out.String())
	}
	if data, err := os.ReadFile(finished); err != nil || string(data) != "helper stayed alive" {
		t.Fatalf("client not cancelled before helper completion: %q %v\n%s", data, err, h.out.String())
	}
	if data, _ := os.ReadFile(signals); string(data) != "INT\n" {
		t.Fatalf("want one client-only interrupt, got %q", data)
	}
}

func TestNativeClientUnreadPeerCannotHoldParentDrain(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	fakeBazel(h, fmt.Sprintf("touch %q\nexec yes output", ready))
	b, err := newNativeBazelClient(context.Background(), h.options(native("client", "true"), registrypb.Stage_STAGE_FULL))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.close() })
	conn, err := net.Dial("unix", b.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if err := writeNativeArguments(conn, []string{"build"}); err != nil {
		t.Fatal(err)
	}
	nativeEventually(t, "real writer started", func() bool { _, err := os.Stat(ready); return err == nil })
	began := time.Now()
	if err := b.close(); err == nil {
		t.Fatal("unfinished unread peer lost its parent error")
	}
	if time.Since(began) > 3*time.Second {
		t.Fatal("unread socket blocked parent close")
	}
}

func TestNativeClientOverlappingCancellationInterruptsOnce(t *testing.T) {
	saved := clientGrace
	clientGrace = 300 * time.Millisecond
	defer func() { clientGrace = saved }()
	h := newHarness(t)
	signals := filepath.Join(t.TempDir(), "signals")
	fakeBazel(h, fmt.Sprintf(`trap 'printf "INT\n" >>%q' INT; while :; do sleep 0.01; done`, signals))
	options := nativeClientOptions(h, `"$1" info`)
	options.Deadline = 500 * time.Millisecond
	if code := h.run(options); code != ExitInterrupted {
		t.Fatalf("deadline returned %d\n%s", code, h.out.String())
	}
	if data, _ := os.ReadFile(signals); string(data) != "INT\n" {
		t.Fatalf("overlapping helper/channel/deadline cancellation sent %q", data)
	}
}
