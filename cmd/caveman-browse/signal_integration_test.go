//go:build integration

package main

import (
	"bufio"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const stdioHelperEnv = "CAVEMAN_BROWSE_STDIO_HELPER"

// TestStdioServerHelper is the subprocess entry point: with the env flag set it
// becomes the real stdio MCP server (serveStdio) instead of a test. The parent
// drives it over stdin/stdout and then signals it.
func TestStdioServerHelper(t *testing.T) {
	if os.Getenv(stdioHelperEnv) == "" {
		return
	}
	os.Exit(serveStdio(slog.New(slog.NewTextHandler(os.Stderr, nil))))
}

// TestStdioSignalReapsEmbeddedChrome proves the abrupt-shutdown leak fix: the
// embedded Chrome is a child of the server process and is reaped only by
// driver.Close(). Before the fix a SIGTERM killed the server without running
// deferred cleanup (chromedp sets no Pdeathsig), orphaning the whole Chrome
// tree. The server must now reap it on SIGTERM while stdin is still open.
func TestStdioSignalReapsEmbeddedChrome(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("process-tree assertion reads /proc; Linux only")
	}
	chrome := os.Getenv("CAVEMAN_BROWSE_CHROME")
	if chrome == "" {
		t.Skip("CAVEMAN_BROWSE_CHROME is required")
	}
	profile := filepath.Join(t.TempDir(), "profile")

	cmd := exec.Command(os.Args[0], "-test.run=^TestStdioServerHelper$")
	cmd.Env = append(os.Environ(),
		stdioHelperEnv+"=1",
		"CAVEMAN_BROWSE_CHROME="+chrome,
		"CAVEMAN_BROWSE_EPHEMERAL=1",
		"CAVEMAN_BROWSE_USER_DATA_DIR="+profile,
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start stdio server: %v", err)
	}
	// Kill only — waitExit owns the single cmd.Wait() for this process. A second
	// Wait here would return "Wait already called" and race the goroutine's.
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	r := bufio.NewReader(stdout)
	writeLine(t, stdin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`)
	readLineOrFail(t, r) // initialize result
	writeLine(t, stdin, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"browser_snapshot","arguments":{"url":"about:blank"}}}`)
	readLineOrFail(t, r) // snapshot result -> Chrome has launched

	if got := chromeProcsFor(profile); got == 0 {
		t.Fatalf("no embedded Chrome launched for profile %q; nothing to prove", profile)
	}

	// Signal while stdin is still open, so this exercises the signal path, not
	// the stdin-EOF path.
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("SIGTERM: %v", err)
	}
	waitExit(t, cmd)

	deadline := time.Now().Add(5 * time.Second)
	for {
		if chromeProcsFor(profile) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("SIGTERM orphaned embedded Chrome for profile %q", profile)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestStdioSignalDuringStartupReapsChrome covers the narrow window the ctx
// binding closes: a SIGTERM that lands while NewCDPDriver is still launching
// Chrome. Because the browser's lifetime is bound to ctx (not to a pointer
// assigned after launch), cancelling ctx reaps it no matter where startup was
// interrupted. Each iteration signals a few milliseconds after Start — early
// enough to overlap the launch — and asserts nothing is orphaned. The jitter
// walks the signal across different points of startup. Run under -race to prove
// the shutdown path shares no unsynchronized state with startup.
func TestStdioSignalDuringStartupReapsChrome(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("process-tree assertion reads /proc; Linux only")
	}
	chrome := os.Getenv("CAVEMAN_BROWSE_CHROME")
	if chrome == "" {
		t.Skip("CAVEMAN_BROWSE_CHROME is required")
	}

	for i := 0; i < 8; i++ {
		profile := filepath.Join(t.TempDir(), "profile", strconv.Itoa(i))

		cmd := exec.Command(os.Args[0], "-test.run=^TestStdioServerHelper$")
		cmd.Env = append(os.Environ(),
			stdioHelperEnv+"=1",
			"CAVEMAN_BROWSE_CHROME="+chrome,
			"CAVEMAN_BROWSE_EPHEMERAL=1",
			"CAVEMAN_BROWSE_USER_DATA_DIR="+profile,
		)
		// Keep stdin open (unread) so the process cannot exit via EOF; only the
		// signal path ends it.
		if _, err := cmd.StdinPipe(); err != nil {
			t.Fatalf("stdin pipe: %v", err)
		}
		cmd.Stdout = io.Discard
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			t.Fatalf("start stdio server: %v", err)
		}
		t.Cleanup(func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		})

		// Walk the signal from ~2ms to ~16ms after Start to overlap different
		// points of store-open + Chrome-launch across iterations.
		time.Sleep(time.Duration(2+2*i) * time.Millisecond)
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatalf("SIGTERM: %v", err)
		}
		waitExit(t, cmd)

		deadline := time.Now().Add(5 * time.Second)
		for {
			if chromeProcsFor(profile) == 0 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("iter %d: startup SIGTERM orphaned embedded Chrome for profile %q", i, profile)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// TestStdioServeErrorReapsChrome covers the other abrupt-death vector: the host
// vanishes mid-session and its end of stdout closes. The next server write hits
// a broken pipe. By default Go would kill the server with a fatal SIGPIPE that
// skips every defer and orphans Chrome; serveStdio ignores SIGPIPE so the write
// fails as a normal error, Serve returns it, and main's defers reap the tree.
// Stdin is kept open the whole time so this is strictly the write-error path,
// not the stdin-EOF path.
func TestStdioServeErrorReapsChrome(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("process-tree assertion reads /proc; Linux only")
	}
	chrome := os.Getenv("CAVEMAN_BROWSE_CHROME")
	if chrome == "" {
		t.Skip("CAVEMAN_BROWSE_CHROME is required")
	}
	profile := filepath.Join(t.TempDir(), "profile")

	// Own both pipes explicitly so the test controls exactly when the read end
	// of stdout closes (breaking the pipe) — StdoutPipe couples closing to Wait.
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestStdioServerHelper$")
	cmd.Env = append(os.Environ(),
		stdioHelperEnv+"=1",
		"CAVEMAN_BROWSE_CHROME="+chrome,
		"CAVEMAN_BROWSE_EPHEMERAL=1",
		"CAVEMAN_BROWSE_USER_DATA_DIR="+profile,
	)
	cmd.Stdin = inR
	cmd.Stdout = outW
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start stdio server: %v", err)
	}
	_ = inR.Close()  // child holds its own dup
	_ = outW.Close() // "
	defer func() { _ = inW.Close() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	r := bufio.NewReader(outR)
	writeLine(t, inW, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`)
	readLineOrFail(t, r) // initialize result
	writeLine(t, inW, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"browser_snapshot","arguments":{"url":"about:blank"}}}`)
	readLineOrFail(t, r) // snapshot result -> Chrome has launched

	if got := chromeProcsFor(profile); got == 0 {
		t.Fatalf("no embedded Chrome launched for profile %q; nothing to prove", profile)
	}

	// Break the pipe: close the only read end. Stdin stays open. The next
	// server response write now hits EPIPE.
	_ = outR.Close()
	// Drive a request whose reply the server will try to write. It may take a
	// couple before the buffered writer actually flushes into the broken pipe.
	for i := 0; i < 5; i++ {
		if err := tryWriteLine(inW, `{"jsonrpc":"2.0","id":3,"method":"tools/list"}`); err != nil {
			break // server (and its stdin) gone — it exited, as intended
		}
		time.Sleep(50 * time.Millisecond)
	}
	waitExit(t, cmd)

	deadline := time.Now().Add(5 * time.Second)
	for {
		if chromeProcsFor(profile) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("broken-stdout Serve error orphaned embedded Chrome for profile %q", profile)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func tryWriteLine(w io.Writer, line string) error {
	_, err := io.WriteString(w, line+"\n")
	return err
}

func writeLine(t *testing.T, w io.Writer, line string) {
	t.Helper()
	if _, err := io.WriteString(w, line+"\n"); err != nil {
		t.Fatalf("write request: %v", err)
	}
}

func readLineOrFail(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return line
}

func waitExit(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("stdio server did not exit within 10s of SIGTERM")
	}
}

// chromeProcsFor counts running Chrome processes whose command line references
// the unique per-test profile path — the embedded browser and its children.
func chromeProcsFor(marker string) int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		comm, err := os.ReadFile("/proc/" + e.Name() + "/comm")
		if err != nil || !strings.HasPrefix(strings.TrimSpace(string(comm)), "chrom") {
			continue
		}
		cmdline, err := os.ReadFile("/proc/" + e.Name() + "/cmdline")
		if err != nil {
			continue
		}
		if strings.Contains(strings.ReplaceAll(string(cmdline), "\x00", " "), marker) {
			n++
		}
	}
	return n
}
