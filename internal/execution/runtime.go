package execution

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Runtime string

const (
	RuntimeAuto   Runtime = "auto"
	RuntimeDocker Runtime = "docker"
	RuntimeColima Runtime = "colima"
	RuntimePodman Runtime = "podman"
)

type RuntimeInfo struct {
	Name       Runtime
	Available  bool
	BinaryPath string
	SocketPath string
	CanAccess  bool
	Hint       string
}

func DetectRuntime(rt Runtime) (RuntimeInfo, error) {
	if rt == RuntimeAuto {
		for _, cand := range []Runtime{RuntimeDocker, RuntimeColima, RuntimePodman} {
			info, _ := DetectRuntime(cand)
			if info.Available && info.CanAccess {
				return info, nil
			}
		}
		return RuntimeInfo{}, fmt.Errorf("no viable container runtime found")
	}
	bin := string(rt)
	if rt == RuntimeColima {
		bin = "docker"
	}
	p, err := exec.LookPath(bin)
	if err != nil {
		return RuntimeInfo{Name: rt, Available: false, Hint: remediationHint(rt, "binary_missing")}, fmt.Errorf("%s not found in PATH", rt)
	}
	info := RuntimeInfo{Name: rt, Available: true, BinaryPath: p}
	info.SocketPath = defaultSocket(rt)
	if info.SocketPath != "" {
		if st, err := os.Stat(info.SocketPath); err == nil {
			_ = st
			f, err := os.OpenFile(info.SocketPath, os.O_RDWR, 0)
			if err == nil {
				_ = f.Close()
				info.CanAccess = true
			} else {
				info.Hint = remediationHint(rt, "socket_perm")
			}
		} else {
			// still might work via runtime daemon defaults; don't hard fail solely on socket path
			info.CanAccess = true
		}
	} else {
		info.CanAccess = true
	}
	if !info.CanAccess && info.Hint == "" {
		info.Hint = remediationHint(rt, "generic")
	}
	return info, nil
}

func defaultSocket(rt Runtime) string {
	home, _ := os.UserHomeDir()
	switch rt {
	case RuntimeDocker:
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, ".docker", "run", "docker.sock")
		}
		return "/var/run/docker.sock"
	case RuntimeColima:
		if home == "" {
			return ""
		}
		return filepath.Join(home, ".colima", "default", "docker.sock")
	case RuntimePodman:
		if home == "" {
			return ""
		}
		return filepath.Join(home, ".local", "share", "containers", "podman", "machine", "podman.sock")
	default:
		return ""
	}
}

func remediationHint(rt Runtime, reason string) string {
	base := []string{}
	if rt == RuntimeDocker {
		base = append(base, "is Docker running?", "do you have access to docker socket?")
		if runtime.GOOS == "darwin" {
			base = append(base, "on mac, try Docker Desktop or Colima")
		}
	}
	if rt == RuntimeColima {
		base = append(base, "is Colima running? try: colima start")
	}
	if rt == RuntimePodman {
		base = append(base, "is podman machine running?")
	}
	if len(base) == 0 {
		base = append(base, "check container runtime installation")
	}
	return strings.Join(base, " ")
}
