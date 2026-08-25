package runmeta

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyRunUnsignedAndHashMismatch(t *testing.T) {
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	runID := "test-run"
	runDir := filepath.Join(".airlock", "runs", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	m := RunManifest{RunID: runID, Digest: DigestInfo{Path: filepath.Join(runDir, "run_digest.json")}}
	if err := Save(filepath.Join(runDir, "run_manifest.json"), m); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "events.jsonl"), []byte("{\"ts\":\"2026-01-01T00:00:00Z\",\"type\":\"RUN_START\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := BuildDigest(runID, runDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveDigest(filepath.Join(runDir, "run_digest.json"), d); err != nil {
		t.Fatal(err)
	}
	res, err := VerifyRun(runID, m)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "verified-unsigned" {
		t.Fatalf("got %s", res.Status)
	}
	if err := os.WriteFile(filepath.Join(runDir, "events.jsonl"), []byte("bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res2, err := VerifyRun(runID, m)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Status != "hash-mismatch" {
		t.Fatalf("got %s", res2.Status)
	}
}
