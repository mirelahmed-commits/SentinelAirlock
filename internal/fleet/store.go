package fleet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Store is the control plane's durable inventory: a single JSON file
// guarded by an in-memory mutex, following the same plain-JSON-file
// convention already used throughout this project for local state
// (.airlock/index.json, .airlock/sentinel.json, .airlock/viewer.json) --
// see progress.md's Prompt 14 handoff for why SQLite was evaluated and not
// chosen for v0. Write volume is bounded by one heartbeat per Sentinel per
// DefaultHeartbeatInterval (not per filesystem event), so a whole-file
// rewrite per update is not a bottleneck at the fleet sizes this control
// plane targets.
type Store struct {
	mu      sync.RWMutex
	path    string
	records map[string]Record
}

// OpenStore loads path if it exists, or starts an empty, durable store that
// will create path on first write. A missing file is not an error -- it is
// simply an empty fleet.
func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, records: map[string]Record{}}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return s, nil
	}
	var recs map[string]Record
	if err := json.Unmarshal(b, &recs); err != nil {
		return nil, err
	}
	if recs != nil {
		s.records = recs
	}
	return s, nil
}

// UpsertEnroll records or refreshes a Sentinel's full identity at (re)start.
// rec should be fully populated by the caller (the HTTP handler); EnrolledAt
// is preserved from the first time this SentinelID was ever seen, overriding
// whatever rec.EnrolledAt was set to, so restarting a Sentinel does not reset
// "how long has this installation existed."
func (s *Store) UpsertEnroll(rec Record) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.records[rec.SentinelID]; ok && !existing.EnrolledAt.IsZero() {
		rec.EnrolledAt = existing.EnrolledAt
	}
	s.records[rec.SentinelID] = rec
	if err := s.saveLocked(); err != nil {
		return Record{}, err
	}
	return rec, nil
}

// UpsertHeartbeat applies apply to the existing record for id, or to a new,
// mostly-empty record if id has never been seen (e.g. the control plane's
// state was reset, or a heartbeat arrives before its own enrollment call
// completes). This keeps a Sentinel visible in inventory -- even if
// incompletely described until its next successful enrollment -- rather
// than dropping heartbeats for an unrecognized identity.
func (s *Store) UpsertHeartbeat(id string, apply func(*Record)) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.records[id]
	if rec.SentinelID == "" {
		rec.SentinelID = id
	}
	apply(&rec)
	s.records[id] = rec
	if err := s.saveLocked(); err != nil {
		return Record{}, err
	}
	return rec, nil
}

// Get returns the record for id, if known.
func (s *Store) Get(id string) (Record, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.records[id]
	return rec, ok
}

// List returns every known Sentinel record, sorted by SentinelID for stable
// output. Offline Sentinels are included -- List never expires entries;
// only Health() (computed by the caller against the current time) reports
// staleness.
func (s *Store) List() []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Record, 0, len(s.records))
	for _, r := range s.records {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SentinelID < out[j].SentinelID })
	return out
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}
