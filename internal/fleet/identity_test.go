package fleet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSentinelID_StableAcrossCalls(t *testing.T) {
	repo := t.TempDir()
	id1, err := SentinelID(repo)
	if err != nil {
		t.Fatal(err)
	}
	id2, err := SentinelID(repo)
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("SentinelID should be stable across calls (simulating a Sentinel restart): %q != %q", id1, id2)
	}
}

func TestSentinelID_DifferentReposGetDifferentIDs(t *testing.T) {
	idA, err := SentinelID(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	idB, err := SentinelID(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if idA == idB {
		t.Fatal("expected distinct repos to get distinct sentinel identities")
	}
}

func TestSentinelID_PersistsOnDisk(t *testing.T) {
	repo := t.TempDir()
	id, err := SentinelID(repo)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo, ".airlock", "sentinel_id")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to be written: %v", path, err)
	}
	// Re-reading via the same helper (simulating a fresh process start)
	// must return the identity that was persisted, not a new one.
	again, err := SentinelID(repo)
	if err != nil {
		t.Fatal(err)
	}
	if again != id {
		t.Fatalf("expected persisted identity to be reused: %q != %q", again, id)
	}
}

func TestMachineID_StableAcrossCalls(t *testing.T) {
	t.Setenv("AIRLOCK_MACHINE_ID_PATH", filepath.Join(t.TempDir(), "machine_id"))
	id1, err := MachineID()
	if err != nil {
		t.Fatal(err)
	}
	id2, err := MachineID()
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("MachineID should be stable across calls: %q != %q", id1, id2)
	}
}

func TestMachineID_SharedAcrossRepos(t *testing.T) {
	t.Setenv("AIRLOCK_MACHINE_ID_PATH", filepath.Join(t.TempDir(), "machine_id"))
	machineForRepoA, err := MachineID()
	if err != nil {
		t.Fatal(err)
	}
	machineForRepoB, err := MachineID()
	if err != nil {
		t.Fatal(err)
	}
	if machineForRepoA != machineForRepoB {
		t.Fatal("expected the same machine identity regardless of which repo's sentinel asks")
	}
}
