package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// Local viewer lifecycle: lets `airlock serve` run detached in the background
// like a normal local tool (print URL/PID/log, keep the terminal free, stop/
// status on demand) instead of occupying the terminal like a dev server.
//
// State lives under .airlock/:
//   viewer.pid  — PID of the serving process (plain text)
//   viewer.json — {pid, mode, url, listen, log, started, background}
//   viewer.log  — stdout/stderr of a backgrounded viewer

const (
	viewerLogName  = "viewer.log"
	viewerPIDName  = "viewer.pid"
	viewerMetaName = "viewer.json"
)

type viewerMeta struct {
	PID        int    `json:"pid"`
	Mode       string `json:"mode"` // "operator" | "read-only"
	URL        string `json:"url"`
	Listen     string `json:"listen"`
	Log        string `json:"log"`
	Started    string `json:"started"`
	Background bool   `json:"background"`
}

func viewerDir() string      { return ".airlock" }
func viewerLogPath() string  { return filepath.Join(viewerDir(), viewerLogName) }
func viewerPIDPath() string  { return filepath.Join(viewerDir(), viewerPIDName) }
func viewerMetaPath() string { return filepath.Join(viewerDir(), viewerMetaName) }

func viewerModeLabel(readOnly bool) string {
	if readOnly {
		return "read-only"
	}
	return "operator"
}

// writeViewerMeta records the serving process's identity so --status/--stop and
// the duplicate-start guard can find it.
func writeViewerMeta(m viewerMeta) error {
	if err := os.MkdirAll(viewerDir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(viewerMetaPath(), b, 0o644); err != nil {
		return err
	}
	return os.WriteFile(viewerPIDPath(), []byte(fmt.Sprintf("%d\n", m.PID)), 0o644)
}

func readViewerMeta() (viewerMeta, bool) {
	var m viewerMeta
	b, err := os.ReadFile(viewerMetaPath())
	if err != nil {
		return m, false
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, false
	}
	return m, true
}

func removeViewerMeta() {
	_ = os.Remove(viewerMetaPath())
	_ = os.Remove(viewerPIDPath())
}

// processAlive reports whether pid refers to a live process (signal 0 probe).
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// runningViewer returns the recorded viewer metadata if a live viewer exists.
// A stale metadata file (process gone) is cleaned and reported as not running.
func runningViewer() (viewerMeta, bool) {
	m, ok := readViewerMeta()
	if !ok {
		return viewerMeta{}, false
	}
	if !processAlive(m.PID) {
		removeViewerMeta() // clean stale
		return viewerMeta{}, false
	}
	return m, true
}

// startViewerBackground launches a detached child that runs the foreground
// viewer, then returns control to the terminal.
func startViewerBackground(addr string, readOnly, openBrowser bool) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot locate airlock binary: %w", err)
	}
	if err := os.MkdirAll(viewerDir(), 0o755); err != nil {
		return err
	}
	logFile, err := os.OpenFile(viewerLogPath(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("cannot open viewer log: %w", err)
	}
	defer logFile.Close()

	args := []string{"serve", "--listen", addr, "--managed"}
	if readOnly {
		args = append(args, "--read-only")
	}
	if openBrowser {
		args = append(args, "--open")
	}

	cmd := exec.Command(self, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	// Detach into its own process group so it survives this terminal.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start background viewer: %w", err)
	}
	// Release the child so it is not reaped when this process exits.
	_ = cmd.Process.Release()

	// The managed child writes viewer.json once it is actually listening.
	m, ok := waitForViewer(3 * time.Second)
	mode := viewerModeLabel(readOnly)
	if !ok {
		fmt.Printf("Background viewer starting (pid %d), but it did not report ready in time.\n", cmd.Process.Pid)
		fmt.Printf("  Mode: %s\n  Log:  %s\n", mode, viewerLogPath())
		fmt.Printf("Check status with: airlock serve --status\n")
		return nil
	}

	fmt.Printf("Background viewer started.\n")
	fmt.Printf("  Mode:   %s\n", m.Mode)
	fmt.Printf("  URL:    %s\n", m.URL)
	fmt.Printf("  PID:    %d\n", m.PID)
	fmt.Printf("  Log:    %s\n", m.Log)
	fmt.Printf("  Stop:   airlock serve --stop\n")
	fmt.Printf("  Status: airlock serve --status\n")
	return nil
}

// waitForViewer polls for the managed child's metadata (presence == listening).
func waitForViewer(timeout time.Duration) (viewerMeta, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if m, ok := readViewerMeta(); ok && processAlive(m.PID) {
			return m, true
		}
		time.Sleep(80 * time.Millisecond)
	}
	return viewerMeta{}, false
}

// viewerStatus reports whether a local viewer is running.
func viewerStatus() error {
	m, running := runningViewer()
	if !running {
		fmt.Println("No local viewer running.")
		fmt.Println("Start one with: airlock serve --background --open")
		return nil
	}
	fmt.Println("Local viewer running.")
	fmt.Printf("  Mode:   %s\n", m.Mode)
	fmt.Printf("  URL:    %s\n", m.URL)
	fmt.Printf("  PID:    %d\n", m.PID)
	fmt.Printf("  Log:    %s\n", m.Log)
	fmt.Printf("  Since:  %s\n", m.Started)
	fmt.Printf("Stop with: airlock serve --stop\n")
	return nil
}

// viewerStop terminates the running viewer and clears its metadata.
func viewerStop() error {
	m, running := runningViewer()
	if !running {
		fmt.Println("No local viewer running (nothing to stop).")
		return nil
	}
	proc, err := os.FindProcess(m.PID)
	if err != nil {
		removeViewerMeta()
		return fmt.Errorf("could not find viewer process %d: %w", m.PID, err)
	}
	// Graceful first; the serving process removes its own metadata on SIGTERM.
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		_ = proc.Kill()
	}
	// Give it a moment to exit and self-clean; then ensure metadata is gone.
	for i := 0; i < 20; i++ {
		if !processAlive(m.PID) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if processAlive(m.PID) {
		_ = proc.Kill()
	}
	removeViewerMeta()
	fmt.Printf("Stopped local viewer (pid %d, %s mode).\n", m.PID, m.Mode)
	return nil
}
