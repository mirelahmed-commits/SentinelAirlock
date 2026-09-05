package fleet

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Server is the Airlock Fleet control plane's HTTP surface: Sentinel
// enrollment/heartbeat ingestion, inventory APIs, policy resource/assignment
// APIs (Prompt 14A), and a small operator UI.
//
// Deliberately absent, by design (fleet foundation only, see progress.md's
// Prompt 14/14A handoffs for what is explicitly deferred to 14B):
//   - no remote command execution of any kind
//   - no full authn/authz (Token is an optional shared-secret v0 trust
//     boundary, not enterprise identity)
//   - no policy signing/issuer trust chain (PolicyVersion reserves the
//     fields; nothing populates or verifies them yet)
type Server struct {
	store       *Store
	policyStore *PolicyStore
	token       string
}

// NewServer builds a Server over store and policyStore. token is an optional
// shared secret; if empty, every endpoint is unauthenticated -- an explicit,
// honest v0 trust boundary suited to a trusted local/private network, not a
// public one. See ServeCmd's --token flag and progress.md for the
// documented limitation.
func NewServer(store *Store, policyStore *PolicyStore, token string) *Server {
	return &Server{store: store, policyStore: policyStore, token: strings.TrimSpace(token)}
}

// Handler returns the complete fleet control-plane HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/fleet/enroll", s.handleEnroll)
	mux.HandleFunc("/api/fleet/heartbeat", s.handleHeartbeat)
	mux.HandleFunc("/api/fleet/sentinels", s.handleList)
	mux.HandleFunc("/api/fleet/sentinels/", s.handleDetailAPI)
	mux.HandleFunc("/api/fleet/policies", s.handlePoliciesRoot)
	mux.HandleFunc("/api/fleet/policies/", s.handlePoliciesSub)
	mux.HandleFunc("/fleet/sentinels/", s.handleDetailPage)
	return mux
}

func (s *Server) authorized(r *http.Request) bool {
	if s.token == "" {
		return true
	}
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if h == "Bearer "+s.token {
		return true
	}
	return strings.TrimSpace(r.Header.Get("X-Airlock-Fleet-Token")) == s.token
}

func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req EnrollRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		http.Error(w, "invalid enrollment payload", http.StatusBadRequest)
		return
	}
	req.SentinelID = strings.TrimSpace(req.SentinelID)
	req.MachineID = strings.TrimSpace(req.MachineID)
	if req.SentinelID == "" || req.MachineID == "" {
		http.Error(w, "sentinel_id and machine_id are required", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	rec := Record{
		SentinelID:      req.SentinelID,
		MachineID:       req.MachineID,
		Hostname:        req.Hostname,
		Platform:        req.Platform,
		RepoPath:        req.RepoPath,
		SentinelVersion: req.SentinelVersion,
		SessionID:       req.SessionID,
		Status:          "running",
		PolicyID:        req.PolicyID,
		PolicyVersion:   req.PolicyVersion,
		PolicyHash:      req.PolicyHash,
		StartedAt:       req.StartedAt,
		LastHeartbeat:   now,
		EnrolledAt:      now,
	}
	if _, err := s.store.UpsertEnroll(rec); err != nil {
		http.Error(w, "could not persist enrollment", http.StatusInternalServerError)
		return
	}
	writeJSON(w, EnrollResponse{SentinelID: req.SentinelID, Enrolled: true})
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req HeartbeatRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		http.Error(w, "invalid heartbeat payload", http.StatusBadRequest)
		return
	}
	req.SentinelID = strings.TrimSpace(req.SentinelID)
	if req.SentinelID == "" {
		http.Error(w, "sentinel_id is required", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	rec, err := s.store.UpsertHeartbeat(req.SentinelID, func(rec *Record) {
		rec.LastHeartbeat = now
		if req.SessionID != "" {
			rec.SessionID = req.SessionID
		}
		if req.Status != "" {
			rec.Status = req.Status
		}
		if req.SentinelVersion != "" {
			rec.SentinelVersion = req.SentinelVersion
		}
		if req.PolicyID != "" {
			rec.PolicyID = req.PolicyID
		}
		if req.PolicyVersion != "" {
			rec.PolicyVersion = req.PolicyVersion
		}
		if req.PolicyHash != "" {
			rec.PolicyHash = req.PolicyHash
		}
		if req.LastEventAt != nil {
			rec.LastEventAt = req.LastEventAt
		}
		rec.AllowCount = req.AllowCount
		rec.DenyCount = req.DenyCount
		rec.RevertedCount = req.RevertedCount
		rec.RevertFailedCount = req.RevertFailedCount

		// Reconcile self-report (Prompt 14A). An empty ReconcileStatus means
		// "nothing new to report" and must not erase a still-relevant
		// RECONCILE_FAILED from an earlier heartbeat -- only overwrite when
		// the Sentinel is actually telling us something.
		if req.ReconcileStatus != "" {
			rec.ReconcileStatus = req.ReconcileStatus
			rec.ReconcileError = req.ReconcileError
			rec.ReconcileForHash = req.ReconcileForHash
			rec.LastReconcileAt = &now
		}
	})
	if err != nil {
		http.Error(w, "could not persist heartbeat", http.StatusInternalServerError)
		return
	}
	writeJSON(w, HeartbeatResponse{
		Accepted:             true,
		DesiredPolicyID:      rec.DesiredPolicyID,
		DesiredPolicyVersion: rec.DesiredPolicyVersion,
		DesiredPolicyHash:    rec.DesiredPolicyHash,
	})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, s.snapshot())
}

func (s *Server) handleDetailAPI(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/fleet/sentinels/")
	if rest == "" {
		s.handleList(w, r)
		return
	}
	if id, ok := strings.CutSuffix(rest, "/assign"); ok {
		s.handleAssignPolicy(w, r, id)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rec, ok := s.store.Get(rest)
	if !ok {
		http.Error(w, "sentinel not found", http.StatusNotFound)
		return
	}
	writeJSON(w, newSentinelView(rec, time.Now().UTC()))
}

// assignPolicyRequest is the body of POST /api/fleet/sentinels/<id>/assign.
type assignPolicyRequest struct {
	PolicyID string `json:"policy_id"`
	Version  int    `json:"version"`
}

// handleAssignPolicy sets sentinelID's desired policy to a specific,
// already-existing version of a named policy (Prompt 14A). It never accepts
// raw policy content directly -- only a reference to a version created via
// POST /api/fleet/policies(/<id>/versions) -- so assignment can never bypass
// the validate-before-store step those endpoints already perform.
func (s *Server) handleAssignPolicy(w http.ResponseWriter, r *http.Request, sentinelID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req assignPolicyRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&req); err != nil {
		http.Error(w, "invalid assignment payload", http.StatusBadRequest)
		return
	}
	req.PolicyID = strings.TrimSpace(req.PolicyID)
	if req.PolicyID == "" || req.Version <= 0 {
		http.Error(w, "policy_id and a positive version are required", http.StatusBadRequest)
		return
	}
	pv, ok := s.policyStore.GetVersion(req.PolicyID, req.Version)
	if !ok {
		http.Error(w, fmt.Sprintf("policy %s version %d does not exist", req.PolicyID, req.Version), http.StatusNotFound)
		return
	}
	rec, err := s.store.AssignPolicy(sentinelID, PolicyRef{PolicyID: pv.PolicyID, Version: pv.Version, Hash: pv.Hash})
	if err != nil {
		http.Error(w, "could not persist assignment", http.StatusInternalServerError)
		return
	}
	writeJSON(w, newSentinelView(rec, time.Now().UTC()))
}

func (s *Server) snapshot() Snapshot {
	now := time.Now().UTC()
	recs := s.store.List()
	resp := Snapshot{Now: now, Sentinels: make([]SentinelView, 0, len(recs))}
	for _, r := range recs {
		view := newSentinelView(r, now)
		if view.Health == "ACTIVE" {
			resp.Active++
		} else {
			resp.Offline++
		}
		resp.Sentinels = append(resp.Sentinels, view)
	}
	sort.Slice(resp.Sentinels, func(i, j int) bool {
		return resp.Sentinels[i].SentinelID < resp.Sentinels[j].SentinelID
	})
	return resp
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// --- Policy resource APIs (Prompt 14A) --------------------------------------
//
// Policy content always arrives as a YAML string in a JSON request body --
// never as a filesystem path -- so the control plane can never be asked to
// read an arbitrary local file. Every write validates by attempting to
// parse the content (ComputePolicyHash) before it is ever stored; invalid
// content is rejected with 400 and never becomes a version.

// policyVersionSummary omits YAML content, for list views.
type policyVersionSummary struct {
	PolicyID    string    `json:"policy_id"`
	Version     int       `json:"version"`
	Hash        string    `json:"hash"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func summarize(v PolicyVersion) policyVersionSummary {
	return policyVersionSummary{PolicyID: v.PolicyID, Version: v.Version, Hash: v.Hash, Description: v.Description, CreatedAt: v.CreatedAt}
}

type createPolicyRequest struct {
	PolicyID    string `json:"policy_id"`
	Description string `json:"description,omitempty"`
	YAML        string `json:"yaml"`
}

// handlePoliciesRoot serves GET (list latest version of every policy) and
// POST (create a brand-new policy_id's first version) on /api/fleet/policies.
func (s *Server) handlePoliciesRoot(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		latest := s.policyStore.ListLatest()
		summaries := make([]policyVersionSummary, 0, len(latest))
		for _, v := range latest {
			summaries = append(summaries, summarize(v))
		}
		writeJSON(w, summaries)
	case http.MethodPost:
		var req createPolicyRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
			http.Error(w, "invalid policy payload", http.StatusBadRequest)
			return
		}
		req.PolicyID = strings.TrimSpace(req.PolicyID)
		if req.PolicyID == "" || strings.TrimSpace(req.YAML) == "" {
			http.Error(w, "policy_id and yaml are required", http.StatusBadRequest)
			return
		}
		v, err := s.policyStore.Create(req.PolicyID, req.Description, req.YAML)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, summarize(v))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handlePoliciesSub serves the /api/fleet/policies/<id>[/versions[/<v>]]
// family:
//
//	GET  /api/fleet/policies/<id>                 all versions (summaries)
//	POST /api/fleet/policies/<id>/versions         add a new version
//	GET  /api/fleet/policies/<id>/versions/<v>     one version, full content
func (s *Server) handlePoliciesSub(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/fleet/policies/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	policyID := parts[0]

	switch {
	case len(parts) == 1:
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		versions, ok := s.policyStore.AllVersions(policyID)
		if !ok {
			http.Error(w, "policy not found", http.StatusNotFound)
			return
		}
		summaries := make([]policyVersionSummary, 0, len(versions))
		for _, v := range versions {
			summaries = append(summaries, summarize(v))
		}
		writeJSON(w, summaries)

	case len(parts) == 2 && parts[1] == "versions":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req createPolicyRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
			http.Error(w, "invalid policy payload", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.YAML) == "" {
			http.Error(w, "yaml is required", http.StatusBadRequest)
			return
		}
		v, err := s.policyStore.AddVersion(policyID, req.Description, req.YAML)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, summarize(v))

	case len(parts) == 3 && parts[1] == "versions":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		version, err := strconv.Atoi(parts[2])
		if err != nil || version <= 0 {
			http.Error(w, "invalid version number", http.StatusBadRequest)
			return
		}
		v, ok := s.policyStore.GetVersion(policyID, version)
		if !ok {
			http.Error(w, "policy version not found", http.StatusNotFound)
			return
		}
		writeJSON(w, v) // full content, including YAML -- this is what Sentinel fetches

	default:
		http.NotFound(w, r)
	}
}
