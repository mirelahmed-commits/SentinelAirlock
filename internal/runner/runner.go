package runner

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
)

func RunAgent(workdir, agentCmd, task string) (exitCode int, stdout string, err error) {
	var cmd *exec.Cmd

	// Run agentCmd as a shell command to allow "claude-code --foo" strings.
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", agentCmd)
	} else {
		cmd = exec.Command("bash", "-lc", agentCmd)
	}

	cmd.Dir = workdir
	cmd.Env = append(os.Environ(), "AIRLOCK_TASK="+task)

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	runErr := cmd.Run()
	stdout = buf.String()

	if runErr == nil {
		return 0, stdout, nil
	}

	// best-effort exit code
	if ee, ok := runErr.(*exec.ExitError); ok {
		return ee.ExitCode(), stdout, runErr
	}
	return 1, stdout, runErr
}
