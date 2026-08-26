package cli

import "github.com/mirelahmed-commits/SentinelAirlock/internal/index"

func refreshIndex() {
	store, err := index.Rebuild(".airlock/runs")
	if err != nil {
		return
	}
	_ = index.Save(index.DefaultPath(), store)
}
