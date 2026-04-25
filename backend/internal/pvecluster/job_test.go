package pvecluster

import (
	"testing"
)

func TestJobStateMachine(t *testing.T) {
	j := &Job{ID: "test", State: JobStateRunning}
	j.Steps = []Step{
		{HostID: "h1", Phase: PhaseCreateCluster, State: StepStatePending},
		{HostID: "h2", Phase: PhaseJoin, State: StepStatePending},
	}

	j.startStep("h1", PhaseCreateCluster)
	snap := j.Snapshot()
	if snap.Steps[0].State != StepStateRunning {
		t.Errorf("start: state = %s, want running", snap.Steps[0].State)
	}
	if snap.Steps[0].StartedAt.IsZero() {
		t.Error("start: StartedAt should be set")
	}

	j.setStepMessage("h1", PhaseCreateCluster, "working on it")
	snap = j.Snapshot()
	if snap.Steps[0].Message != "working on it" {
		t.Errorf("message = %q", snap.Steps[0].Message)
	}

	j.setStepUPID("h1", PhaseCreateCluster, "UPID:abc")
	snap = j.Snapshot()
	if snap.Steps[0].UPID != "UPID:abc" {
		t.Errorf("UPID = %q", snap.Steps[0].UPID)
	}

	j.succeedStep("h1", PhaseCreateCluster, "done")
	snap = j.Snapshot()
	if snap.Steps[0].State != StepStateSucceeded {
		t.Errorf("succeed: state = %s", snap.Steps[0].State)
	}

	j.failStep("h2", PhaseJoin, "boom")
	snap = j.Snapshot()
	if snap.Steps[1].State != StepStateFailed || snap.Steps[1].Error != "boom" {
		t.Errorf("fail: state=%s err=%q", snap.Steps[1].State, snap.Steps[1].Error)
	}

	j.markFailed("orchestrator bailed")
	snap = j.Snapshot()
	if snap.State != JobStateFailed {
		t.Errorf("job state = %s", snap.State)
	}
	if snap.Error != "orchestrator bailed" {
		t.Errorf("job error = %q", snap.Error)
	}

	// markSucceeded overrides failed (shouldn't happen in practice but the
	// method itself should behave). We start a second fresh job.
	k := &Job{State: JobStateRunning}
	k.markSucceeded("cl-123")
	if ks := k.Snapshot(); ks.State != JobStateSucceeded || ks.InventoryClusterID != "cl-123" {
		t.Errorf("markSucceeded: state=%s id=%q", ks.State, ks.InventoryClusterID)
	}
}

func TestJobSnapshotIsCopy(t *testing.T) {
	j := &Job{Steps: []Step{{HostID: "h1", Phase: PhaseJoin, Message: "original"}}}
	snap := j.Snapshot()
	snap.Steps[0].Message = "mutated"
	if j.Steps[0].Message != "original" {
		t.Errorf("Snapshot must return a copy; original was mutated to %q", j.Steps[0].Message)
	}
}

func TestShortUPIDAndTruncate(t *testing.T) {
	if got := shortUPID("short"); got != "short" {
		t.Errorf("shortUPID(short) = %q", got)
	}
	long := "UPID:pve01:01234:5678:ABCDEFGH:clustercreate::root@pam:"
	if got := shortUPID(long); len(got) >= len(long) {
		t.Errorf("shortUPID didn't shrink %q → %q", long, got)
	}
	if got := truncate("hello", 3); got != "hel…" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("hi", 5); got != "hi" {
		t.Errorf("truncate should not pad, got %q", got)
	}
}

func TestHostPart(t *testing.T) {
	cases := map[string]string{
		"10.0.0.1:8006": "10.0.0.1",
		"pve01:8006":    "pve01",
		"plain":         "plain",
		"":              "",
	}
	for in, want := range cases {
		if got := hostPart(in); got != want {
			t.Errorf("hostPart(%q) = %q, want %q", in, got, want)
		}
	}
}
