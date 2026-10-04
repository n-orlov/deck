package store

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLaunchLeaseCallsRejectMissingArguments(t *testing.T) {
	ctx := context.Background()
	s := openValidationStore(t)
	const id = "00000000-0000-4000-8000-000000000d01"
	_, err := s.AcquireLaunchLease(ctx, "", "1@b", time.Minute, leaseTestNow)
	if err == nil || err.Error() != "session id is required" {
		t.Fatalf("acquire without id error = %v", err)
	}
	_, err = s.AcquireLaunchLease(ctx, id, "", time.Minute, leaseTestNow)
	if err == nil || err.Error() != "lease owner is required" {
		t.Fatalf("acquire without owner error = %v", err)
	}
	_, err = s.AcquireLaunchLease(ctx, id, "1@b", time.Minute, leaseTestNow)
	if err == nil || err.Error() != fmt.Sprintf("session %q not found", id) {
		t.Fatalf("acquire on a missing session error = %v; want session not found", err)
	}
	if _, err = s.ReleaseLaunchLease(ctx, "", "1@b"); err == nil || err.Error() != "session id is required" {
		t.Fatalf("release without id error = %v", err)
	}
	if _, err = s.ReleaseLaunchLease(ctx, id, ""); err == nil || err.Error() != "lease owner is required" {
		t.Fatalf("release without owner error = %v", err)
	}
	released, err := s.ReleaseLaunchLease(ctx, id, "1@b")
	if err != nil || released {
		t.Fatalf("release on a missing session = %v, %v; want false, nil", released, err)
	}
}

func TestAcquireLaunchLeaseDefaultsNonPositiveTTL(t *testing.T) {
	ctx := context.Background()
	s := openValidationStore(t)
	const id = "00000000-0000-4000-8000-000000000d02"
	newLeaseTestSession(t, s, id, "stopped")
	res, err := s.AcquireLaunchLease(ctx, id, CurrentLaunchLeaseOwner(), 0, leaseTestNow)
	if err != nil || res.Outcome != LaunchLeaseAcquired {
		t.Fatalf("acquire with ttl 0 = %+v, %v; want acquired", res, err)
	}
	var until int64
	if err := s.DB().QueryRow(`SELECT launch_lease_until FROM sessions WHERE id = ?`, id).Scan(&until); err != nil {
		t.Fatal(err)
	}
	if want := leaseTestNow + DefaultLaunchLeaseTTL.Milliseconds(); until != want {
		t.Fatalf("launch_lease_until = %d; want %d (default TTL)", until, want)
	}
	if res.LaunchGeneration == "" || len(res.LaunchGeneration) != 16 {
		t.Fatalf("launch generation = %q; want 16 hex chars", res.LaunchGeneration)
	}
}

func TestParseLeaseOwner(t *testing.T) {
	cases := []struct {
		owner string
		pid   int
		boot  string
		ok    bool
	}{
		{"123@boot-a", 123, "boot-a", true},
		{"123@boot-a#deadbeef", 123, "boot-a", true},
		{"123@", 123, "", true},
		{"no-at-sign", 0, "", false},
		{"abc@boot", 0, "", false},
		{"0@boot", 0, "", false},
		{"-5@boot", 0, "", false},
		{"", 0, "", false},
	}
	for _, tc := range cases {
		pid, boot, ok := parseLeaseOwner(tc.owner)
		if pid != tc.pid || boot != tc.boot || ok != tc.ok {
			t.Errorf("parseLeaseOwner(%q) = (%d, %q, %v); want (%d, %q, %v)", tc.owner, pid, boot, ok, tc.pid, tc.boot, tc.ok)
		}
	}
}

func TestLeaseOwnerAliveJudgement(t *testing.T) {
	self := CurrentLaunchLeaseOwner()
	if !leaseOwnerAlive(self) {
		t.Errorf("leaseOwnerAlive(%q) = false; the running process must be alive", self)
	}
	if !leaseOwnerAlive(self + "#0123456789abcdef") {
		t.Errorf("a generation suffix must not change the liveness judgement")
	}
	if leaseOwnerAlive("garbage") {
		t.Error("an unparseable owner must be judged stale")
	}
	if boot := bootID(); boot != "" {
		if leaseOwnerAlive(fmt.Sprintf("%d@%s-previous-boot", os.Getpid(), boot)) {
			t.Error("a live pid from a different boot must be judged stale")
		}
	}
	if leaseOwnerAlive(fmt.Sprintf("2147483646@%s", bootID())) {
		t.Error("a pid that answers no signal must be judged stale")
	}
}

func TestCurrentLaunchLeaseOwnerFormat(t *testing.T) {
	owner := CurrentLaunchLeaseOwner()
	prefix := fmt.Sprintf("%d@", os.Getpid())
	if !strings.HasPrefix(owner, prefix) {
		t.Fatalf("owner = %q; want prefix %q", owner, prefix)
	}
	data, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err == nil && strings.TrimPrefix(owner, prefix) != strings.TrimSpace(string(data)) {
		t.Fatalf("owner boot component = %q; want %q", strings.TrimPrefix(owner, prefix), strings.TrimSpace(string(data)))
	}
}
