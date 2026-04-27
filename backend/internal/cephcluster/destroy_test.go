package cephcluster

import (
	"strings"
	"testing"
)

// TestDestroyClusterPhases_OSDBeforePurge guards against the regression where
// the destroy orchestration ran `pveceph purge` while OSDs were still
// configured. PVE's purge aborts in that state, leaving the cluster in a
// half-torn-down condition (config + CRUSH map intact, MGRs gone).
func TestDestroyClusterPhases_OSDBeforePurge(t *testing.T) {
	phases := destroyClusterPhases()

	indexOf := func(target StepPhase) int {
		for i, p := range phases {
			if p == target {
				return i
			}
		}
		return -1
	}

	// Sanity: the OSD phase exists.
	osdIdx := indexOf(PhaseDeleteOSD)
	if osdIdx < 0 {
		t.Fatal("PhaseDeleteOSD missing from destroyClusterPhases — pveceph purge will abort with leftover OSDs")
	}

	// OSDs must be destroyed before MONs — `ceph osd destroy` writes the
	// OSDmap and needs MONs alive to commit.
	monIdx := indexOf(PhaseDeleteMON)
	if monIdx < 0 || osdIdx >= monIdx {
		t.Errorf("PhaseDeleteOSD (idx=%d) must come before PhaseDeleteMON (idx=%d)", osdIdx, monIdx)
	}

	// OSDs should also come before MGR — without an MGR, OSD destroy can
	// stall on safe-to-destroy responses.
	mgrIdx := indexOf(PhaseDeleteMGR)
	if mgrIdx < 0 || osdIdx >= mgrIdx {
		t.Errorf("PhaseDeleteOSD (idx=%d) must come before PhaseDeleteMGR (idx=%d)", osdIdx, mgrIdx)
	}
}

// TestFinalizeDestroyResult_AllPurgesFailed asserts that when every per-node
// pveceph purge step failed, the job is marked failed (not silently
// succeeded as it used to be — that masked half-destroyed clusters from
// the UI).
func TestFinalizeDestroyResult_AllPurgesFailed(t *testing.T) {
	m := &Manager{}
	job := &Job{
		Kind: JobKindDestroy,
		Steps: []Step{
			{Phase: PhaseDestroyPreflight, State: StepStateSucceeded},
			{Host: "n1", Phase: PhaseCephPurge, State: StepStateFailed, Error: "boom"},
			{Host: "n2", Phase: PhaseCephPurge, State: StepStateFailed, Error: "boom"},
		},
	}

	m.finalizeDestroyResult(job)

	if job.State != JobStateFailed {
		t.Fatalf("State = %q, want %q", job.State, JobStateFailed)
	}
	if !strings.Contains(job.Error, "pveceph purge failed on all 2 node(s)") {
		t.Errorf("job.Error missing per-node count: %q", job.Error)
	}
}

// TestFinalizeDestroyResult_OnePurgeSucceeded asserts the soft rule: any
// successful purge keeps the job in the succeeded state. The remaining
// node-failures are visible per-step for the operator to mop up by hand.
func TestFinalizeDestroyResult_OnePurgeSucceeded(t *testing.T) {
	m := &Manager{}
	job := &Job{
		Kind: JobKindDestroy,
		Steps: []Step{
			{Host: "n1", Phase: PhaseCephPurge, State: StepStateSucceeded},
			{Host: "n2", Phase: PhaseCephPurge, State: StepStateFailed, Error: "boom"},
		},
	}

	m.finalizeDestroyResult(job)

	if job.State != JobStateSucceeded {
		t.Fatalf("State = %q, want %q", job.State, JobStateSucceeded)
	}
	if job.Error != "" {
		t.Errorf("job.Error = %q, want empty", job.Error)
	}
}

// TestFinalizeDestroyResult_NoPurgeSteps covers the early-exit branch (Ceph
// not installed → no purge attempted). Job should succeed.
func TestFinalizeDestroyResult_NoPurgeSteps(t *testing.T) {
	m := &Manager{}
	job := &Job{
		Kind: JobKindDestroy,
		Steps: []Step{
			{Phase: PhaseDestroyPreflight, State: StepStateSucceeded},
		},
	}

	m.finalizeDestroyResult(job)

	if job.State != JobStateSucceeded {
		t.Fatalf("State = %q, want %q", job.State, JobStateSucceeded)
	}
}
