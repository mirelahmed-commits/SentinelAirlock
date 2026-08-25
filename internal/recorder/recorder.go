package recorder

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/yourname/sentinel-airlock/internal/events"
	"github.com/yourname/sentinel-airlock/internal/governance"
	"github.com/yourname/sentinel-airlock/internal/policy"
)

type Recorder struct {
	root         string
	cfg          *policy.Config
	log          *events.Logger
	approvalMode governance.ApprovalMode

	w      *fsnotify.Watcher
	stopCh chan struct{}
	wg     sync.WaitGroup

	mu            sync.Mutex
	lastBytes     map[string][]byte
	suppressUntil map[string]time.Time
}

func New(root string, log *events.Logger, cfg *policy.Config, approvalMode governance.ApprovalMode) (*Recorder, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Recorder{
		root:          root,
		cfg:           cfg,
		log:           log,
		approvalMode:  approvalMode,
		w:             w,
		stopCh:        make(chan struct{}),
		lastBytes:     map[string][]byte{},
		suppressUntil: map[string]time.Time{},
	}, nil
}

func (r *Recorder) Start() error {
	// add watchers recursively
	if err := filepath.WalkDir(r.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel := filepath.ToSlash(mustRel(r.root, path))
		if rel != "." && r.shouldIgnore(rel, d.IsDir()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			_ = r.w.Add(path)
		}
		return nil
	}); err != nil {
		return err
	}

	r.wg.Add(1)
	go r.loop()
	return nil
}

func (r *Recorder) Stop() error {
	close(r.stopCh)
	r.wg.Wait()
	return r.w.Close()
}

func (r *Recorder) loop() {
	defer r.wg.Done()
	dmp := diffmatchpatch.New()

	for {
		select {
		case <-r.stopCh:
			return
		case ev, ok := <-r.w.Events:
			if !ok {
				return
			}
			// Handle new dirs to watch
			if ev.Op&fsnotify.Create == fsnotify.Create {
				if st, err := os.Stat(ev.Name); err == nil && st.IsDir() {
					_ = r.w.Add(ev.Name)
					continue
				}
			}

			rel := filepath.ToSlash(mustRel(r.root, ev.Name))
			if rel == "." || rel == "" {
				continue
			}
			if r.shouldIgnore(rel, false) {
				continue
			}
			if r.isSuppressed(rel) {
				continue
			}

			// Record create/write/rename/remove for v0.
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
				continue
			}

			before := r.getLast(rel)
			after, _ := os.ReadFile(ev.Name) // if removed, read fails -> empty

			assessment := governance.ClassifyFilesystem(rel, opName(ev.Op), r.cfg)
			approvalDecision := governance.Decide(r.approvalMode, assessment)
			if approvalDecision != governance.DecisionAllow {
				// compute attempted diff for logging
				diffText := ""
				patches := dmp.PatchMake(string(before), string(after))
				if len(patches) > 0 {
					diffText = dmp.PatchToText(patches)
				}

				// revert: restore previous bytes if existed, else delete newly created file
				r.markSuppress(rel, 500*time.Millisecond)
				if len(before) > 0 {
					_ = os.WriteFile(ev.Name, before, 0o644)
				} else {
					_ = os.Remove(ev.Name)
				}

				// keep cache as "before"
				r.setLast(rel, before)

				evType := "POLICY_DENY"
				summary := "write blocked and reverted"
				if approvalDecision == governance.DecisionPrompt {
					evType = "APPROVAL_REQUIRED"
					summary = "write requires approval and was reverted"
				}
				r.log.Add(events.Event{
					TS:      time.Now().UTC(),
					Type:    evType,
					Path:    rel,
					Summary: summary,
					Diff:    diffText,
					Meta: map[string]any{
						"op": ev.Op.String(),
					},
					Risk: map[string]any{
						"level":    string(assessment.Level),
						"category": string(assessment.Category),
						"reason":   assessment.Reason,
					},
					Approval: map[string]any{
						"mode":     string(r.approvalMode),
						"decision": string(approvalDecision),
					},
				})

				continue
			}

			// allowed: update cache normally
			r.setLast(rel, after)

			// diff + event logging (your existing logic continues here)
			diffText := ""
			patches := dmp.PatchMake(string(before), string(after))
			if len(patches) > 0 {
				diffText = dmp.PatchToText(patches)
			}

			r.log.Add(events.Event{
				TS:      time.Now().UTC(),
				Type:    eventType(ev.Op),
				Path:    rel,
				Summary: "workspace change",
				Diff:    diffText,
				Risk: map[string]any{
					"level":    string(assessment.Level),
					"category": string(assessment.Category),
					"reason":   assessment.Reason,
				},
				Approval: map[string]any{
					"mode":     string(r.approvalMode),
					"decision": string(governance.DecisionAllow),
				},
			})
		case err, ok := <-r.w.Errors:
			if !ok {
				return
			}
			r.log.Add(events.Event{
				TS:      time.Now().UTC(),
				Type:    "RECORDER_ERROR",
				Summary: err.Error(),
			})
		}
	}
}

func (r *Recorder) shouldIgnore(rel string, isDir bool) bool {
	rel = strings.TrimPrefix(rel, "./")
	// built-in ignores
	if rel == ".git" || strings.HasPrefix(rel, ".git/") {
		return true
	}
	if rel == ".airlock" || strings.HasPrefix(rel, ".airlock/") {
		return true
	}
	if rel == "node_modules" || strings.HasPrefix(rel, "node_modules/") {
		return true
	}
	// config ignores (simple handling)
	if r.cfg != nil {
		for _, g := range r.cfg.Workspace.Ignore {
			g = filepath.ToSlash(strings.TrimSpace(g))
			if g == "" {
				continue
			}
			if strings.HasSuffix(g, "/**") {
				p := strings.TrimSuffix(g, "/**")
				if rel == p || strings.HasPrefix(rel, p+"/") {
					return true
				}
			}
			if strings.HasPrefix(g, "**/") {
				s := strings.TrimPrefix(g, "**/")
				if rel == s || strings.HasSuffix(rel, "/"+s) {
					return true
				}
			}
			if rel == g {
				return true
			}
		}
	}
	return false
}

func (r *Recorder) getLast(rel string) []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]byte(nil), r.lastBytes[rel]...)
}

func (r *Recorder) setLast(rel string, b []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastBytes[rel] = append([]byte(nil), b...)
}

func (r *Recorder) markSuppress(rel string, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.suppressUntil[rel] = time.Now().UTC().Add(d)
}

func (r *Recorder) isSuppressed(rel string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	until, ok := r.suppressUntil[rel]
	if !ok {
		return false
	}
	if time.Now().UTC().Before(until) {
		return true
	}
	delete(r.suppressUntil, rel)
	return false
}

func eventType(op fsnotify.Op) string {
	switch {
	case op&fsnotify.Write == fsnotify.Write:
		return "FILE_WRITE"
	case op&fsnotify.Remove == fsnotify.Remove:
		return "FILE_REMOVE"
	case op&fsnotify.Rename == fsnotify.Rename:
		return "FILE_RENAME"
	case op&fsnotify.Create == fsnotify.Create:
		return "FILE_CREATE"
	default:
		return "FILE_EVENT"
	}
}

func opName(op fsnotify.Op) string {
	switch {
	case op&fsnotify.Write == fsnotify.Write:
		return "WRITE"
	case op&fsnotify.Remove == fsnotify.Remove:
		return "REMOVE"
	case op&fsnotify.Rename == fsnotify.Rename:
		return "RENAME"
	case op&fsnotify.Create == fsnotify.Create:
		return "CREATE"
	default:
		return "EVENT"
	}
}

func mustRel(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}
