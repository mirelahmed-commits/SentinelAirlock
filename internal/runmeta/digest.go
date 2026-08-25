package runmeta

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type RunDigest struct {
	RunID   string            `json:"run_id"`
	Created string            `json:"created"`
	Files   map[string]string `json:"files"`
}

func BuildDigest(runID, runDir string) (RunDigest, error) {
	d := RunDigest{RunID: runID, Created: nowUTC(), Files: map[string]string{}}
	add := func(name string) error {
		p := filepath.Join(runDir, name)
		if _, err := os.Stat(p); err != nil {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		h := sha256.Sum256(b)
		d.Files[name] = hex.EncodeToString(h[:])
		return nil
	}
	for _, n := range []string{"events.jsonl", "session_events.jsonl", "run_manifest.json", "changes.patch"} {
		if err := add(n); err != nil {
			return d, err
		}
	}
	if csum, err := checkpointMetaHash(filepath.Join(runDir, "checkpoints")); err == nil && csum != "" {
		d.Files["checkpoints.meta"] = csum
	}
	return d, nil
}

func SaveDigest(path string, d RunDigest) error {
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// LoadDigest reads a run_digest.json file from the given path.
func LoadDigest(path string) (RunDigest, error) {
	var d RunDigest
	b, err := os.ReadFile(path)
	if err != nil {
		return d, err
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return d, err
	}
	return d, nil
}

// LoadDigestFromDir reads run_digest.json from a run directory.
func LoadDigestFromDir(runDir string) (RunDigest, error) {
	return LoadDigest(filepath.Join(runDir, "run_digest.json"))
}

func checkpointMetaHash(root string) (string, error) {
	if _, err := os.Stat(root); err != nil {
		return "", nil
	}
	names := []string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		names = append(names, rel)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(names)
	h := sha256.Sum256([]byte(joinLines(names)))
	return hex.EncodeToString(h[:]), nil
}

func joinLines(in []string) string {
	out := ""
	for _, s := range in {
		out += s + "\n"
	}
	return out
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }
