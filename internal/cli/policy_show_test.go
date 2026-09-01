package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShowEffectivePolicy_MissingFile(t *testing.T) {
	// Change to a temp dir with no airlock.yaml.
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	// Must not return an error; prints guidance instead.
	if err := showEffectivePolicy(); err != nil {
		t.Errorf("expected nil error for missing file, got %v", err)
	}
}

func TestShowEffectivePolicy_EmptyConfig_NoDenyRules(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	defer func() { _ = os.Chdir(orig) }()
	_ = os.Chdir(dir)

	_ = os.WriteFile(filepath.Join(dir, "airlock.yaml"), []byte("version: 1\n"), 0o644)

	// Capture stdout by redirecting os.Stdout.
	r, w, _ := os.Pipe()
	oldStdout := os.Stdout
	os.Stdout = w

	err := showEffectivePolicy()

	_ = w.Close()
	os.Stdout = oldStdout
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	out := string(buf[:n])

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Deny rules") {
		t.Error("expected 'Deny rules' label in output")
	}
	if !strings.Contains(out, "none configured") {
		t.Error("expected 'none configured' for empty deny rules")
	}
	if !strings.Contains(out, "No explicit path deny rules") {
		t.Error("expected no-deny-rules guidance message")
	}
}

func TestShowEffectivePolicy_WithDenyRules(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	defer func() { _ = os.Chdir(orig) }()
	_ = os.Chdir(dir)

	yaml := `version: 1
policy:
  deny_write:
    - "**/.env"
    - "**/*.key"
`
	_ = os.WriteFile(filepath.Join(dir, "airlock.yaml"), []byte(yaml), 0o644)

	r, w, _ := os.Pipe()
	oldStdout := os.Stdout
	os.Stdout = w

	err := showEffectivePolicy()

	_ = w.Close()
	os.Stdout = oldStdout
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	out := string(buf[:n])

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "**/.env") {
		t.Error("expected deny rule **/.env in output")
	}
	if !strings.Contains(out, "**/*.key") {
		t.Error("expected deny rule **/*.key in output")
	}
	// When deny rules are present the guidance message must NOT appear.
	if strings.Contains(out, "No explicit path deny rules") {
		t.Error("must not show no-deny-rules message when rules are configured")
	}
}

func TestShowEffectivePolicy_UsesEffectiveConfig(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	defer func() { _ = os.Chdir(orig) }()
	_ = os.Chdir(dir)

	// Config references a known pack; output should include that pack name.
	yaml := `version: 1
defaults:
  policy_pack: balanced
`
	_ = os.WriteFile(filepath.Join(dir, "airlock.yaml"), []byte(yaml), 0o644)

	r, w, _ := os.Pipe()
	oldStdout := os.Stdout
	os.Stdout = w

	err := showEffectivePolicy()

	_ = w.Close()
	os.Stdout = oldStdout
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	out := string(buf[:n])

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "balanced") {
		t.Error("expected pack name 'balanced' in output")
	}
}
