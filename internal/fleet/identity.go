package fleet

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// MachineID returns a durable identity for the current machine, generating
// and persisting one at ~/.airlock/machine_id on first use. Every Sentinel
// running on this machine reports the same MachineID, distinguishing "which
// machine" from "which repository" (SentinelID, below) -- one machine can
// run several Sentinels (one per repo), each with its own SentinelID.
//
// AIRLOCK_MACHINE_ID_PATH overrides the storage path (test hook only, so
// `go test` never reads or writes a developer's real
// ~/.airlock/machine_id).
func MachineID() (string, error) {
	if p := strings.TrimSpace(os.Getenv("AIRLOCK_MACHINE_ID_PATH")); p != "" {
		return readOrCreateID(p)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return readOrCreateID(filepath.Join(home, ".airlock", "machine_id"))
}

// SentinelID returns a durable identity for "the Sentinel governing this
// repository," persisted at <repo>/.airlock/sentinel_id. It survives
// Sentinel restarts (stop/start, background/foreground toggling, crash
// recovery) so the fleet control plane sees one continuous inventory entry
// rather than a new identity every time Sentinel is relaunched against the
// same repository.
//
// This is deliberately distinct from the existing per-run SessionID
// (internal/cli/sentinel.go), which is a fresh UUID every time Sentinel
// starts -- it names that run's evidence directory
// (.airlock/runs/<session-id>/) and must never collide across restarts.
// SentinelID identifies the installation; SessionID identifies the current
// monitoring session running under it.
func SentinelID(repoAbs string) (string, error) {
	return readOrCreateID(filepath.Join(repoAbs, ".airlock", "sentinel_id"))
}

func readOrCreateID(path string) (string, error) {
	if b, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(b)); id != "" {
			return id, nil
		}
	}
	id := uuid.New().String()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(id+"\n"), 0o644); err != nil {
		return "", err
	}
	return id, nil
}
