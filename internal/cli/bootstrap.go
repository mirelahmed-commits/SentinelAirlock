package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func bootstrapCmd() *cobra.Command {
	var policyPath string
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstrap environment for first safe run",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := os.Stat(policyPath); err != nil {
				fmt.Printf("Policy missing: %s (running init)\n", policyPath)
				if err := initCmd().RunE(cmd, nil); err != nil {
					return err
				}
			}
			fmt.Println("Step 1/3: environment doctor")
			if err := doctorCmd().RunE(cmd, nil); err != nil {
				return err
			}
			fmt.Println("Step 2/3: adapter doctor")
			if err := agentsDoctorCmd().RunE(cmd, nil); err != nil {
				return err
			}
			fmt.Println("Step 3/3: config doctor")
			if err := configDoctorCmd().RunE(cmd, nil); err != nil {
				return err
			}
			fmt.Println("Bootstrap complete. Next steps:")
			fmt.Println("1) ./airlock run --agent generic-shell --cmd 'mkdir -p src; echo hi > src/test.txt' --repo .")
			fmt.Println("2) ./airlock inspect <run_id>")
			fmt.Println("3) ./airlock serve --open")
			fmt.Printf("Policy path: %s\n", filepath.Clean(policyPath))
			return nil
		},
	}
	cmd.Flags().StringVar(&policyPath, "policy", "airlock.yaml", "Policy config path")
	return cmd
}
