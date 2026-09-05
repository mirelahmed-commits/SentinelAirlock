package fleet

import "testing"

const sampleYAML = `version: 1
policy:
  deny_write: ["**/.env", "secrets/**"]
  allow_write: ["status.txt"]
network:
  mode: "off"
`

func TestComputePolicyHash_Deterministic(t *testing.T) {
	h1, _, err := ComputePolicyHash(sampleYAML)
	if err != nil {
		t.Fatal(err)
	}
	h2, _, err := ComputePolicyHash(sampleYAML)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("expected identical content to hash identically: %q != %q", h1, h2)
	}
}

func TestComputePolicyHash_IgnoresWhitespaceAndCommentDifferences(t *testing.T) {
	withComment := `# a comment that should not affect the hash
version: 1
policy:
  deny_write:
    - "**/.env"
    - "secrets/**"
  allow_write:
    - "status.txt"
network:
  mode: "off"
`
	h1, _, err := ComputePolicyHash(sampleYAML)
	if err != nil {
		t.Fatal(err)
	}
	h2, _, err := ComputePolicyHash(withComment)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("expected equivalent content (comment/whitespace only difference) to hash identically: %q != %q", h1, h2)
	}
}

func TestComputePolicyHash_ContentChangeProducesDifferentHash(t *testing.T) {
	h1, _, err := ComputePolicyHash(sampleYAML)
	if err != nil {
		t.Fatal(err)
	}
	changed := `version: 1
policy:
  deny_write: ["**/.env", "secrets/**"]
  allow_write: ["status.txt", "config.txt"]
network:
  mode: "off"
`
	h2, _, err := ComputePolicyHash(changed)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Fatal("expected a real rule change to produce a different hash")
	}
}

func TestComputePolicyHash_InvalidYAML_Errors(t *testing.T) {
	if _, _, err := ComputePolicyHash("not: valid: yaml: at: all: [["); err == nil {
		t.Fatal("expected an error for invalid YAML")
	}
}

func TestPolicyRef_Equal(t *testing.T) {
	a := PolicyRef{PolicyID: "production", Version: 2, Hash: "abc"}
	b := PolicyRef{PolicyID: "production", Version: 2, Hash: "abc"}
	c := PolicyRef{PolicyID: "production", Version: 2, Hash: "different"}
	if !a.Equal(b) {
		t.Fatal("expected identical refs to be equal")
	}
	if a.Equal(c) {
		t.Fatal("expected refs with different hashes to be unequal")
	}
}

func TestReconcileState_Unmanaged(t *testing.T) {
	status, _ := ReconcileState(Record{})
	if status != "" {
		t.Fatalf("expected unmanaged (no desired policy) to report empty status, got %q", status)
	}
}

func TestReconcileState_InSync(t *testing.T) {
	rec := Record{
		DesiredPolicyID: "production", DesiredPolicyVersion: 2, DesiredPolicyHash: "abc",
		PolicyID: "production", PolicyVersion: "2", PolicyHash: "abc",
	}
	if status, _ := ReconcileState(rec); status != "IN_SYNC" {
		t.Fatalf("expected IN_SYNC, got %q", status)
	}
}

func TestReconcileState_VersionMismatch_Drifted(t *testing.T) {
	rec := Record{
		DesiredPolicyID: "production", DesiredPolicyVersion: 2, DesiredPolicyHash: "v2hash",
		PolicyID: "production", PolicyVersion: "1", PolicyHash: "v1hash",
	}
	if status, _ := ReconcileState(rec); status != "DRIFTED" {
		t.Fatalf("expected DRIFTED for a version mismatch, got %q", status)
	}
}

func TestReconcileState_HashMismatchSameNominalVersion_Drifted(t *testing.T) {
	rec := Record{
		DesiredPolicyID: "production", DesiredPolicyVersion: 2, DesiredPolicyHash: "expected-hash",
		PolicyID: "production", PolicyVersion: "2", PolicyHash: "tampered-hash",
	}
	if status, _ := ReconcileState(rec); status != "DRIFTED" {
		t.Fatalf("expected DRIFTED when hash differs even with a matching nominal version, got %q", status)
	}
}

func TestReconcileState_ReconcileFailed_ForCurrentDesiredHash(t *testing.T) {
	rec := Record{
		DesiredPolicyID: "production", DesiredPolicyVersion: 2, DesiredPolicyHash: "v2hash",
		PolicyID: "production", PolicyVersion: "1", PolicyHash: "v1hash",
		ReconcileStatus: "RECONCILE_FAILED", ReconcileError: "invalid policy document", ReconcileForHash: "v2hash",
	}
	status, errMsg := ReconcileState(rec)
	if status != "RECONCILE_FAILED" || errMsg != "invalid policy document" {
		t.Fatalf("expected RECONCILE_FAILED with the error message, got status=%q err=%q", status, errMsg)
	}
}

func TestReconcileState_StaleFailure_ClearsOnNewAssignment(t *testing.T) {
	// A failure recorded against an OLD desired hash must not haunt a fresh
	// assignment -- it should read as a normal DRIFTED, not a stuck failure.
	rec := Record{
		DesiredPolicyID: "production", DesiredPolicyVersion: 3, DesiredPolicyHash: "v3hash",
		PolicyID: "production", PolicyVersion: "1", PolicyHash: "v1hash",
		ReconcileStatus: "RECONCILE_FAILED", ReconcileError: "old failure", ReconcileForHash: "v2hash",
	}
	status, _ := ReconcileState(rec)
	if status != "DRIFTED" {
		t.Fatalf("expected a stale failure against an old hash to read as DRIFTED after reassignment, got %q", status)
	}
}

func TestReconcileState_GroundTruthWinsOverStaleFailure(t *testing.T) {
	// If actual now matches desired (e.g. a transient failure that
	// succeeded on retry), IN_SYNC must win even if ReconcileStatus/
	// ReconcileForHash still says RECONCILE_FAILED for that exact hash.
	rec := Record{
		DesiredPolicyID: "production", DesiredPolicyVersion: 2, DesiredPolicyHash: "v2hash",
		PolicyID: "production", PolicyVersion: "2", PolicyHash: "v2hash",
		ReconcileStatus: "RECONCILE_FAILED", ReconcileError: "stale", ReconcileForHash: "v2hash",
	}
	status, _ := ReconcileState(rec)
	if status != "IN_SYNC" {
		t.Fatalf("expected ground-truth actual==desired to report IN_SYNC despite a stale failure report, got %q", status)
	}
}

func TestReconcileState_Reconciling(t *testing.T) {
	rec := Record{
		DesiredPolicyID: "production", DesiredPolicyVersion: 2, DesiredPolicyHash: "v2hash",
		PolicyID: "production", PolicyVersion: "1", PolicyHash: "v1hash",
		ReconcileStatus: "RECONCILING",
	}
	if status, _ := ReconcileState(rec); status != "RECONCILING" {
		t.Fatalf("expected RECONCILING, got %q", status)
	}
}
