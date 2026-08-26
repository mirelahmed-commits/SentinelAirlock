package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/mirelahmed-commits/SentinelAirlock/internal/policypack"
)

func policyCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "policy", Short: "Policy pack operations"}
	cmd.AddCommand(policyListCmd())
	cmd.AddCommand(policyShowCmd())
	cmd.AddCommand(policyApplyCmd())
	return cmd
}

func policyListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available policy packs",
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, p := range policypack.List() {
				fmt.Printf("%s\t%s\t%s\n", p.Name, p.Version, p.Source)
			}
			return nil
		},
	}
}

func policyShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <pack>",
		Short: "Show policy pack YAML",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := policypack.Get(args[0])
			if err != nil {
				return err
			}
			fmt.Println(p.YAML)
			return nil
		},
	}
}

func policyApplyCmd() *cobra.Command {
	var out string
	c := &cobra.Command{
		Use:   "apply <pack>",
		Short: "Apply policy pack to airlock.yaml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := policypack.Get(args[0])
			if err != nil {
				return err
			}
			if out == "" {
				out = "airlock.yaml"
			}
			if err := os.WriteFile(out, []byte(p.YAML), 0o644); err != nil {
				return err
			}
			fmt.Printf("Applied policy pack %s to %s\n", p.Name, out)
			return nil
		},
	}
	c.Flags().StringVar(&out, "out", "airlock.yaml", "Policy output path")
	return c
}
