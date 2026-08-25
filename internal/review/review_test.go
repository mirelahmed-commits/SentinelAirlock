package review

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadReviewRecord(t *testing.T) {
	runDir := t.TempDir()
	if err := Save(runDir, Record{State: StateApproved, PreviousState: StateUnreviewed, Note: "ok", Reviewer: "tester"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	r, err := Load(runDir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if r.State != StateApproved || r.PreviousState != StateUnreviewed {
		t.Fatalf("unexpected state: %+v", r)
	}
	if r.Timestamp == "" {
		t.Fatalf("expected timestamp")
	}
	if _, err := os.Stat(filepath.Join(runDir, "review.json")); err != nil {
		t.Fatalf("missing review file: %v", err)
	}
}
