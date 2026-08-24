//go:build !windows

package main

// superviseChildren is a no-op off Windows. Catchable termination (SIGINT/
// SIGTERM, broken stdout) is handled by the serveStdio signal path; the only
// uncatchable case here is SIGKILL of this process, which has no in-process
// remedy and must be scoped by a supervisor (e.g. a systemd cgroup or a PID
// namespace) rather than by the binary.
func superviseChildren() error { return nil }
