//go:build windows

package cli

import (
	"os"
	"os/exec"
)

// Background viewer detachment is not supported on Windows.
// airlock run, inspect, verify, serve (foreground) all work normally.

func osProcAlive(pid int) bool {
	// os.FindProcess always succeeds on Windows; use Kill(0) equivalent via Signal.
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows the only portable liveness check without cgo is attempting
	// to send os.Interrupt — but that would actually interrupt the process.
	// Instead, conservatively treat the recorded PID as alive if the metadata
	// file exists (caller already checked that). Return false to prevent the
	// duplicate-start guard from blocking a new viewer start after a crash.
	_ = proc
	return false
}

func osDetachCmd(cmd *exec.Cmd) {
	// No-op on Windows — background detachment is not supported.
}

func osSendStop(proc *os.Process) error {
	return proc.Kill()
}
