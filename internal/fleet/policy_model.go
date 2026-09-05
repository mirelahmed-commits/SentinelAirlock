package fleet

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mirelahmed-commits/SentinelAirlock/internal/policy"
	"gopkg.in/yaml.v3"
)

// PolicyVersion is one immutable, centrally-managed version of a named
// policy. Content is stored as the raw YAML text of an airlock.yaml-shaped
// document (internal/policy.Config) -- the exact same shape a Sentinel
// already knows how to load locally, so applying a fetched PolicyVersion
// requires no new parsing logic, only a new source for it.
//
// Prepared for Prompt 14B without a destructive redesign: Signature/Issuer/
// IssuedAt are reserved (json-omitted while empty) rather than bolted on
// later as a schema migration.
type PolicyVersion struct {
	PolicyID    string    `json:"policy_id"`
	Version     int       `json:"version"`
	Hash        string    `json:"hash"`
	YAML        string    `json:"yaml"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`

	// Reserved for Prompt 14B (trust/signing). Deliberately present-but-empty
	// now so adding real values later is additive, not a schema break.
	Signature string    `json:"signature,omitempty"`
	Issuer    string    `json:"issuer,omitempty"`
	IssuedAt  time.Time `json:"issued_at,omitempty"`
}

// PolicyRef names a specific version of a named policy -- what gets assigned
// to a Sentinel as its desired state, and what a Sentinel reports back as
// its actual state.
type PolicyRef struct {
	PolicyID string `json:"policy_id"`
	Version  int    `json:"version"`
	Hash     string `json:"hash,omitempty"`
}

func (r PolicyRef) Empty() bool { return r.PolicyID == "" }

// Equal reports whether two refs name the same policy content. Hash is the
// authoritative equality check (it's what actually changes when content
// changes); Version is compared too so a hash collision alone can never
// silently mask a version mismatch in the (extremely unlikely) case one
// occurs.
func (r PolicyRef) Equal(other PolicyRef) bool {
	return r.PolicyID == other.PolicyID && r.Version == other.Version && r.Hash == other.Hash
}

// ComputePolicyHash parses yamlContent as an airlock.yaml-shaped document
// and returns a deterministic content hash plus the parsed config, or an
// error if the content does not parse. The hash is computed from the parsed
// *policy.Config re-marshaled to JSON -- not from the raw YAML text -- so
// whitespace, comments, and key-order differences in equivalent documents
// never produce different hashes, and two independently-typed-out YAML
// files with the same effective policy hash identically. This is called
// both server-side (at policy creation) and Sentinel-side (after fetching,
// to verify the content matches what the server claims before installing
// it) using the exact same algorithm, so a mismatch reliably indicates
// either corruption or a version that has genuinely changed.
func ComputePolicyHash(yamlContent string) (string, *policy.Config, error) {
	var cfg policy.Config
	if err := yaml.Unmarshal([]byte(yamlContent), &cfg); err != nil {
		return "", nil, fmt.Errorf("invalid policy document: %w", err)
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:16], &cfg, nil
}

// ReconcileState derives a Sentinel's policy sync status for display,
// analogous to Health() for liveness: computed fresh from the record's
// current desired/actual/reported-error fields rather than trusted as a
// standing flag. See Store.AssignPolicy for how Desired* is set and
// internal/cli/sentinel.go's fleetLoop for how Actual*/Reconcile* get
// reported.
//
//   - "" (unmanaged): no desired policy has ever been assigned.
//   - RECONCILING: the Sentinel has told us it is actively fetching/applying
//     the current desired ref right now.
//   - RECONCILE_FAILED: the Sentinel tried the *current* desired ref and
//     failed; ReconcileError explains why. Reported only against the
//     specific hash it failed for (ReconcileForHash), so assigning a new
//     desired policy after a failure clears the stale error back to DRIFTED
//     instead of leaving an old failure message stuck forever.
//   - IN_SYNC: actual matches desired (policy_id, version, and hash all
//     agree).
//   - DRIFTED: anything else -- including the normal, brief window between
//     an assignment and the Sentinel's next successful reconciliation.
func ReconcileState(rec Record) (status string, errMsg string) {
	desired := PolicyRef{PolicyID: rec.DesiredPolicyID, Version: rec.DesiredPolicyVersion, Hash: rec.DesiredPolicyHash}
	if desired.Empty() {
		return "", ""
	}
	// Ground truth first: if actual already matches desired, that wins over
	// any stale self-reported RECONCILING/RECONCILE_FAILED from an earlier
	// attempt against the same hash (e.g. a transient failure that
	// succeeded on retry) -- the Sentinel is not required to explicitly
	// "clear" its own prior status report for this to display correctly.
	actual := PolicyRef{PolicyID: rec.PolicyID, Version: actualVersionAsInt(rec.PolicyVersion), Hash: rec.PolicyHash}
	if actual.Equal(desired) {
		return "IN_SYNC", ""
	}
	if rec.ReconcileStatus == "RECONCILING" {
		return "RECONCILING", ""
	}
	if rec.ReconcileStatus == "RECONCILE_FAILED" && rec.ReconcileForHash == desired.Hash {
		return "RECONCILE_FAILED", rec.ReconcileError
	}
	return "DRIFTED", ""
}

func actualVersionAsInt(v string) int {
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return -1 // non-numeric actual version (e.g. a local policy-pack version string) can never equal a desired int version
		}
		n = n*10 + int(c-'0')
	}
	if v == "" {
		return -1
	}
	return n
}
