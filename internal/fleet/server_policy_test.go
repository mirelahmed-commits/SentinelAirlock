package fleet

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestServer_CreatePolicy_ThenListedAndShowable(t *testing.T) {
	srv, _, _ := newTestServer(t, "")

	rr := doJSON(t, srv, http.MethodPost, "/api/fleet/policies", map[string]string{
		"policy_id": "production", "description": "prod", "yaml": sampleYAML,
	}, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created policyVersionSummary
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Version != 1 || created.Hash == "" {
		t.Fatalf("unexpected create response: %+v", created)
	}

	rr = doJSON(t, srv, http.MethodGet, "/api/fleet/policies", nil, "")
	var list []policyVersionSummary
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].PolicyID != "production" {
		t.Fatalf("expected the new policy in the list, got %+v", list)
	}

	rr = doJSON(t, srv, http.MethodGet, "/api/fleet/policies/production", nil, "")
	var versions []policyVersionSummary
	if err := json.Unmarshal(rr.Body.Bytes(), &versions); err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}
}

func TestServer_CreatePolicy_InvalidYAML_Rejected(t *testing.T) {
	srv, _, _ := newTestServer(t, "")
	rr := doJSON(t, srv, http.MethodPost, "/api/fleet/policies", map[string]string{
		"policy_id": "production", "yaml": "not: valid: yaml: [[",
	}, "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid policy YAML, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestServer_AddPolicyVersion_FetchByVersionReturnsFullContent(t *testing.T) {
	srv, _, policyStore := newTestServer(t, "")
	if _, err := policyStore.Create("production", "v1", sampleYAML); err != nil {
		t.Fatal(err)
	}

	rr := doJSON(t, srv, http.MethodPost, "/api/fleet/policies/production/versions", map[string]string{
		"description": "v2", "yaml": sampleYAML + "\n",
	}, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("add version status=%d body=%s", rr.Code, rr.Body.String())
	}
	var v2 policyVersionSummary
	if err := json.Unmarshal(rr.Body.Bytes(), &v2); err != nil {
		t.Fatal(err)
	}
	if v2.Version != 2 {
		t.Fatalf("expected version 2, got %d", v2.Version)
	}

	rr = doJSON(t, srv, http.MethodGet, "/api/fleet/policies/production/versions/1", nil, "")
	var full PolicyVersion
	if err := json.Unmarshal(rr.Body.Bytes(), &full); err != nil {
		t.Fatal(err)
	}
	if full.YAML != sampleYAML {
		t.Fatalf("expected version 1's original YAML content, got %q", full.YAML)
	}
}

func TestServer_AssignPolicy_ThenHeartbeatReportsDesired(t *testing.T) {
	srv, _, policyStore := newTestServer(t, "")
	v, err := policyStore.Create("production", "", sampleYAML)
	if err != nil {
		t.Fatal(err)
	}
	doJSON(t, srv, http.MethodPost, "/api/fleet/enroll", EnrollRequest{SentinelID: "sen-1", MachineID: "mach-1"}, "")

	rr := doJSON(t, srv, http.MethodPost, "/api/fleet/sentinels/sen-1/assign", map[string]any{
		"policy_id": "production", "version": v.Version,
	}, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("assign status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = doJSON(t, srv, http.MethodPost, "/api/fleet/heartbeat", HeartbeatRequest{
		SentinelID: "sen-1", Timestamp: time.Now().UTC(),
	}, "")
	var hbResp HeartbeatResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &hbResp); err != nil {
		t.Fatal(err)
	}
	if hbResp.DesiredPolicyID != "production" || hbResp.DesiredPolicyVersion != v.Version || hbResp.DesiredPolicyHash != v.Hash {
		t.Fatalf("expected heartbeat response to carry the assigned desired policy, got %+v", hbResp)
	}
}

func TestServer_AssignPolicy_UnknownVersion_Rejected(t *testing.T) {
	srv, _, policyStore := newTestServer(t, "")
	if _, err := policyStore.Create("production", "", sampleYAML); err != nil {
		t.Fatal(err)
	}
	rr := doJSON(t, srv, http.MethodPost, "/api/fleet/sentinels/sen-1/assign", map[string]any{
		"policy_id": "production", "version": 99,
	}, "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a nonexistent policy version, got %d", rr.Code)
	}
}

func TestServer_MultipleSentinels_DifferentDesiredPolicies(t *testing.T) {
	srv, _, policyStore := newTestServer(t, "")
	prod, _ := policyStore.Create("production", "", sampleYAML)
	ci, _ := policyStore.Create("ci-restricted", "", sampleYAML+"\n")

	doJSON(t, srv, http.MethodPost, "/api/fleet/enroll", EnrollRequest{SentinelID: "sen-a", MachineID: "m"}, "")
	doJSON(t, srv, http.MethodPost, "/api/fleet/enroll", EnrollRequest{SentinelID: "sen-b", MachineID: "m"}, "")

	doJSON(t, srv, http.MethodPost, "/api/fleet/sentinels/sen-a/assign", map[string]any{"policy_id": "production", "version": prod.Version}, "")
	doJSON(t, srv, http.MethodPost, "/api/fleet/sentinels/sen-b/assign", map[string]any{"policy_id": "ci-restricted", "version": ci.Version}, "")

	rr := doJSON(t, srv, http.MethodGet, "/api/fleet/sentinels", nil, "")
	var snap Snapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, sv := range snap.Sentinels {
		got[sv.SentinelID] = sv.DesiredPolicyID
	}
	if got["sen-a"] != "production" || got["sen-b"] != "ci-restricted" {
		t.Fatalf("expected distinct desired policies per sentinel, got %+v", got)
	}
}

func TestServer_DriftAndReconcile_FullCycle(t *testing.T) {
	srv, store, policyStore := newTestServer(t, "")
	v1, err := policyStore.Create("production", "", sampleYAML)
	if err != nil {
		t.Fatal(err)
	}
	doJSON(t, srv, http.MethodPost, "/api/fleet/enroll", EnrollRequest{
		SentinelID: "sen-1", MachineID: "m", PolicyID: "production", PolicyVersion: "1", PolicyHash: v1.Hash,
	}, "")
	doJSON(t, srv, http.MethodPost, "/api/fleet/sentinels/sen-1/assign", map[string]any{"policy_id": "production", "version": 1}, "")

	rr := doJSON(t, srv, http.MethodGet, "/api/fleet/sentinels/sen-1", nil, "")
	var view SentinelView
	json.Unmarshal(rr.Body.Bytes(), &view)
	if view.PolicyState != "IN_SYNC" {
		t.Fatalf("expected IN_SYNC once actual already matches the assignment, got %s", view.PolicyState)
	}

	// A new version is created and assigned: actual (still v1) now drifts
	// from desired (v2) until a heartbeat reports the new actual state.
	v2, err := policyStore.AddVersion("production", "", sampleYAML+"\n")
	if err != nil {
		t.Fatal(err)
	}
	doJSON(t, srv, http.MethodPost, "/api/fleet/sentinels/sen-1/assign", map[string]any{"policy_id": "production", "version": v2.Version}, "")

	rr = doJSON(t, srv, http.MethodGet, "/api/fleet/sentinels/sen-1", nil, "")
	json.Unmarshal(rr.Body.Bytes(), &view)
	if view.PolicyState != "DRIFTED" {
		t.Fatalf("expected DRIFTED immediately after reassignment, before reconciliation, got %s", view.PolicyState)
	}

	// Reconciliation: Sentinel reports the new actual state via heartbeat.
	doJSON(t, srv, http.MethodPost, "/api/fleet/heartbeat", HeartbeatRequest{
		SentinelID: "sen-1", Timestamp: time.Now().UTC(),
		PolicyID: "production", PolicyVersion: "2", PolicyHash: v2.Hash,
	}, "")

	rr = doJSON(t, srv, http.MethodGet, "/api/fleet/sentinels/sen-1", nil, "")
	json.Unmarshal(rr.Body.Bytes(), &view)
	if view.PolicyState != "IN_SYNC" {
		t.Fatalf("expected drift to clear back to IN_SYNC after reconciliation, got %s", view.PolicyState)
	}

	rec, _ := store.Get("sen-1")
	if rec.PolicyVersion != "2" {
		t.Fatalf("expected the stored actual version to be updated to 2, got %s", rec.PolicyVersion)
	}
}

func TestServer_ReconcileFailed_ReportedAndSurvivesInInventory(t *testing.T) {
	srv, _, policyStore := newTestServer(t, "")
	v1, _ := policyStore.Create("production", "", sampleYAML)
	doJSON(t, srv, http.MethodPost, "/api/fleet/enroll", EnrollRequest{SentinelID: "sen-1", MachineID: "m"}, "")
	doJSON(t, srv, http.MethodPost, "/api/fleet/sentinels/sen-1/assign", map[string]any{"policy_id": "production", "version": v1.Version}, "")

	doJSON(t, srv, http.MethodPost, "/api/fleet/heartbeat", HeartbeatRequest{
		SentinelID: "sen-1", Timestamp: time.Now().UTC(),
		ReconcileStatus: "RECONCILE_FAILED", ReconcileError: "invalid policy document: yaml: line 3", ReconcileForHash: v1.Hash,
	}, "")

	rr := doJSON(t, srv, http.MethodGet, "/api/fleet/sentinels/sen-1", nil, "")
	var view SentinelView
	json.Unmarshal(rr.Body.Bytes(), &view)
	if view.PolicyState != "RECONCILE_FAILED" || view.PolicyStateError == "" {
		t.Fatalf("expected RECONCILE_FAILED with an error message, got state=%s err=%q", view.PolicyState, view.PolicyStateError)
	}
}

func TestServer_UIRendersDesiredActualSyncState(t *testing.T) {
	srv, _, policyStore := newTestServer(t, "")
	v1, err := policyStore.Create("production", "", sampleYAML)
	if err != nil {
		t.Fatal(err)
	}
	doJSON(t, srv, http.MethodPost, "/api/fleet/enroll", EnrollRequest{
		SentinelID: "sen-1", MachineID: "m", PolicyID: "production", PolicyVersion: "1", PolicyHash: v1.Hash,
	}, "")
	doJSON(t, srv, http.MethodPost, "/api/fleet/sentinels/sen-1/assign", map[string]any{"policy_id": "production", "version": v1.Version}, "")

	rr := doJSON(t, srv, http.MethodGet, "/", nil, "")
	body := rr.Body.String()
	if !bytesContainsCI([]byte(body), "desired") || !bytesContainsCI([]byte(body), "actual") {
		t.Fatal("expected the index page to reference desired/actual policy columns")
	}

	rr = doJSON(t, srv, http.MethodGet, "/fleet/sentinels/sen-1", nil, "")
	detail := rr.Body.String()
	if !bytesContainsCI([]byte(detail), "in_sync") {
		t.Fatalf("expected the detail page to show IN_SYNC for a fully reconciled sentinel, got:\n%s", detail)
	}
	if !bytesContainsCI([]byte(detail), "production") {
		t.Fatal("expected the detail page to show the policy id")
	}
}

func TestServer_PolicyAPIsDoNotAcceptFilesystemPaths(t *testing.T) {
	// The create/add-version request types must not have a field that could
	// be mistaken for "read this path on the server" -- content always
	// travels as a YAML string in the request body.
	b, _ := json.Marshal(createPolicyRequest{PolicyID: "x", YAML: sampleYAML})
	if bytesContainsCI(b, "\"path\"") || bytesContainsCI(b, "\"file\"") {
		t.Fatalf("policy create/update request unexpectedly references a path/file field: %s", b)
	}
}
