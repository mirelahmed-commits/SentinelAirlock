package cli

import (
	"net"
	"strconv"
	"testing"
)

func TestIsPortInUse_UnixMessage(t *testing.T) {
	if !isPortInUse(fakeErr("bind: address already in use")) {
		t.Error("expected true for unix address-in-use message")
	}
}

func TestIsPortInUse_WindowsMessage(t *testing.T) {
	if !isPortInUse(fakeErr("Only one usage of each socket address")) {
		t.Error("expected true for windows socket-in-use message")
	}
}

func TestIsPortInUse_UnrelatedError(t *testing.T) {
	if isPortInUse(fakeErr("permission denied")) {
		t.Error("expected false for unrelated error")
	}
	if isPortInUse(fakeErr("connection refused")) {
		t.Error("expected false for connection refused")
	}
}

func TestFindAlternativePorts_OccupiedNotSuggested(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	occupied := ln.Addr().(*net.TCPAddr).Port

	alts := findAlternativePorts("127.0.0.1", occupied-1, 3)

	for _, p := range alts {
		if p == occupied {
			t.Errorf("occupied port %d appeared in suggestions", occupied)
		}
	}
}

func TestFindAlternativePorts_MaxThree(t *testing.T) {
	alts := findAlternativePorts("127.0.0.1", 40000, 3)
	if len(alts) > 3 {
		t.Errorf("got %d alternatives, want ≤3", len(alts))
	}
}

func TestFindAlternativePorts_AllActuallyBindable(t *testing.T) {
	alts := findAlternativePorts("127.0.0.1", 40200, 3)
	for _, p := range alts {
		probe, probeErr := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p))
		if probeErr != nil {
			t.Errorf("alternative port %d is not actually bindable: %v", p, probeErr)
		} else {
			_ = probe.Close()
		}
	}
}

func TestFindAlternativePorts_NoDuplicates(t *testing.T) {
	alts := findAlternativePorts("127.0.0.1", 40400, 3)
	seen := map[int]bool{}
	for _, p := range alts {
		if seen[p] {
			t.Errorf("duplicate port %d in suggestions", p)
		}
		seen[p] = true
	}
}

// fakeErr is a minimal error implementation for testing string-match logic.
type fakeErr string

func (e fakeErr) Error() string { return string(e) }
