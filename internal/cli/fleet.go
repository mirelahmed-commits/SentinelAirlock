package cli

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mirelahmed-commits/SentinelAirlock/internal/fleet"
	"github.com/spf13/cobra"
)

// fleetCmd wires the Airlock Fleet control-plane CLI. This is fleet
// foundation only: coordination/inventory, not filesystem-policy
// enforcement (that stays entirely local to each Sentinel -- see
// internal/cli/sentinel.go). No central policy distribution/reconciliation
// (Prompt 14A) and no full auth/signing/revocation (Prompt 14B) yet.
func fleetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fleet",
		Short: "Airlock Fleet control plane -- Sentinel enrollment, heartbeats, and inventory",
	}
	cmd.AddCommand(fleetServeCmd())
	cmd.AddCommand(fleetListCmd())
	cmd.AddCommand(fleetStatusCmd())
	return cmd
}

func defaultFleetDBPath() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".airlock", "fleet.json")
	}
	return filepath.Join(".airlock", "fleet.json")
}

func fleetServeCmd() *cobra.Command {
	var listen, dbPath, token string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Airlock Fleet control plane",
		Long: `Starts the coordination/inventory plane for many Sentinels.

    Airlock Fleet Control Plane
             |
             +-- Sentinel A (repo A)
             +-- Sentinel B (repo B)
             +-- Sentinel C (repo C)

The control plane is NOT in the filesystem-policy decision path. Every
Sentinel allows/denies/reverts filesystem mutations entirely from its own
local policy engine; enrollment and heartbeats are asynchronous management
traffic layered on top. If this process is unreachable or stopped, every
enrolled Sentinel keeps governing its repository locally and reconnects
automatically once this process comes back -- no Sentinel restart required.

Runs in the foreground (like 'airlock worker start'); Ctrl-C to stop, or
manage it with your own process supervisor. --token is an optional shared
secret for the v0 trust boundary -- see docs/architecture.md for what is and
is not authenticated in this fleet foundation.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(dbPath) == "" {
				dbPath = defaultFleetDBPath()
			}
			store, err := fleet.OpenStore(dbPath)
			if err != nil {
				return fmt.Errorf("could not open fleet store %s: %w", dbPath, err)
			}
			ln, err := net.Listen("tcp", listen)
			if err != nil {
				return fmt.Errorf("unable to bind %s: %w", listen, err)
			}
			srv := fleet.NewServer(store, token)
			fmt.Println("Airlock Fleet control plane started")
			fmt.Printf("Listen: http://%s\n", ln.Addr().String())
			fmt.Printf("Store:  %s\n", dbPath)
			if strings.TrimSpace(token) == "" {
				fmt.Println("Auth:   disabled (no --token set; see docs/architecture.md for the v0 trust boundary)")
			} else {
				fmt.Println("Auth:   enabled (shared token required)")
			}
			fmt.Println("Ctrl-C to stop. Enrolled Sentinels keep governing locally regardless of this process's availability.")
			return http.Serve(ln, srv.Handler())
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:9090", "Listen address")
	cmd.Flags().StringVar(&dbPath, "db", "", "Fleet inventory storage path (default ~/.airlock/fleet.json)")
	cmd.Flags().StringVar(&token, "token", "", "Optional shared enrollment/heartbeat token")
	return cmd
}

func fleetListCmd() *cobra.Command {
	var fleetURL, token string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Sentinels known to a fleet control plane",
		RunE: func(cmd *cobra.Command, args []string) error {
			snap, err := fetchFleetSnapshot(fleetURL, token)
			if err != nil {
				return err
			}
			printFleetTable(fleetURL, snap)
			return nil
		},
	}
	cmd.Flags().StringVar(&fleetURL, "fleet", "http://127.0.0.1:9090", "Fleet control plane URL")
	cmd.Flags().StringVar(&token, "token", "", "Fleet auth token")
	return cmd
}

func fleetStatusCmd() *cobra.Command {
	var fleetURL, token string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show fleet health summary (active/offline Sentinel counts)",
		RunE: func(cmd *cobra.Command, args []string) error {
			snap, err := fetchFleetSnapshot(fleetURL, token)
			if err != nil {
				return err
			}
			fmt.Printf("Fleet:   %s\n", fleetURL)
			fmt.Printf("Active:  %d\n", snap.Active)
			fmt.Printf("Offline: %d\n", snap.Offline)
			fmt.Printf("Total:   %d\n", len(snap.Sentinels))
			return nil
		},
	}
	cmd.Flags().StringVar(&fleetURL, "fleet", "http://127.0.0.1:9090", "Fleet control plane URL")
	cmd.Flags().StringVar(&token, "token", "", "Fleet auth token")
	return cmd
}

func fetchFleetSnapshot(fleetURL, token string) (*fleet.Snapshot, error) {
	fleetURL = strings.TrimSpace(fleetURL)
	if fleetURL == "" {
		return nil, fmt.Errorf("--fleet is required")
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(fleetURL, "/")+"/api/fleet/sentinels", nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach fleet control plane at %s: %w", fleetURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fleet control plane at %s returned %s", fleetURL, resp.Status)
	}
	var snap fleet.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return nil, fmt.Errorf("could not parse fleet response: %w", err)
	}
	return &snap, nil
}

func printFleetTable(fleetURL string, snap *fleet.Snapshot) {
	fmt.Println("AIRLOCK FLEET")
	fmt.Println()
	fmt.Printf("%d Active\n%d Offline\n\n", snap.Active, snap.Offline)
	if len(snap.Sentinels) == 0 {
		fmt.Printf("No Sentinels enrolled with %s yet.\n", fleetURL)
		fmt.Println("Enroll one with: airlock sentinel --repo . --fleet " + fleetURL + " --background")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "SENTINEL\tSTATUS\tREPOSITORY\tPOLICY\tHEARTBEAT")
	for _, sv := range snap.Sentinels {
		policy := sv.PolicyID
		if policy == "" {
			policy = "-"
		} else if sv.PolicyVersion != "" {
			policy += "@" + sv.PolicyVersion
		}
		id := sv.SentinelID
		if len(id) > 8 {
			id = id[:8]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", id, sv.Health, sv.RepoPath, policy, fleet.FormatAge(sv.LastHeartbeat))
	}
	_ = w.Flush()
}
