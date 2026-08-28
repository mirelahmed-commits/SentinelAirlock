//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"syscall"
)

func osProcAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func osDetachCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func osSendStop(proc *os.Process) error {
	return proc.Signal(syscall.SIGTERM)
}
