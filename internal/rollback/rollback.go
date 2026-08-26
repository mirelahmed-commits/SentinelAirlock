// Package rollback is the single, shared implementation of Airlock's workspace
// rollback. Both the CLI (`airlock rollback`) and the web viewer's operator-mode
// Restore action call Execute here, so there is exactly one code path that
// restores a workspace, appends the ROLLBACK event, resets review, rebuilds the
// digest, writes rollback.json, regenerates the report, and refreshes the index.
//
// Honesty guarantees (unchanged from v2.2.0-rc1):
//   - Restores the isolated Airlock workspace (.airlock/workspaces/<id>/repo)
//     ONLY. The original --repo source directory is never touched.
//   - One checkpoint per run (cp-0), taken before agent execution.
//   - Operation-level rollback (undo last N operations) is future work.
package rollback

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mirelahmed-commits/SentinelAirlock/internal/events"
	"github.com/mirelahmed-commits/SentinelAirlock/internal/index"
	"github.com/mirelahmed-commits/SentinelAirlock/internal/report"
	"github.com/mirelahmed-commits/SentinelAirlock/internal/review"
	"github.com/mirelahmed-commits/SentinelAirlock/internal/runmeta"
	"github.com/mirelahmed-commits/SentinelAirlock/internal/workspace"
)

// Options describes a rollback request. RunID must already be resolved to a
// concrete run ID (callers use runmeta.ResolveRunID for "latest" etc.).
type Options struct {
	RunID      string
	Checkpoint string // defaults to "cp-0"
	Path       string // optional repo-relative subtree; empty = full restore
}

// Plan is the resolved, validated target of a rollback. It is computed without
// modifying anything, so it also backs the CLI dry-run preview.
type Plan struct {
	RunID          string
	Checkpoint     string
	CheckpointPath string
	WorkspacePath  string
	Path           string // cleaned repo-relative path, empty for full restore
	Mode           string // "full" | "path"
}

// Record is the on-disk rollback.json artifact.
type Record struct {
	RunID      string    `json:"run_id"`
	Checkpoint string    `json:"checkpoint"`
	Mode       string    `json:"mode"` // "full" | "path"
	Paths      []string  `json:"paths,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	Status     string    `json:"status"` // "complete" | "dry-run"
}

// Result summarizes a completed rollback.
type Result struct {
	RunID         string
	Checkpoint    string
	Mode          string
	Paths         []string
	WorkspacePath string
	DigestRebuilt bool
}

// BuildPlan resolves and validates a rollback target without modifying anything.
func BuildPlan(opts Options) (Plan, error) {
	var p Plan
	if opts.RunID == "" {
		return p, fmt.Errorf("run ID required")
	}
	checkpoint := opts.Checkpoint
	if checkpoint == "" {
		checkpoint = "cp-0"
	}

	runsDir := filepath.Join(".airlock", "runs", opts.RunID)
	manifest, err := runmeta.Load(filepath.Join(runsDir, "run_manifest.json"))
	if err != nil {
		return p, err
	}

	// Resolve checkpoint path from manifest, fall back to convention.
	cpPath := filepath.Join(runsDir, "checkpoints", checkpoint)
	for _, cp := range manifest.Checkpoints {
		if cp.ID == checkpoint && cp.Path != "" {
			cpPath = cp.Path
			break
		}
	}
	if _, err := os.Stat(cpPath); err != nil {
		return p, fmt.Errorf("checkpoint not found: %s", cpPath)
	}

	wsPath := manifest.WorkspacePath
	mode := "full"
	var cleanedPath string

	if opts.Path != "" {
		if filepath.IsAbs(opts.Path) {
			return p, fmt.Errorf("--path must be repo-relative, not absolute: %q", opts.Path)
		}
		cleaned := filepath.Clean(opts.Path)
		if cleaned == "." || strings.HasPrefix(cleaned, "..") {
			return p, fmt.Errorf("--path is invalid or escapes the workspace: %q", opts.Path)
		}
		cpSub := filepath.Join(cpPath, cleaned)
		wsSub := filepath.Join(wsPath, cleaned)
		if !fsExists(cpSub) && !fsExists(wsSub) {
			return p, fmt.Errorf("--path %q does not exist in checkpoint or workspace", cleaned)
		}
		cleanedPath = cleaned
		mode = "path"
	}

	return Plan{
		RunID:          opts.RunID,
		Checkpoint:     checkpoint,
		CheckpointPath: cpPath,
		WorkspacePath:  wsPath,
		Path:           cleanedPath,
		Mode:           mode,
	}, nil
}

// Execute performs the rollback and writes every derived artifact. It is the one
// implementation shared by the CLI and the web operator UI.
func Execute(opts Options) (Result, error) {
	plan, err := BuildPlan(opts)
	if err != nil {
		return Result{}, err
	}

	runsDir := filepath.Join(".airlock", "runs", plan.RunID)
	var restoredPaths []string

	// --- restore ---------------------------------------------------------
	if plan.Path != "" {
		if err := restoreSubtree(plan.CheckpointPath, plan.WorkspacePath, plan.Path); err != nil {
			return Result{}, fmt.Errorf("subtree restore failed: %w", err)
		}
		restoredPaths = []string{plan.Path}
	} else {
		if err := os.RemoveAll(plan.WorkspacePath); err != nil {
			return Result{}, fmt.Errorf("failed to clear workspace: %w", err)
		}
		if err := workspace.CopyRepo(plan.CheckpointPath, plan.WorkspacePath, nil); err != nil {
			return Result{}, fmt.Errorf("failed to restore from checkpoint: %w", err)
		}
	}

	now := time.Now().UTC()

	// 1. Append ROLLBACK event.
	evMeta := map[string]any{
		"run_id":     plan.RunID,
		"checkpoint": plan.Checkpoint,
		"mode":       plan.Mode,
		"cp_path":    plan.CheckpointPath,
	}
	if plan.Path != "" {
		evMeta["path"] = plan.Path
	}
	jsonlPath := filepath.Join(runsDir, "events.jsonl")
	if err := events.AppendJSONLEvent(jsonlPath, events.Event{
		TS:      now,
		Type:    "ROLLBACK",
		Summary: fmt.Sprintf("workspace %s restored from checkpoint %s", plan.Mode, plan.Checkpoint),
		Meta:    evMeta,
	}); err != nil {
		return Result{}, fmt.Errorf("failed to record rollback event: %w", err)
	}

	// 2. Mark review as needs-attention.
	prev, _ := review.Load(runsDir)
	if saveErr := review.Save(runsDir, review.Record{
		State:         review.StateNeedsAttention,
		PreviousState: prev.State,
		Note:          fmt.Sprintf("Rollback performed (mode=%s checkpoint=%s); re-review required.", plan.Mode, plan.Checkpoint),
		Reviewer:      "airlock",
		Timestamp:     now.Format(time.RFC3339),
	}); saveErr != nil {
		return Result{}, fmt.Errorf("failed to update review state: %w", saveErr)
	}

	// 3. Rebuild digest.
	digestRebuilt := false
	if digest, err := runmeta.BuildDigest(plan.RunID, runsDir); err == nil {
		if err := runmeta.SaveDigest(filepath.Join(runsDir, "run_digest.json"), digest); err == nil {
			digestRebuilt = true
		}
	}

	// 4. Write rollback.json.
	if err := saveRecord(runsDir, Record{
		RunID:      plan.RunID,
		Checkpoint: plan.Checkpoint,
		Mode:       plan.Mode,
		Paths:      restoredPaths,
		Timestamp:  now,
		Status:     "complete",
	}); err != nil {
		return Result{}, fmt.Errorf("failed to write rollback.json: %w", err)
	}

	// 5. Regenerate report.
	if evs, err := events.ReadJSONL(jsonlPath); err == nil {
		_ = report.Generate(runsDir, evs)
	}

	// 6. Refresh index.
	if store, err := index.Rebuild(".airlock/runs"); err == nil {
		_ = index.Save(index.DefaultPath(), store)
	}

	return Result{
		RunID:         plan.RunID,
		Checkpoint:    plan.Checkpoint,
		Mode:          plan.Mode,
		Paths:         restoredPaths,
		WorkspacePath: plan.WorkspacePath,
		DigestRebuilt: digestRebuilt,
	}, nil
}

// restoreSubtree restores a single repo-relative path from the checkpoint into
// the workspace. If the path was not in the checkpoint, it is removed.
func restoreSubtree(cpPath, wsPath, rel string) error {
	cpSub := filepath.Join(cpPath, rel)
	wsSub := filepath.Join(wsPath, rel)

	if !fsExists(cpSub) {
		return os.RemoveAll(wsSub)
	}
	info, err := os.Stat(cpSub)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := os.RemoveAll(wsSub); err != nil {
			return err
		}
		return workspace.CopyRepo(cpSub, wsSub, nil)
	}
	if err := os.MkdirAll(filepath.Dir(wsSub), 0o755); err != nil {
		return err
	}
	b, err := os.ReadFile(cpSub)
	if err != nil {
		return err
	}
	return os.WriteFile(wsSub, b, info.Mode().Perm())
}

func saveRecord(runsDir string, rec Record) error {
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(runsDir, "rollback.json"), b, 0o644)
}

func fsExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
