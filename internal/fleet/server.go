package fleet

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Server is the Airlock Fleet control plane's HTTP surface: Sentinel
// enrollment/heartbeat ingestion, inventory APIs, and a small operator UI.
//
// Deliberately absent, by design (fleet foundation only, see progress.md's
// Prompt 14 handoff for what is explicitly deferred to 14A/14B):
//   - no policy distribution/reconciliation endpoint
//   - no remote command execution of any kind
//   - no full authn/authz (Token is an optional shared-secret v0 trust
//     boundary, not enterprise identity)
type Server struct {
	store *Store
	token string
}

// NewServer builds a Server over store. token is an optional shared secret;
// if empty, every endpoint is unauthenticated -- an explicit, honest v0
// trust boundary suited to a trusted local/private network, not a public
// one. See ServeCmd's --token flag and progress.md for the documented
// limitation.
func NewServer(store *Store, token string) *Server {
	return &Server{store: store, token: strings.TrimSpace(token)}
}

// Handler returns the complete fleet control-plane HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/fleet/enroll", s.handleEnroll)
	mux.HandleFunc("/api/fleet/heartbeat", s.handleHeartbeat)
	mux.HandleFunc("/api/fleet/sentinels", s.handleList)
	mux.HandleFunc("/api/fleet/sentinels/", s.handleDetailAPI)
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
	_, err := s.store.UpsertHeartbeat(req.SentinelID, func(rec *Record) {
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
	})
	if err != nil {
		http.Error(w, "could not persist heartbeat", http.StatusInternalServerError)
		return
	}
	writeJSON(w, HeartbeatResponse{Accepted: true})
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
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/fleet/sentinels/")
	if id == "" {
		s.handleList(w, r)
		return
	}
	rec, ok := s.store.Get(id)
	if !ok {
		http.Error(w, "sentinel not found", http.StatusNotFound)
		return
	}
	writeJSON(w, SentinelView{Record: rec, Health: Health(rec, time.Now().UTC())})
}

func (s *Server) snapshot() Snapshot {
	now := time.Now().UTC()
	recs := s.store.List()
	resp := Snapshot{Now: now, Sentinels: make([]SentinelView, 0, len(recs))}
	for _, r := range recs {
		h := Health(r, now)
		if h == "ACTIVE" {
			resp.Active++
		} else {
			resp.Offline++
		}
		resp.Sentinels = append(resp.Sentinels, SentinelView{Record: r, Health: h})
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
