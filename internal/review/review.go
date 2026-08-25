package review

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type State string

const (
	StateUnreviewed     State = "unreviewed"
	StateApproved       State = "approved"
	StateRejected       State = "rejected"
	StateNeedsAttention State = "needs-attention"
)

type Record struct {
	State         State  `json:"state"`
	PreviousState State  `json:"previous_state,omitempty"`
	Note          string `json:"note,omitempty"`
	Reviewer      string `json:"reviewer,omitempty"`
	Timestamp     string `json:"timestamp,omitempty"`
}

func Path(runDir string) string { return filepath.Join(runDir, "review.json") }

func Load(runDir string) (Record, error) {
	p := Path(runDir)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return Record{State: StateUnreviewed}, nil
		}
		return Record{}, err
	}
	var r Record
	if err := json.Unmarshal(b, &r); err != nil {
		return Record{}, err
	}
	if r.State == "" {
		r.State = StateUnreviewed
	}
	return r, nil
}

func Save(runDir string, r Record) error {
	r.State = normalizeState(string(r.State))
	if r.Timestamp == "" {
		r.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(runDir), b, 0o644)
}

func normalizeState(s string) State {
	s = strings.TrimSpace(strings.ToLower(s))
	switch State(s) {
	case StateApproved, StateRejected, StateNeedsAttention:
		return State(s)
	default:
		return StateUnreviewed
	}
}
