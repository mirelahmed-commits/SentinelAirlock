package fleet

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func newTestServer(t *testing.T, token string) (*Server, *Store) {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "fleet.json"))
	if err != nil {
		t.Fatal(err)
	}
	return NewServer(store, token), store
}

func doJSON(t *testing.T, srv *Server, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	return rr
}

func TestServer_Enroll_ThenAppearsInInventory(t *testing.T) {
	srv, _ := newTestServer(t, "")
	rr := doJSON(t, srv, http.MethodPost, "/api/fleet/enroll", EnrollRequest{
		SentinelID: "sen-1", MachineID: "mach-1", RepoPath: "/repo/a", Hostname: "host-a",
		Platform: "darwin/arm64", SentinelVersion: "dev", SessionID: "sess-1", StartedAt: time.Now().UTC(),
	}, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("enroll status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = doJSON(t, srv, http.MethodGet, "/api/fleet/sentinels", nil, "")
	var snap Snapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.Sentinels) != 1 || snap.Sentinels[0].SentinelID != "sen-1" {
		t.Fatalf("expected enrolled sentinel in inventory, got %+v", snap)
	}
	if snap.Active != 1 || snap.Offline != 0 {
		t.Fatalf("expected freshly-enrolled sentinel to count as active: %+v", snap)
	}
}

func TestServer_Heartbeat_UpdatesLastSeenAndStatus(t *testing.T) {
	srv, store := newTestServer(t, "")
	doJSON(t, srv, http.MethodPost, "/api/fleet/enroll", EnrollRequest{SentinelID: "sen-1", MachineID: "mach-1"}, "")

	rr := doJSON(t, srv, http.MethodPost, "/api/fleet/heartbeat", HeartbeatRequest{
		SentinelID: "sen-1", SessionID: "sess-2", Status: "running", Timestamp: time.Now().UTC(),
		AllowCount: 3, DenyCount: 1, RevertedCount: 1,
	}, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", rr.Code, rr.Body.String())
	}
	rec, ok := store.Get("sen-1")
	if !ok {
		t.Fatal("expected record to exist after heartbeat")
	}
	if rec.SessionID != "sess-2" || rec.AllowCount != 3 || rec.DenyCount != 1 || rec.RevertedCount != 1 {
		t.Fatalf("heartbeat did not update record as expected: %+v", rec)
	}
}

func TestServer_RecentHeartbeat_ReportsActive(t *testing.T) {
	srv, _ := newTestServer(t, "")
	doJSON(t, srv, http.MethodPost, "/api/fleet/enroll", EnrollRequest{SentinelID: "sen-1", MachineID: "mach-1"}, "")
	doJSON(t, srv, http.MethodPost, "/api/fleet/heartbeat", HeartbeatRequest{SentinelID: "sen-1", Timestamp: time.Now().UTC()}, "")

	rr := doJSON(t, srv, http.MethodGet, "/api/fleet/sentinels/sen-1", nil, "")
	var view SentinelView
	if err := json.Unmarshal(rr.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Health != "ACTIVE" {
		t.Fatalf("expected ACTIVE after a fresh heartbeat, got %s", view.Health)
	}
}

func TestServer_StaleHeartbeat_ReportsOffline(t *testing.T) {
	srv, store := newTestServer(t, "")
	stale := time.Now().UTC().Add(-OfflineThreshold - time.Minute)
	if _, err := store.UpsertEnroll(Record{SentinelID: "sen-1", MachineID: "mach-1", EnrolledAt: stale, LastHeartbeat: stale}); err != nil {
		t.Fatal(err)
	}

	rr := doJSON(t, srv, http.MethodGet, "/api/fleet/sentinels/sen-1", nil, "")
	var view SentinelView
	if err := json.Unmarshal(rr.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Health != "OFFLINE" {
		t.Fatalf("expected OFFLINE for a stale heartbeat, got %s", view.Health)
	}

	rr = doJSON(t, srv, http.MethodGet, "/api/fleet/sentinels", nil, "")
	var snap Snapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Offline != 1 {
		t.Fatalf("expected offline sentinel to remain in inventory with Offline=1, got %+v", snap)
	}
}

func TestServer_MultipleSentinelsEnroll(t *testing.T) {
	srv, _ := newTestServer(t, "")
	for _, id := range []string{"sen-a", "sen-b"} {
		rr := doJSON(t, srv, http.MethodPost, "/api/fleet/enroll", EnrollRequest{SentinelID: id, MachineID: "mach-1", RepoPath: "/repo/" + id}, "")
		if rr.Code != http.StatusOK {
			t.Fatalf("enroll %s: status=%d", id, rr.Code)
		}
	}
	rr := doJSON(t, srv, http.MethodGet, "/api/fleet/sentinels", nil, "")
	var snap Snapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.Sentinels) != 2 {
		t.Fatalf("expected 2 distinct sentinels, got %d", len(snap.Sentinels))
	}
}

func TestServer_RepeatedEnroll_DoesNotDuplicate(t *testing.T) {
	srv, _ := newTestServer(t, "")
	for i := 0; i < 3; i++ {
		doJSON(t, srv, http.MethodPost, "/api/fleet/enroll", EnrollRequest{SentinelID: "sen-1", MachineID: "mach-1"}, "")
	}
	rr := doJSON(t, srv, http.MethodGet, "/api/fleet/sentinels", nil, "")
	var snap Snapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.Sentinels) != 1 {
		t.Fatalf("re-enrolling the same sentinel_id should not duplicate it, got %d entries", len(snap.Sentinels))
	}
}

func TestServer_EnrollMissingFields_Rejected(t *testing.T) {
	srv, _ := newTestServer(t, "")
	rr := doJSON(t, srv, http.MethodPost, "/api/fleet/enroll", EnrollRequest{SentinelID: "sen-1"}, "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing machine_id, got %d", rr.Code)
	}
}

func TestServer_TokenRequired_RejectsUnauthenticated(t *testing.T) {
	srv, _ := newTestServer(t, "secret-token")
	rr := doJSON(t, srv, http.MethodPost, "/api/fleet/enroll", EnrollRequest{SentinelID: "sen-1", MachineID: "mach-1"}, "")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", rr.Code)
	}
	rr = doJSON(t, srv, http.MethodPost, "/api/fleet/enroll", EnrollRequest{SentinelID: "sen-1", MachineID: "mach-1"}, "wrong")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong token, got %d", rr.Code)
	}
	rr = doJSON(t, srv, http.MethodPost, "/api/fleet/enroll", EnrollRequest{SentinelID: "sen-1", MachineID: "mach-1"}, "secret-token")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 with correct token, got %d", rr.Code)
	}
}

func TestServer_NoTokenConfigured_AllowsAnyRequest(t *testing.T) {
	srv, _ := newTestServer(t, "")
	rr := doJSON(t, srv, http.MethodPost, "/api/fleet/enroll", EnrollRequest{SentinelID: "sen-1", MachineID: "mach-1"}, "irrelevant")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 when no token is configured, got %d", rr.Code)
	}
}

func TestServer_DetailAPI_UnknownSentinel_404(t *testing.T) {
	srv, _ := newTestServer(t, "")
	rr := doJSON(t, srv, http.MethodGet, "/api/fleet/sentinels/does-not-exist", nil, "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestServer_IndexAndDetailPages_Render(t *testing.T) {
	srv, _ := newTestServer(t, "")
	doJSON(t, srv, http.MethodPost, "/api/fleet/enroll", EnrollRequest{SentinelID: "sen-1", MachineID: "mach-1", RepoPath: "/repo/a"}, "")

	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK || rr.Body.Len() == 0 {
		t.Fatalf("index page did not render: status=%d len=%d", rr.Code, rr.Body.Len())
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/fleet/sentinels/sen-1", nil))
	if rr.Code != http.StatusOK || rr.Body.Len() == 0 {
		t.Fatalf("detail page did not render: status=%d len=%d", rr.Code, rr.Body.Len())
	}
}

func TestServer_NoRepositoryContentsTransmitted(t *testing.T) {
	// The enroll/heartbeat request/response types are structurally incapable
	// of carrying file contents -- this test guards against a future field
	// being added that would violate that. It marshals both request types
	// and asserts none of the known non-metadata field names ever appear.
	enroll, _ := json.Marshal(EnrollRequest{SentinelID: "s", MachineID: "m", RepoPath: "/repo/a"})
	hb, _ := json.Marshal(HeartbeatRequest{SentinelID: "s"})
	for _, forbidden := range []string{"contents", "diff", "body", "source", "file_data"} {
		if bytesContainsCI(enroll, forbidden) || bytesContainsCI(hb, forbidden) {
			t.Fatalf("fleet protocol payload unexpectedly references %q", forbidden)
		}
	}
}

func bytesContainsCI(b []byte, sub string) bool {
	return bytes.Contains(bytes.ToLower(b), []byte(sub))
}
