package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mirelahmed-commits/SentinelAirlock/internal/fleet"
	"github.com/spf13/cobra"
)

// fleetCmd wires the Airlock Fleet control-plane CLI: coordination,
// inventory, and desired-state policy distribution -- never filesystem-
// policy enforcement, which stays entirely local to each Sentinel (see
// internal/cli/sentinel.go). No full auth/signing/revocation (Prompt 14B)
// yet.
func fleetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fleet",
		Short: "Airlock Fleet control plane -- Sentinel enrollment, heartbeats, inventory, and policy distribution",
	}
	cmd.AddCommand(fleetServeCmd())
	cmd.AddCommand(fleetListCmd())
	cmd.AddCommand(fleetStatusCmd())
	cmd.AddCommand(fleetPolicyCmd())
	return cmd
}

func defaultFleetDBPath() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".airlock", "fleet.json")
	}
	return filepath.Join(".airlock", "fleet.json")
}

func fleetServeCmd() *cobra.Command {
	var listen, dbPath, policyDBPath, token string
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
			if strings.TrimSpace(policyDBPath) == "" {
				policyDBPath = filepath.Join(filepath.Dir(dbPath), "fleet-policies.json")
			}
			store, err := fleet.OpenStore(dbPath)
			if err != nil {
				return fmt.Errorf("could not open fleet store %s: %w", dbPath, err)
			}
			policyStore, err := fleet.OpenPolicyStore(policyDBPath)
			if err != nil {
				return fmt.Errorf("could not open fleet policy store %s: %w", policyDBPath, err)
			}
			ln, err := net.Listen("tcp", listen)
			if err != nil {
				return fmt.Errorf("unable to bind %s: %w", listen, err)
			}
			srv := fleet.NewServer(store, policyStore, token)
			fmt.Println("Airlock Fleet control plane started")
			fmt.Printf("Listen: http://%s\n", ln.Addr().String())
			fmt.Printf("Store:  %s\n", dbPath)
			fmt.Printf("Policy store: %s\n", policyDBPath)
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
	cmd.Flags().StringVar(&policyDBPath, "policy-db", "", "Fleet policy storage path (default: fleet-policies.json next to --db)")
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
	fmt.Fprintln(w, "SENTINEL\tSTATUS\tREPOSITORY\tDESIRED\tACTUAL\tSYNC\tHEARTBEAT")
	for _, sv := range snap.Sentinels {
		id := sv.SentinelID
		if len(id) > 8 {
			id = id[:8]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			id, sv.Health, sv.RepoPath, policyRefLabel(sv.DesiredPolicyID, itoaOrDash(sv.DesiredPolicyVersion)),
			policyRefLabel(sv.PolicyID, sv.PolicyVersion), syncLabel(sv.PolicyState), fleet.FormatAge(sv.LastHeartbeat))
	}
	_ = w.Flush()
}

func policyRefLabel(id, version string) string {
	if id == "" {
		return "-"
	}
	if version == "" || version == "-" {
		return id
	}
	return id + " v" + version
}

func itoaOrDash(v int) string {
	if v <= 0 {
		return "-"
	}
	return strconv.Itoa(v)
}

func syncLabel(policyState string) string {
	if policyState == "" {
		return "-"
	}
	return policyState
}

// --- Policy resource CLI (Prompt 14A) ---------------------------------------
//
// `--file` always means "read this local file's content and send it" -- the
// control plane never accepts or dereferences a filesystem path itself (see
// internal/fleet/server.go's policy handlers), so there is no way to ask a
// remote fleet control plane to read an arbitrary path on its own host.

func fleetPolicyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Manage Fleet-distributed desired-state policies",
	}
	cmd.AddCommand(fleetPolicyListCmd())
	cmd.AddCommand(fleetPolicyShowCmd())
	cmd.AddCommand(fleetPolicyCreateCmd())
	cmd.AddCommand(fleetPolicyUpdateCmd())
	cmd.AddCommand(fleetPolicyAssignCmd())
	return cmd
}

type policyVersionSummary struct {
	PolicyID    string    `json:"policy_id"`
	Version     int       `json:"version"`
	Hash        string    `json:"hash"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func fleetPolicyListCmd() *cobra.Command {
	var fleetURL, token string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Fleet-managed policies (latest version of each)",
		RunE: func(cmd *cobra.Command, args []string) error {
			var summaries []policyVersionSummary
			if err := fleetGet(fleetURL, token, "/api/fleet/policies", &summaries); err != nil {
				return err
			}
			if len(summaries) == 0 {
				fmt.Println("No Fleet policies created yet.")
				fmt.Println("Create one with: airlock fleet policy create <policy-id> --file <path>")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "POLICY\tLATEST VERSION\tHASH\tDESCRIPTION")
			for _, p := range summaries {
				fmt.Fprintf(w, "%s\tv%d\t%s\t%s\n", p.PolicyID, p.Version, p.Hash, p.Description)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&fleetURL, "fleet", "http://127.0.0.1:9090", "Fleet control plane URL")
	cmd.Flags().StringVar(&token, "token", "", "Fleet auth token")
	return cmd
}

func fleetPolicyShowCmd() *cobra.Command {
	var fleetURL, token string
	cmd := &cobra.Command{
		Use:   "show <policy-id>",
		Short: "Show every version of a Fleet-managed policy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var summaries []policyVersionSummary
			if err := fleetGet(fleetURL, token, "/api/fleet/policies/"+args[0], &summaries); err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "VERSION\tHASH\tCREATED\tDESCRIPTION")
			for _, p := range summaries {
				fmt.Fprintf(w, "v%d\t%s\t%s\t%s\n", p.Version, p.Hash, p.CreatedAt.Format(time.RFC3339), p.Description)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&fleetURL, "fleet", "http://127.0.0.1:9090", "Fleet control plane URL")
	cmd.Flags().StringVar(&token, "token", "", "Fleet auth token")
	return cmd
}

func fleetPolicyCreateCmd() *cobra.Command {
	var fleetURL, token, file, description string
	cmd := &cobra.Command{
		Use:   "create <policy-id>",
		Short: "Create a brand-new Fleet-managed policy (its first version)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			content, err := readPolicyFile(file)
			if err != nil {
				return err
			}
			var summary policyVersionSummary
			body := map[string]string{"policy_id": args[0], "description": description, "yaml": content}
			if err := fleetPost(fleetURL, token, "/api/fleet/policies", body, &summary); err != nil {
				return err
			}
			fmt.Printf("Created policy %s v%d (hash %s)\n", summary.PolicyID, summary.Version, summary.Hash)
			return nil
		},
	}
	cmd.Flags().StringVar(&fleetURL, "fleet", "http://127.0.0.1:9090", "Fleet control plane URL")
	cmd.Flags().StringVar(&token, "token", "", "Fleet auth token")
	cmd.Flags().StringVar(&file, "file", "", "Path to a local airlock.yaml-shaped policy document (required)")
	cmd.Flags().StringVar(&description, "description", "", "Optional human-readable description")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func fleetPolicyUpdateCmd() *cobra.Command {
	var fleetURL, token, file, description string
	cmd := &cobra.Command{
		Use:   "update <policy-id>",
		Short: "Add a new version to an existing Fleet-managed policy",
		Long: `Adds a new, immutable version to an existing policy -- it never rewrites
an existing version's content. "airlock fleet policy show <id>" lists every
version that has ever existed, including this new one, by its own number.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			content, err := readPolicyFile(file)
			if err != nil {
				return err
			}
			var summary policyVersionSummary
			body := map[string]string{"description": description, "yaml": content}
			if err := fleetPost(fleetURL, token, "/api/fleet/policies/"+args[0]+"/versions", body, &summary); err != nil {
				return err
			}
			fmt.Printf("Created %s v%d (hash %s)\n", summary.PolicyID, summary.Version, summary.Hash)
			return nil
		},
	}
	cmd.Flags().StringVar(&fleetURL, "fleet", "http://127.0.0.1:9090", "Fleet control plane URL")
	cmd.Flags().StringVar(&token, "token", "", "Fleet auth token")
	cmd.Flags().StringVar(&file, "file", "", "Path to a local airlock.yaml-shaped policy document (required)")
	cmd.Flags().StringVar(&description, "description", "", "Optional human-readable description")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func fleetPolicyAssignCmd() *cobra.Command {
	var fleetURL, token, sentinelID string
	var version int
	cmd := &cobra.Command{
		Use:   "assign <policy-id>",
		Short: "Assign a specific policy version to a Sentinel as its desired state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(sentinelID) == "" {
				return fmt.Errorf("--sentinel is required")
			}
			if version <= 0 {
				return fmt.Errorf("--version is required and must be positive")
			}
			body := map[string]any{"policy_id": args[0], "version": version}
			var view fleet.SentinelView
			path := "/api/fleet/sentinels/" + sentinelID + "/assign"
			if err := fleetPost(fleetURL, token, path, body, &view); err != nil {
				return err
			}
			fmt.Printf("Assigned %s v%d to Sentinel %s\n", args[0], version, sentinelID)
			fmt.Println("It will pick this up on its next heartbeat -- no restart required.")
			return nil
		},
	}
	cmd.Flags().StringVar(&fleetURL, "fleet", "http://127.0.0.1:9090", "Fleet control plane URL")
	cmd.Flags().StringVar(&token, "token", "", "Fleet auth token")
	cmd.Flags().StringVar(&sentinelID, "sentinel", "", "Target Sentinel ID (see 'airlock fleet list')")
	cmd.Flags().IntVar(&version, "version", 0, "Policy version to assign")
	return cmd
}

func readPolicyFile(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("--file is required")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("could not read %s: %w", path, err)
	}
	if strings.TrimSpace(string(b)) == "" {
		return "", fmt.Errorf("%s is empty", path)
	}
	return string(b), nil
}

func fleetGet(fleetURL, token, path string, out any) error {
	fleetURL = strings.TrimSpace(fleetURL)
	if fleetURL == "" {
		return fmt.Errorf("--fleet is required")
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(fleetURL, "/")+path, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach fleet control plane at %s: %w", fleetURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("fleet control plane returned %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func fleetPost(fleetURL, token, path string, body, out any) error {
	fleetURL = strings.TrimSpace(fleetURL)
	if fleetURL == "" {
		return fmt.Errorf("--fleet is required")
	}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(fleetURL, "/")+path, strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach fleet control plane at %s: %w", fleetURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("fleet control plane returned %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
