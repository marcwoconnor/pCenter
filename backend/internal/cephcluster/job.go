// Package cephcluster orchestrates Ceph install + destroy operations across
// the nodes of a PVE cluster. Mirrors the pvecluster package — same Manager
// + Job + phased Step shape so frontends can reuse one progress widget.
//
// v1 install path: SSH `pveceph install` and `pveceph init` (consistent with
// the existing Ceph SSH helpers in pve.Client), then REST POST /ceph/mon and
// /ceph/mgr to wire monitors and managers. Streaming progress + the optional
// pve-agent route are tracked as future work in docs/ceph-lifecycle-plan.md.
//
// Design: fail-fast, no auto-rollback. On step failure the Job error
// includes a manual-recovery hint (e.g. `pveceph purge` to reset).
package cephcluster

import (
	"sync"
	"time"
)

// JobState is the top-level state of a Ceph install/destroy Job.
type JobState string

const (
	JobStateRunning   JobState = "running"
	JobStateSucceeded JobState = "succeeded"
	JobStateFailed    JobState = "failed"
)

// JobKind distinguishes install vs destroy in the registry. Both use the
// same Job + Step shape so the UI can render either with one component.
type JobKind string

const (
	JobKindInstall JobKind = "install"
	JobKindDestroy JobKind = "destroy"
)

// StepState tracks an individual step within a Job.
type StepState string

const (
	StepStatePending   StepState = "pending"
	StepStateRunning   StepState = "running"
	StepStateSucceeded StepState = "succeeded"
	StepStateFailed    StepState = "failed"
)

// StepPhase names the operation a Step represents. Names are stable strings
// the UI can pattern-match on for icons / friendly labels.
type StepPhase string

const (
	// Install phases.
	PhaseInstallPreflight StepPhase = "install_preflight"
	PhaseInstallPackages  StepPhase = "install_packages"
	PhaseCephInit         StepPhase = "ceph_init"
	PhaseCreateMON        StepPhase = "create_mon"
	PhaseCreateMGR        StepPhase = "create_mgr"
	PhaseWaitHealthy      StepPhase = "wait_healthy"

	// Destroy phases (used by PR 4).
	PhaseDestroyPreflight StepPhase = "destroy_preflight"
	PhaseSetNoout         StepPhase = "set_noout"
	PhaseDeletePools      StepPhase = "delete_pools"
	PhaseDeleteFS         StepPhase = "delete_fs"
	PhaseDeleteMDS        StepPhase = "delete_mds"
	PhaseDeleteMGR        StepPhase = "delete_mgr"
	PhaseDeleteMON        StepPhase = "delete_mon"
	PhaseCephPurge        StepPhase = "ceph_purge"
)

// Step is one line in the progress view: a (host, phase) tuple with state +
// optional message / error / UPID.
type Step struct {
	Host      string    `json:"host"`            // PVE node name; empty for cluster-scoped steps
	Phase     StepPhase `json:"phase"`
	State     StepState `json:"state"`
	UPID      string    `json:"upid,omitempty"`
	Message   string    `json:"message,omitempty"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
}

// Job is the end-to-end state of one install or destroy request.
type Job struct {
	mu sync.Mutex

	ID        string
	Kind      JobKind
	Cluster   string
	State     JobState
	Error     string
	Steps     []Step
	StartedAt time.Time
	EndedAt   time.Time
}

// JobSnapshot is a JSON-safe point-in-time copy of a Job for HTTP handlers.
type JobSnapshot struct {
	JobID     string    `json:"job_id"`
	Kind      JobKind   `json:"kind"`
	Cluster   string    `json:"cluster"`
	State     JobState  `json:"state"`
	Error     string    `json:"error,omitempty"`
	Steps     []Step    `json:"steps"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
}

// Snapshot returns a deep-copied JSON-safe view of the Job.
func (j *Job) Snapshot() JobSnapshot {
	j.mu.Lock()
	defer j.mu.Unlock()
	steps := make([]Step, len(j.Steps))
	copy(steps, j.Steps)
	return JobSnapshot{
		JobID:     j.ID,
		Kind:      j.Kind,
		Cluster:   j.Cluster,
		State:     j.State,
		Error:     j.Error,
		Steps:     steps,
		StartedAt: j.StartedAt,
		EndedAt:   j.EndedAt,
	}
}

// setStep locates (or appends) a step for (host, phase) and applies the
// mutation under the job lock.
func (j *Job) setStep(host string, phase StepPhase, mutate func(*Step)) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for i := range j.Steps {
		s := &j.Steps[i]
		if s.Host == host && s.Phase == phase {
			mutate(s)
			return
		}
	}
	step := Step{Host: host, Phase: phase, State: StepStatePending}
	mutate(&step)
	j.Steps = append(j.Steps, step)
}

func (j *Job) startStep(host string, phase StepPhase) {
	j.setStep(host, phase, func(s *Step) {
		s.State = StepStateRunning
		s.StartedAt = time.Now()
		s.Error = ""
	})
}

func (j *Job) succeedStep(host string, phase StepPhase, msg string) {
	j.setStep(host, phase, func(s *Step) {
		s.State = StepStateSucceeded
		s.Message = msg
		s.EndedAt = time.Now()
	})
}

func (j *Job) failStep(host string, phase StepPhase, errMsg string) {
	j.setStep(host, phase, func(s *Step) {
		s.State = StepStateFailed
		s.Error = errMsg
		s.EndedAt = time.Now()
	})
}

func (j *Job) setStepMessage(host string, phase StepPhase, msg string) {
	j.setStep(host, phase, func(s *Step) {
		s.Message = msg
	})
}

func (j *Job) setStepUPID(host string, phase StepPhase, upid string) {
	j.setStep(host, phase, func(s *Step) {
		s.UPID = upid
	})
}

func (j *Job) markFailed(errMsg string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.State = JobStateFailed
	j.Error = errMsg
	j.EndedAt = time.Now()
}

func (j *Job) markSucceeded() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.State = JobStateSucceeded
	j.EndedAt = time.Now()
}
