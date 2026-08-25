package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func cleanupCmd() *cobra.Command {
	var olderThan string
	var keepLatest int
	var dryRun bool
	cmd := &cobra.Command{Use: "cleanup", Short: "Cleanup stale workspaces and old runs", RunE: func(cmd *cobra.Command, args []string) error {
		cutoff := time.Time{}
		if strings.TrimSpace(olderThan) != "" {
			d, err := parseAge(olderThan)
			if err != nil {
				return err
			}
			cutoff = time.Now().Add(-d)
		}
		runsRoot := filepath.Join(".airlock", "runs")
		ents, err := os.ReadDir(runsRoot)
		if err != nil {
			return err
		}
		type item struct {
			name string
			mod  time.Time
		}
		items := []item{}
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			info, _ := e.Info()
			items = append(items, item{name: e.Name(), mod: info.ModTime()})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].mod.After(items[j].mod) })
		toDelete := map[string]struct{}{}
		if keepLatest > 0 && len(items) > keepLatest {
			for _, it := range items[keepLatest:] {
				toDelete[it.name] = struct{}{}
			}
		}
		if !cutoff.IsZero() {
			for _, it := range items {
				if it.mod.Before(cutoff) {
					toDelete[it.name] = struct{}{}
				}
			}
		}
		for id := range toDelete {
			runDir := filepath.Join(runsRoot, id)
			wsDir := filepath.Join(".airlock", "workspaces", id)
			if dryRun {
				fmt.Printf("would remove %s and %s\n", runDir, wsDir)
				continue
			}
			_ = os.RemoveAll(runDir)
			_ = os.RemoveAll(wsDir)
			fmt.Printf("removed %s\n", id)
		}
		refreshIndex()
		action := "Removed"
		if dryRun {
			action = "Would remove"
		}
		fmt.Printf("%s %d run(s)\n", action, len(toDelete))
		return nil
	}}
	cmd.Flags().StringVar(&olderThan, "runs-older-than", "", "Remove runs older than duration (e.g. 7d)")
	cmd.Flags().IntVar(&keepLatest, "keep-latest", 0, "Keep only latest N runs")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be removed")
	return cmd
}

func parseAge(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
