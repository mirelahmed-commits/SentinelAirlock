package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/mirelahmed-commits/SentinelAirlock/internal/events"
	"github.com/mirelahmed-commits/SentinelAirlock/internal/fleet"
	"github.com/mirelahmed-commits/SentinelAirlock/internal/governance"
	"github.com/mirelahmed-commits/SentinelAirlock/internal/index"
	"github.com/mirelahmed-commits/SentinelAirlock/internal/policy"
	"github.com/mirelahmed-commits/SentinelAirlock/internal/policypack"
	"github.com/mirelahmed-commits/SentinelAirlock/internal/recorder"
	"github.com/mirelahmed-commits/SentinelAirlock/internal/report"
	"github.com/mirelahmed-commits/SentinelAirlock/internal/runmeta"
	"github.com/mirelahmed-commits/SentinelAirlock/internal/util"
	"github.com/mirelahmed-commits/SentinelAirlock/internal/workspace"
	"github.com/spf13/cobra"
)

// sentinelDebounce coalesces rapid-fire fsnotify events for the same path
// (editor atomic saves, multi-syscall writes) into one evaluation. See
// recorder.NewDebounced's doc comment. airlock run keeps immediate (0)
// evaluation — a single short-lived command needs synchronous-relative
// revert behavior, so this value is Sentinel-only.
const sentinelDebounce = 200 * time.Millisecond

// sentinelRefreshInterval is how often a running Sentinel rewrites its
// manifest/digest/report/index while active, so `airlock inspect/verify/
// replay <session>` reflect near-real-time state without the cost of doing
// it on every single filesystem event (which is not bounded by anything
// Sentinel controls — real editor/IDE activity can be arbitrarily frequent).
const sentinelRefreshInterval = 2 * time.Second

// fleetHeartbeatInterval returns fleet.DefaultHeartbeatInterval, unless
// AIRLOCK_FLEET_HEARTBEAT_INTERVAL is set to a valid Go duration -- a test
// hook only, so integration tests don't have to wait out the real ~10s
// production cadence to observe a heartbeat.
func fleetHeartbeatInterval() time.Duration {
	if v := strings.TrimSpace(os.Getenv("AIRLOCK_FLEET_HEARTBEAT_INTERVAL")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return fleet.DefaultHeartbeatInterval
}

// fleetEnrollBackoffBase returns the starting backoff for fleetTryEnroll's
// exponential retry burst, unless AIRLOCK_FLEET_ENROLL_BACKOFF_BASE is set
// to a valid Go duration -- a test hook only, so a test can exercise the
// bounded-retry behavior in milliseconds instead of the real up-to-30s
// production worst case.
func fleetEnrollBackoffBase() time.Duration {
	if v := strings.TrimSpace(os.Getenv("AIRLOCK_FLEET_ENROLL_BACKOFF_BASE")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return time.Second
}

func sentinelCmd() *cobra.Command {
	var (
		repoPath   string
		policyPath string
		policyPack string
		background bool
		status     bool
		stop       bool
		managed    bool
		fleetURL   string
		fleetToken string
	)

	cmd := &cobra.Command{
		Use:   "sentinel",
		Short: "Persistent policy monitoring for a repository, regardless of which process writes to it",
		Long: `Continuously governs a repository instead of one execution.

airlock run       = govern one execution Airlock launches
airlock sentinel  = continuously govern a repository, no matter which
                     process writes to it (VS Code, Codex, Claude Code,
                     OpenClaw, shell, git — anything, launched any way)

Honest semantics: this is persistent policy monitoring and best-effort
revert, not kernel-level mandatory access control. A filesystem watcher
observes mutations after the OS has already accepted them:

    filesystem mutation -> Sentinel detects -> policy evaluation
        -> ALLOW: preserved, recorded
        -> DENY:  reverted from baseline (best-effort), recorded

A process that reads a file in the narrow window before Sentinel reverts
it will see the denied content. Sentinel does not prevent that; it detects,
evaluates, and reverts as fast as it reasonably can, and always records
what happened.

Lifecycle:
  airlock sentinel --repo .                      foreground, attached
  airlock sentinel --repo . --background          detached, returns the terminal
  airlock sentinel --repo . --status              show whether it's running
  airlock sentinel --repo . --stop                stop it

Evidence lives under .airlock/runs/<session-id>/ — the same artifact model
as 'airlock run' — so 'airlock inspect/replay/verify <session-id>' all work
against a Sentinel session with no separate inspection stack.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			repoPath = util.DefaultIfEmpty(repoPath, ".")
			repoAbs, err := filepath.Abs(repoPath)
			if err != nil {
				return fmt.Errorf("could not resolve --repo %q: %w", repoPath, err)
			}
			if st, statErr := os.Stat(repoAbs); statErr != nil || !st.IsDir() {
				return fmt.Errorf("--repo %q is not a usable directory", repoAbs)
			}

			if status {
				return sentinelStatusCmd(repoAbs)
			}
			if stop {
				return sentinelStopCmd(repoAbs)
			}

			if existing, ok := runningSentinel(repoAbs); ok {
				fmt.Printf("Sentinel is already running for this repository.\n")
				fmt.Printf("  Repository: %s\n  Session:    %s\n  PID:        %d\n", existing.Repo, existing.Session, existing.PID)
				fmt.Printf("Stop it first with: airlock sentinel --repo %s --stop\n", repoAbs)
				return nil
			}

			if background {
				return startSentinelBackground(repoAbs, policyPath, policyPack, fleetURL, fleetToken)
			}

			return runSentinelForeground(repoAbs, policyPath, policyPack, managed, fleetURL, fleetToken)
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", ".", "Path to the repository to govern")
	cmd.Flags().StringVar(&policyPath, "policy", "airlock.yaml", "Policy config path (relative to --repo unless absolute)")
	cmd.Flags().StringVar(&policyPack, "policy-pack", "", "Policy pack to apply (strict, balanced, oss-maintainer, ci-safe, research)")
	cmd.Flags().BoolVar(&background, "background", false, "Run Sentinel detached in the background")
	cmd.Flags().BoolVar(&status, "status", false, "Show whether Sentinel is running for --repo")
	cmd.Flags().BoolVar(&stop, "stop", false, "Stop the running Sentinel for --repo")
	cmd.Flags().BoolVar(&managed, "managed", false, "Internal: run as the managed background sentinel")
	_ = cmd.Flags().MarkHidden("managed")
	cmd.Flags().StringVar(&fleetURL, "fleet", "", "Airlock Fleet control plane URL to enroll with and heartbeat to (optional; standalone if unset)")
	cmd.Flags().StringVar(&fleetToken, "fleet-token", "", "Shared token for the fleet control plane, if it requires one")
	return cmd
}

// runSentinelForeground resolves policy, takes the session-start checkpoint,
// seeds and starts the recorder against the real repo, and blocks until a
// stop signal (Ctrl-C, SIGTERM, or `airlock sentinel --stop`) arrives.
func runSentinelForeground(repoAbs, policyPath, policyPack string, managed bool, fleetURL, fleetToken string) error {
	sess, err := startSentinelSession(repoAbs, policyPath, policyPack, managed, fleetURL, fleetToken)
	if err != nil {
		return err
	}

	fmt.Println("Sentinel started")
	fmt.Printf("Repository: %s\n", repoAbs)
	fmt.Printf("Session:    %s\n", sess.sessionID)
	fmt.Printf("Policy:     %s\n", sess.policyPath)
	if !managed {
		fmt.Println("Persistent policy monitoring and best-effort revert. Ctrl-C to stop cleanly.")
		fmt.Printf("Or from another terminal: airlock sentinel --repo %s --stop\n", repoAbs)
	}

	stopCh := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		close(stopCh)
	}()

	ticker := time.NewTicker(sentinelRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			sess.shutdown()
			return nil
		case <-ticker.C:
			sess.refreshEvidence("running")
		}
	}
}

// startSentinelSession does everything a Sentinel session needs before it can
// start observing the repository: resolve policy, take the session-start
// checkpoint, seed and start the recorder, and publish the process's
// lifecycle metadata. Split out from runSentinelForeground (which just adds
// the OS-signal-driven wait/shutdown loop around it) so tests can drive a
// session deterministically — perform filesystem operations, assert on
// evidence, then call sess.shutdown() directly — without needing to send a
// real signal to the test process.
func startSentinelSession(repoAbs, policyPath, policyPack string, managed bool, fleetURL, fleetToken string) (*sentinelSession, error) {
	resolvedPolicyPath := policyPath
	if !filepath.IsAbs(resolvedPolicyPath) {
		resolvedPolicyPath = filepath.Join(repoAbs, resolvedPolicyPath)
	}
	cfg, cfgErr := policy.Load(resolvedPolicyPath)
	if cfgErr != nil {
		fmt.Printf("WARN: could not load %s (%v). Running with built-in ignores.\n", resolvedPolicyPath, cfgErr)
	}
	if strings.TrimSpace(policyPack) == "" && cfg != nil {
		policyPack = cfg.Defaults.PolicyPack
	}
	if strings.TrimSpace(policyPack) != "" {
		pack, err := policypack.Get(policyPack)
		if err != nil {
			return nil, err
		}
		if packCfg, err := policypack.ParseConfig(pack); err == nil {
			cfg = policypack.Merge(cfg, packCfg)
		}
	}

	sessionID := strings.TrimSpace(os.Getenv("AIRLOCK_RUN_ID_FORCE"))
	if sessionID == "" {
		sessionID = uuid.New().String()
	}
	runsDir := filepath.Join(repoAbs, ".airlock", "runs", sessionID)
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		return nil, err
	}

	logger, err := events.NewLogger(filepath.Join(runsDir, "events.jsonl"))
	if err != nil {
		return nil, err
	}

	ignore := []string{".git/**", ".airlock/**", "node_modules/**"}
	if cfg != nil && len(cfg.Workspace.Ignore) > 0 {
		ignore = cfg.Workspace.Ignore
	}
	checkpointPath := filepath.Join(runsDir, "checkpoints", "cp-0")
	if err := workspace.CopyRepo(repoAbs, checkpointPath, ignore); err != nil {
		_ = logger.Close()
		return nil, fmt.Errorf("could not take session-start checkpoint: %w", err)
	}

	startedAt := time.Now().UTC()
	logger.Add(events.Event{TS: startedAt, Type: "SENTINEL_START", Summary: "sentinel session started", Meta: map[string]any{
		"session_id": sessionID, "repo": repoAbs, "pid": os.Getpid(), "policy_path": resolvedPolicyPath,
	}})

	sess := &sentinelSession{
		repoAbs:        repoAbs,
		sessionID:      sessionID,
		runsDir:        runsDir,
		checkpointPath: checkpointPath,
		policyPath:     resolvedPolicyPath,
		policyPack:     policyPack,
		cfg:            cfg,
		logger:         logger,
		startedAt:      startedAt,
	}
	if err := sess.writeManifest("running"); err != nil {
		_ = logger.Close()
		return nil, err
	}
	sess.refreshEvidence("running")

	rec, err := recorder.NewDebounced(repoAbs, logger, cfg, governance.ApprovalAuto, sentinelDebounce)
	if err != nil {
		_ = logger.Close()
		return nil, err
	}
	if err := rec.Seed(); err != nil {
		_ = logger.Close()
		return nil, err
	}
	if err := rec.Start(); err != nil {
		_ = logger.Close()
		return nil, err
	}
	sess.rec = rec

	logDesc := "(foreground terminal)"
	if managed {
		logDesc = sentinelLogPath(repoAbs)
	}
	if err := writeSentinelMeta(repoAbs, sentinelMeta{
		PID: os.Getpid(), Repo: repoAbs, Session: sessionID,
		Started: startedAt.Format(time.RFC3339), Log: logDesc, Background: managed,
	}); err != nil {
		sess.shutdown()
		return nil, err
	}

	if strings.TrimSpace(fleetURL) != "" {
		sess.startFleet(fleetURL, fleetToken)
	}

	return sess, nil
}

// sentinelSession bundles the state a running Sentinel needs to keep its
// evidence (manifest, digest, report, index) current and to finalize
// cleanly on stop. It deliberately does not do a full-repo re-copy on every
// refresh — only the session-start checkpoint (once) plus re-deriving small
// evidence artifacts (manifest/digest/report/index) from what the recorder
// has already logged.
type sentinelSession struct {
	repoAbs        string
	sessionID      string
	runsDir        string
	checkpointPath string
	policyPath     string
	policyPack     string
	cfg            *policy.Config
	logger         *events.Logger
	rec            *recorder.Recorder
	startedAt      time.Time

	// Fleet reporting (optional; nil/zero when --fleet is unset). See
	// startFleet and fleetLoop below. sentinelID is the durable identity for
	// "this Sentinel governing this repo" -- distinct from sessionID, which
	// is fresh every restart. See internal/fleet/identity.go.
	sentinelID     string
	fleetMachineID string
	fleetClient    *fleet.Client
	fleetStopCh    chan struct{}
	fleetDone      chan struct{}
}

func (s *sentinelSession) writeManifest(status string) error {
	evs := s.logger.EventsSnapshot()
	manifest := runmeta.RunManifest{
		RunID:           s.sessionID,
		WorkspacePath:   s.repoAbs,
		PolicySummary:   runmeta.BuildPolicySummary(s.policyPath, s.cfg),
		ExecutionMode:   "sentinel",
		TouchedPaths:    touchedPathsFromEvents(evs),
		DeniedPaths:     deniedPathsFromEvents(evs),
		Checkpoints:     []runmeta.Checkpoint{{ID: "cp-0", Path: s.checkpointPath}},
		RiskSummary:     riskSummaryFromEvents(evs),
		ApprovalSummary: approvalSummaryFromEvents(evs),
		Adapter:         runmeta.AdapterSummary{Name: "sentinel"},
		Invocation:      runmeta.InvocationSummary{DisplayCommand: fmt.Sprintf("airlock sentinel --repo %s", s.repoAbs)},
		Sandbox:         runmeta.SandboxInfo{Mode: "off"},
		Digest:          runmeta.DigestInfo{Path: filepath.Join(s.runsDir, "run_digest.json")},
		Status:          runmeta.RunStatus{Terminal: status},
		Product:         runmeta.ProductInfo{Version: Version, Commit: Commit, BuildDate: BuildDate},
	}
	if s.policyPack != "" {
		if pack, err := policypack.Get(s.policyPack); err == nil {
			manifest.PolicyPack = runmeta.PolicyPackInfo{Name: pack.Name, Version: pack.Version, Source: pack.Source}
		}
	}
	return runmeta.Save(filepath.Join(s.runsDir, "run_manifest.json"), manifest)
}

// refreshEvidence rewrites manifest/digest/report/index from what's been
// logged so far, so inspect/replay/verify reflect near-real-time state for a
// still-running session. Cheap: no full-repo re-copy, just re-deriving small
// artifacts already backed by the events log. status is the manifest's
// terminal-status label ("running" while active, "stopped" once shutdown
// has logged SENTINEL_STOP) — a caller-supplied value rather than a fixed
// "running" so shutdown's final refresh doesn't clobber its own status write.
func (s *sentinelSession) refreshEvidence(status string) {
	_ = s.writeManifest(status)
	if digest, err := runmeta.BuildDigest(s.sessionID, s.runsDir); err == nil {
		_ = runmeta.SaveDigest(filepath.Join(s.runsDir, "run_digest.json"), digest)
	}
	_ = report.Generate(s.runsDir, s.logger.EventsSnapshot())
	if store, err := index.Rebuild(filepath.Join(s.repoAbs, ".airlock", "runs")); err == nil {
		_ = index.Save(filepath.Join(s.repoAbs, ".airlock", "index.json"), store)
	}
}

// shutdown stops the recorder, finalizes evidence, and removes the process's
// lifecycle metadata. Order matters: the recorder must stop (and its
// pendingWG must drain — see recorder.Stop) before the final manifest/digest
// are written, so no in-flight debounced evaluation is lost or races the
// final write; SENTINEL_STOP is logged and evidence refreshed one last time
// before metadata is removed, so --status can never observe a "not running"
// state before evidence is actually consistent.
func (s *sentinelSession) shutdown() {
	s.stopFleet()
	if s.rec != nil {
		_ = s.rec.Stop()
	}
	s.logger.Add(events.Event{TS: time.Now().UTC(), Type: "SENTINEL_STOP", Summary: "sentinel session stopped", Meta: map[string]any{
		"session_id": s.sessionID, "repo": s.repoAbs,
	}})
	s.refreshEvidence("stopped")
	_ = s.logger.Close()
	removeSentinelMeta(s.repoAbs)
}

// --- Fleet reporting -------------------------------------------------------
//
// Everything below this point is asynchronous management traffic to an
// optional Airlock Fleet control plane (`airlock fleet serve`). It never
// participates in a filesystem-mutation decision: local governance
// (recorder + governance packages, above) has already allowed/denied/
// reverted a change before any of this code runs. This goroutine's only
// job is to periodically tell a control plane "I exist, here is my
// identity/version/policy, and here is what I've done" -- and to keep doing
// that indefinitely, tolerating any number of failures, for as long as the
// session runs.

// startFleet resolves this Sentinel's durable identity and launches the
// enroll+heartbeat goroutine. Any failure here (e.g. cannot resolve a home
// directory for the machine identity file) only disables fleet reporting
// for this run -- it never fails session startup, since Sentinel must work
// standalone regardless of fleet configuration or fleet reachability.
func (s *sentinelSession) startFleet(fleetURL, fleetToken string) {
	machineID, err := fleet.MachineID()
	if err != nil {
		fmt.Printf("WARN: fleet reporting disabled: could not establish machine identity: %v\n", err)
		return
	}
	sentinelID, err := fleet.SentinelID(s.repoAbs)
	if err != nil {
		fmt.Printf("WARN: fleet reporting disabled: could not establish sentinel identity: %v\n", err)
		return
	}
	s.sentinelID = sentinelID
	s.fleetMachineID = machineID
	s.fleetClient = fleet.NewClient(fleetURL, fleetToken)
	s.fleetStopCh = make(chan struct{})
	s.fleetDone = make(chan struct{})
	go s.fleetLoop()
}

// stopFleet signals the fleet goroutine to exit and waits briefly for it, so
// shutdown() does not race a final in-flight HTTP call and does not leak the
// goroutine past session end. It never blocks long: fleetStopCh is buffered
// by nothing but is always drained by fleetLoop's select within one HTTP
// round trip (bounded by fleet.ClientTimeout) or immediately if idle.
func (s *sentinelSession) stopFleet() {
	if s.fleetStopCh == nil {
		return
	}
	close(s.fleetStopCh)
	select {
	case <-s.fleetDone:
	case <-time.After(fleet.ClientTimeout + time.Second):
	}
}

// fleetLoop enrolls (with a bounded, exponentially-backed-off burst of
// attempts) and then heartbeats on a fixed interval indefinitely. If the
// initial burst does not succeed, it keeps retrying enrollment once per
// heartbeat tick forever -- never faster than DefaultHeartbeatInterval, so
// an unreachable control plane never becomes a tight retry loop, and a
// control plane that comes back later is reconnected to automatically with
// no Sentinel restart required. Every failure is logged and swallowed:
// nothing here ever returns an error to the caller or affects local
// enforcement.
func (s *sentinelSession) fleetLoop() {
	defer close(s.fleetDone)
	enrolled := s.fleetTryEnroll()
	ticker := time.NewTicker(fleetHeartbeatInterval())
	defer ticker.Stop()
	for {
		select {
		case <-s.fleetStopCh:
			return
		case <-ticker.C:
			if !enrolled {
				if err := s.fleetClient.Enroll(s.buildEnrollRequest()); err != nil {
					fmt.Printf("WARN: fleet enrollment retry failed: %v (still governing %s locally)\n", err, s.repoAbs)
					continue
				}
				enrolled = true
			}
			if err := s.fleetClient.Heartbeat(s.buildHeartbeatRequest()); err != nil {
				fmt.Printf("WARN: fleet heartbeat failed: %v (continuing local governance; retrying next interval)\n", err)
			}
		}
	}
}

// fleetTryEnroll makes a bounded, exponentially-backed-off burst of
// enrollment attempts at session startup. It returns quickly (false) if the
// control plane is unreachable rather than blocking indefinitely --
// fleetLoop's ticker takes over retrying afterward at a much slower, bounded
// rate.
func (s *sentinelSession) fleetTryEnroll() bool {
	backoff := fleetEnrollBackoffBase()
	for attempt := 0; attempt < fleet.MaxEnrollAttempts; attempt++ {
		if err := s.fleetClient.Enroll(s.buildEnrollRequest()); err == nil {
			return true
		} else if attempt == 0 {
			fmt.Printf("WARN: fleet control plane unreachable (%v); Sentinel continues local governance and will keep retrying enrollment.\n", err)
		}
		select {
		case <-time.After(backoff):
			if backoff < fleet.MaxEnrollBackoff {
				backoff *= 2
			}
		case <-s.fleetStopCh:
			return false
		}
	}
	return false
}

func (s *sentinelSession) buildEnrollRequest() fleet.EnrollRequest {
	policyID, policyVersion, policyHash := s.policyIdentity()
	return fleet.EnrollRequest{
		SentinelID:      s.sentinelID,
		MachineID:       s.fleetMachineID,
		Hostname:        hostName(),
		Platform:        runtime.GOOS + "/" + runtime.GOARCH,
		RepoPath:        s.repoAbs,
		SentinelVersion: Version,
		SessionID:       s.sessionID,
		StartedAt:       s.startedAt,
		PolicyID:        policyID,
		PolicyVersion:   policyVersion,
		PolicyHash:      policyHash,
	}
}

func (s *sentinelSession) buildHeartbeatRequest() fleet.HeartbeatRequest {
	policyID, policyVersion, policyHash := s.policyIdentity()
	allow, deny, reverted, revertFailed, lastEventAt := governanceCounters(s.logger.EventsSnapshot())
	return fleet.HeartbeatRequest{
		SentinelID:        s.sentinelID,
		SessionID:         s.sessionID,
		Status:            "running",
		Timestamp:         time.Now().UTC(),
		SentinelVersion:   Version,
		PolicyID:          policyID,
		PolicyVersion:     policyVersion,
		PolicyHash:        policyHash,
		LastEventAt:       lastEventAt,
		AllowCount:        allow,
		DenyCount:         deny,
		RevertedCount:     reverted,
		RevertFailedCount: revertFailed,
	}
}

// policyIdentity derives policy_id/policy_version/policy_hash from what this
// session already resolved. policy_hash is a stable digest of the effective
// write/read policy actually in force (not the raw config file, so
// formatting/comment changes don't spuriously change it) -- enough for a
// fleet operator to notice two Sentinels have diverging effective policy,
// without Prompt 14 needing to implement any policy distribution.
func (s *sentinelSession) policyIdentity() (id, version, hash string) {
	id = "local"
	if s.policyPack != "" {
		id = s.policyPack
		if pack, err := policypack.Get(s.policyPack); err == nil {
			version = pack.Version
		}
	}
	b, _ := json.Marshal(runmeta.BuildPolicySummary(s.policyPath, s.cfg))
	sum := sha256.Sum256(b)
	hash = hex.EncodeToString(sum[:])[:16]
	return id, version, hash
}

// governanceCounters summarizes evs the same way the Sentinel viewer's
// activity feed does (internal/web/sentinel.go): FILE_* events are allowed
// mutations; POLICY_DENY/APPROVAL_REQUIRED are denials, further split by
// whether the revert Meta recorded success or failure. lastEventAt is the
// timestamp of the most recent governance-relevant event, or nil if none
// have occurred yet.
func governanceCounters(evs []events.Event) (allow, deny, reverted, revertFailed int, lastEventAt *time.Time) {
	for i := range evs {
		e := evs[i]
		switch {
		case strings.HasPrefix(e.Type, "FILE_"):
			allow++
		case e.Type == "POLICY_DENY" || e.Type == "APPROVAL_REQUIRED":
			deny++
			failed := false
			if rv, ok := e.Meta["reverted"].(bool); ok {
				if rv {
					reverted++
				} else {
					failed = true
				}
			}
			if es, ok := e.Meta["revert_error"].(string); ok && es != "" {
				failed = true
			}
			if failed {
				revertFailed++
			}
		default:
			continue
		}
		ts := e.TS
		lastEventAt = &ts
	}
	return allow, deny, reverted, revertFailed, lastEventAt
}
