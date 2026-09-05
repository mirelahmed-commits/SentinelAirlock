// Package fleet implements the Airlock Fleet control plane: a minimal
// coordination/inventory service that tracks which Sentinels exist, what
// they govern, whether they are alive, and their recently-reported policy
// and governance health.
//
// The control plane is explicitly NOT in the filesystem-policy decision
// path. A Sentinel allows/denies/reverts filesystem mutations entirely from
// its own local policy engine (internal/recorder + internal/governance);
// enrollment and heartbeats are asynchronous, best-effort management
// traffic layered on top, and their failure must never affect local
// enforcement. See internal/cli/sentinel.go's fleet integration for how
// that boundary is enforced (a goroutine independent of the recorder).
package fleet

import "time"

const (
	// DefaultHeartbeatInterval is how often a healthy, reachable Sentinel
	// sends a heartbeat to an enrolled control plane. Within the 5-15s
	// range called for by the fleet-foundation design: frequent enough for
	// the UI to feel live, infrequent enough to never be mistaken for a
	// per-filesystem-event reporting channel (it is not one).
	DefaultHeartbeatInterval = 10 * time.Second

	// OfflineThreshold is how stale a Sentinel's last heartbeat must be
	// before the control plane reports it as OFFLINE instead of ACTIVE.
	// Set to 3x the heartbeat interval so a single delayed/dropped
	// heartbeat (GC pause, transient network blip) does not flap a healthy
	// Sentinel to OFFLINE and back.
	OfflineThreshold = 3 * DefaultHeartbeatInterval

	// MaxEnrollAttempts bounds the Sentinel-side enrollment retry burst at
	// startup so an unreachable control plane never becomes an unbounded
	// retry storm. After this burst, the Sentinel keeps retrying
	// enrollment at the (much slower) heartbeat-tick cadence indefinitely
	// -- see internal/cli/sentinel.go's fleetLoop -- which is how a
	// Sentinel reconnects to a control plane that comes back later without
	// needing to be restarted.
	MaxEnrollAttempts = 5

	// MaxEnrollBackoff caps exponential backoff between the bounded burst
	// of startup enrollment attempts.
	MaxEnrollBackoff = 30 * time.Second

	// ClientTimeout bounds every single fleet HTTP call. It exists so a
	// hung or slow-to-respond control plane can never stall a Sentinel's
	// fleet-reporting goroutine for long, let alone its (entirely separate)
	// local enforcement loop.
	ClientTimeout = 3 * time.Second
)

// EnrollRequest establishes a Sentinel's identity with the control plane.
// It intentionally carries no repository contents and no raw evidence --
// only identity, version, and reported-policy metadata. Fleet v0 is
// inventory/health, not centralized data exfiltration.
type EnrollRequest struct {
	SentinelID      string    `json:"sentinel_id"`
	MachineID       string    `json:"machine_id"`
	Hostname        string    `json:"hostname"`
	Platform        string    `json:"platform"`
	RepoPath        string    `json:"repo_path"`
	SentinelVersion string    `json:"sentinel_version"`
	SessionID       string    `json:"session_id"`
	StartedAt       time.Time `json:"started_at"`
	PolicyID        string    `json:"policy_id,omitempty"`
	PolicyVersion   string    `json:"policy_version,omitempty"`
	PolicyHash      string    `json:"policy_hash,omitempty"`
}

// EnrollResponse acknowledges enrollment.
type EnrollResponse struct {
	SentinelID string `json:"sentinel_id"`
	Enrolled   bool   `json:"enrolled"`
}

// HeartbeatRequest is small, frequent, and carries no file contents, diffs,
// or raw evidence -- only counters and a reference (SessionID) to evidence
// that stays local to the Sentinel's own machine and remains inspectable
// there with the existing `airlock inspect/replay/verify` commands. Central
// evidence aggregation is out of scope for the fleet foundation.
type HeartbeatRequest struct {
	SentinelID        string     `json:"sentinel_id"`
	SessionID         string     `json:"session_id"`
	Status            string     `json:"status"`
	Timestamp         time.Time  `json:"timestamp"`
	SentinelVersion   string     `json:"sentinel_version"`
	PolicyID          string     `json:"policy_id,omitempty"`
	PolicyVersion     string     `json:"policy_version,omitempty"`
	PolicyHash        string     `json:"policy_hash,omitempty"`
	LastEventAt       *time.Time `json:"last_event_at,omitempty"`
	AllowCount        int        `json:"allow_count"`
	DenyCount         int        `json:"deny_count"`
	RevertedCount     int        `json:"reverted_count"`
	RevertFailedCount int        `json:"revert_failed_count"`
}

// HeartbeatResponse acknowledges a heartbeat.
type HeartbeatResponse struct {
	Accepted bool `json:"accepted"`
}

// Record is the control plane's durable view of one Sentinel installation.
//
// Identity vs session, made explicit:
//   - SentinelID is a durable identity for "the Sentinel governing this
//     repository on this machine." It survives Sentinel restarts (stop/
//     start, background/foreground toggling, crash recovery) -- see
//     internal/fleet/identity.go. Restarting Sentinel does not create a new
//     fleet inventory entry.
//   - SessionID is the existing per-run monitoring-session identity
//     (internal/cli/sentinel.go), already used to name each session's
//     evidence directory (.airlock/runs/<session-id>/). It intentionally
//     changes every time Sentinel (re)starts, independent of SentinelID.
//
// EnrolledAt is sticky: it is set the first time this SentinelID is ever
// seen and preserved across every later re-enrollment. StartedAt reflects
// the current/most recent process start and is refreshed on every
// enrollment (i.e. every Sentinel start).
type Record struct {
	SentinelID        string     `json:"sentinel_id"`
	MachineID         string     `json:"machine_id"`
	Hostname          string     `json:"hostname"`
	Platform          string     `json:"platform"`
	RepoPath          string     `json:"repo_path"`
	SentinelVersion   string     `json:"sentinel_version"`
	SessionID         string     `json:"session_id"`
	Status            string     `json:"status"`
	PolicyID          string     `json:"policy_id,omitempty"`
	PolicyVersion     string     `json:"policy_version,omitempty"`
	PolicyHash        string     `json:"policy_hash,omitempty"`
	EnrolledAt        time.Time  `json:"enrolled_at"`
	StartedAt         time.Time  `json:"started_at"`
	LastHeartbeat     time.Time  `json:"last_heartbeat"`
	LastEventAt       *time.Time `json:"last_event_at,omitempty"`
	AllowCount        int        `json:"allow_count"`
	DenyCount         int        `json:"deny_count"`
	RevertedCount     int        `json:"reverted_count"`
	RevertFailedCount int        `json:"revert_failed_count"`
}

// Health derives ACTIVE/OFFLINE from heartbeat freshness against now, rather
// than trusting a Sentinel's self-reported Status forever once it has
// reported in -- the fleet-foundation's explicit inventory requirement.
// There is no DEGRADED state in v0: it was left out rather than invented
// without a concrete signal to justify it (see progress.md's Prompt 14
// handoff for what a future DEGRADED signal could be based on).
func Health(rec Record, now time.Time) string {
	if rec.LastHeartbeat.IsZero() || now.Sub(rec.LastHeartbeat) > OfflineThreshold {
		return "OFFLINE"
	}
	return "ACTIVE"
}

// SentinelView is a Record plus its computed Health, as returned by the
// inventory APIs.
type SentinelView struct {
	Record
	Health string `json:"health"`
}

// Snapshot is the fleet inventory at a point in time: summary counts plus
// every known Sentinel (offline ones are retained with last-seen
// information, never silently dropped).
type Snapshot struct {
	Now       time.Time      `json:"now"`
	Active    int            `json:"active"`
	Offline   int            `json:"offline"`
	Sentinels []SentinelView `json:"sentinels"`
}
