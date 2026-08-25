package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func whoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Print caller identity context",
		RunE: func(cmd *cobra.Command, args []string) error {
			host, _ := os.Hostname()
			fmt.Printf("user=%s\nhost=%s\nteam=%s\n", os.Getenv("USER"), host, os.Getenv("AIRLOCK_TEAM"))
			return nil
		},
	}
}
