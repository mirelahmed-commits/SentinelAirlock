package fleet

import (
	"path/filepath"
	"testing"
)

func TestPolicyStore_Create(t *testing.T) {
	ps, err := OpenPolicyStore(filepath.Join(t.TempDir(), "policies.json"))
	if err != nil {
		t.Fatal(err)
	}
	v, err := ps.Create("production", "prod policy", sampleYAML)
	if err != nil {
		t.Fatal(err)
	}
	if v.Version != 1 {
		t.Fatalf("expected the first version to be 1, got %d", v.Version)
	}
	if v.Hash == "" {
		t.Fatal("expected a non-empty hash")
	}
}

func TestPolicyStore_Create_RejectsInvalidYAML(t *testing.T) {
	ps, err := OpenPolicyStore(filepath.Join(t.TempDir(), "policies.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ps.Create("production", "", "not valid yaml: [["); err == nil {
		t.Fatal("expected an error creating a policy from invalid YAML")
	}
}

func TestPolicyStore_Create_DuplicateID_Rejected(t *testing.T) {
	ps, err := OpenPolicyStore(filepath.Join(t.TempDir(), "policies.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ps.Create("production", "", sampleYAML); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.Create("production", "", sampleYAML); err == nil {
		t.Fatal("expected creating an already-existing policy_id to fail")
	}
}

func TestPolicyStore_AddVersion_IncrementsAndPreservesHistory(t *testing.T) {
	ps, err := OpenPolicyStore(filepath.Join(t.TempDir(), "policies.json"))
	if err != nil {
		t.Fatal(err)
	}
	v1, err := ps.Create("production", "v1", sampleYAML)
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
	v2, err := ps.AddVersion("production", "v2", changed)
	if err != nil {
		t.Fatal(err)
	}
	if v2.Version != 2 {
		t.Fatalf("expected version 2, got %d", v2.Version)
	}
	if v2.Hash == v1.Hash {
		t.Fatal("expected different content to produce a different hash")
	}

	// v1's exact original content must remain identifiable and unmutated.
	got, ok := ps.GetVersion("production", 1)
	if !ok {
		t.Fatal("expected version 1 to remain retrievable after v2 was created")
	}
	if got.Hash != v1.Hash || got.YAML != sampleYAML {
		t.Fatal("v1's content must not be mutated by creating v2")
	}
}

func TestPolicyStore_AddVersion_UnknownPolicy_Rejected(t *testing.T) {
	ps, err := OpenPolicyStore(filepath.Join(t.TempDir(), "policies.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AddVersion("does-not-exist", "", sampleYAML); err == nil {
		t.Fatal("expected adding a version to a nonexistent policy to fail")
	}
}

func TestPolicyStore_GetLatest(t *testing.T) {
	ps, err := OpenPolicyStore(filepath.Join(t.TempDir(), "policies.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ps.Create("production", "", sampleYAML); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AddVersion("production", "", sampleYAML+"\n"); err != nil {
		t.Fatal(err)
	}
	latest, ok := ps.GetLatest("production")
	if !ok || latest.Version != 2 {
		t.Fatalf("expected latest version to be 2, got %+v ok=%v", latest, ok)
	}
}

func TestPolicyStore_ListLatest_MultiplePolicies(t *testing.T) {
	ps, err := OpenPolicyStore(filepath.Join(t.TempDir(), "policies.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ps.Create("production", "", sampleYAML); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.Create("ci-restricted", "", sampleYAML); err != nil {
		t.Fatal(err)
	}
	latest := ps.ListLatest()
	if len(latest) != 2 {
		t.Fatalf("expected 2 policies, got %d", len(latest))
	}
}

func TestPolicyStore_RestartPreservesHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policies.json")
	ps1, err := OpenPolicyStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ps1.Create("production", "", sampleYAML); err != nil {
		t.Fatal(err)
	}
	if _, err := ps1.AddVersion("production", "", sampleYAML+"\n"); err != nil {
		t.Fatal(err)
	}

	ps2, err := OpenPolicyStore(path)
	if err != nil {
		t.Fatal(err)
	}
	versions, ok := ps2.AllVersions("production")
	if !ok || len(versions) != 2 {
		t.Fatalf("expected both versions to survive a fresh OpenPolicyStore, got %d ok=%v", len(versions), ok)
	}
}
