package adapters

import (
	"runtime"
	"testing"
)

func TestShellInvocation_Windows(t *testing.T) {
	exe, args := shellInvocationForOS("windows", "echo hello")
	if exe != "powershell" {
		t.Errorf("windows: exe = %q, want %q", exe, "powershell")
	}
	want := []string{"-NoProfile", "-NonInteractive", "-Command", "echo hello"}
	if len(args) != len(want) {
		t.Fatalf("windows: args = %v, want %v", args, want)
	}
	for i, a := range args {
		if a != want[i] {
			t.Errorf("windows: args[%d] = %q, want %q", i, a, want[i])
		}
	}
}

func TestShellInvocation_Unix(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		exe, args := shellInvocationForOS(goos, "echo hello")
		if exe != "bash" {
			t.Errorf("%s: exe = %q, want %q", goos, exe, "bash")
		}
		want := []string{"-lc", "echo hello"}
		if len(args) != len(want) {
			t.Fatalf("%s: args = %v, want %v", goos, args, want)
		}
		for i, a := range args {
			if a != want[i] {
				t.Errorf("%s: args[%d] = %q, want %q", goos, i, a, want[i])
			}
		}
	}
}

func TestShellInvocation_CurrentPlatform(t *testing.T) {
	exe, args := shellInvocation("mycommand")
	if runtime.GOOS == "windows" {
		if exe != "powershell" {
			t.Errorf("current platform (windows): exe = %q, want powershell", exe)
		}
		if len(args) < 1 || args[len(args)-1] != "mycommand" {
			t.Errorf("current platform (windows): command not last arg: %v", args)
		}
	} else {
		if exe != "bash" {
			t.Errorf("current platform (%s): exe = %q, want bash", runtime.GOOS, exe)
		}
		if len(args) < 1 || args[len(args)-1] != "mycommand" {
			t.Errorf("current platform (%s): command not last arg: %v", runtime.GOOS, args)
		}
	}
}

func TestGenericShellPrepare_EmptyCommand(t *testing.T) {
	a := GenericShellAdapter{}
	_, err := a.Prepare(RunContext{Command: ""})
	if err == nil {
		t.Fatal("expected error for empty command, got nil")
	}
}

func TestGenericShellPrepare_DisplayCommand(t *testing.T) {
	a := GenericShellAdapter{}
	inv, err := a.Prepare(RunContext{Command: "echo hi", WorkspacePath: "/some/path"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.DisplayCommand != "echo hi" {
		t.Errorf("DisplayCommand = %q, want %q", inv.DisplayCommand, "echo hi")
	}
	if inv.WorkingDir != "/some/path" {
		t.Errorf("WorkingDir = %q, want %q", inv.WorkingDir, "/some/path")
	}
}
