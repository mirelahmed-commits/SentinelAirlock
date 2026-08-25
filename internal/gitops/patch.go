package gitops

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// CreatePatchForPaths builds a unified patch for selected relative file paths.
// It scopes patch generation to known changed files to avoid noisy whole-tree diffs.
func CreatePatchForPaths(origRepoPath, workspaceRepoPath string, relPaths []string) ([]byte, error) {
	origAbs, err := filepath.Abs(origRepoPath)
	if err != nil {
		return nil, err
	}
	wsAbs, err := filepath.Abs(workspaceRepoPath)
	if err != nil {
		return nil, err
	}

	paths := normalizePaths(relPaths)
	var all bytes.Buffer

	for _, rel := range paths {
		left := filepath.Join(origAbs, filepath.FromSlash(rel))
		right := filepath.Join(wsAbs, filepath.FromSlash(rel))
		leftExists := fileExists(left)
		rightExists := fileExists(right)

		// Skip impossible cases where neither side exists.
		if !leftExists && !rightExists {
			continue
		}
		if !leftExists {
			left = os.DevNull
		}
		if !rightExists {
			right = os.DevNull
		}

		cmd := exec.Command("git", "diff", "--no-index", "--", left, right)
		var out bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr

		runErr := cmd.Run()
		if runErr != nil && out.Len() == 0 {
			return nil, fmt.Errorf("git diff failed for %s: %v | %s", rel, runErr, stderr.String())
		}
		if out.Len() > 0 {
			all.Write(out.Bytes())
		}
	}

	return all.Bytes(), nil
}

func normalizePaths(relPaths []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(relPaths))
	for _, p := range relPaths {
		p = filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(p), "./"))
		if p == "" || p == "." {
			continue
		}
		if isExcludedPath(p) {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func isExcludedPath(p string) bool {
	return p == ".airlock" || strings.HasPrefix(p, ".airlock/") ||
		p == ".git" || strings.HasPrefix(p, ".git/") ||
		p == "node_modules" || strings.HasPrefix(p, "node_modules/")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
