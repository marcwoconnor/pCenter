// Package pvecluster orchestrates Proxmox VE cluster formation: taking a set
// of standalone hosts already registered in pcenter and turning them into a
// real Corosync-backed PVE cluster.
//
// The heavy lifting — `POST /cluster/config` on the founder, `POST
// /cluster/config/join` on each joiner, credential refresh after join,
// pcenter inventory update, poller reconfiguration — runs in a single
// long-lived goroutine per Job, tracked in memory by a Manager and surfaced
// to the frontend through an HTTP polling API.
//
// Design: fail-fast, no auto-rollback. On step failure, the Job error
// includes the specific manual-recovery command (pvecm delnode) so the
// operator can clean up and retry.
package pvecluster

import (
	"sync"
	"time"
)

// JobState is the top-level state of a cluster-formation Job.
type JobState string

const (
	JobStateRunning   JobState = "running"
	JobStateSucceeded JobState = "succeeded"
	JobStateFailed    JobState = "failed"
)

// StepState tracks an individual host step within a Job.
type StepState string

const (
	StepStatePending   StepState = "pending"
	StepStateRunning   StepState = "running"
	StepStateSucceeded StepState = "succeeded"
	StepStateFailed    StepState = "failed"
)

// StepPhase names the operation a Step represents.
type StepPhase string

const (
	PhaseCleanupCorosync StepPhase = "cleanup_corosync"
	PhaseCreateCluster   StepPhase = "create_cluster"
	PhaseFetchJoinInfo   StepPhase = "fetch_join_info"
	PhaseJoin            StepPhase = "join"
	PhaseReauthToken     StepPhase = "reauth_token"
	PhaseUpdateInventor  StepPhase = "update_inventory"
)

// Role is which side of the join this host is on.
type Role string

const (
	RoleFounder Role = "founder"
	RoleJoiner  Role = "joiner"
)

// Step is one line in the progress view: a (host, phase) tuple with timestamps
// and either a latest-message string or an error.
type Step struct {
	HostID    string    `json:"host_id"`
	Address   string    `json:"address"`
	NodeName  string    `json:"node_name,omitempty"`
	Role      Role      `json:"role"`
	Phase     StepPhase `json:"phase"`
	State     StepState `json:"state"`
	UPID      string    `json:"upid,omitempty"`
	Message   string    `json:"message,omitempty"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
}

// Job is the end-to-end state of one cluster-formation request. The mutex is
// held briefly for each field mutation; the manager returns deep copies to
// HTTP handlers so we never expose the live pointer.
type Job struct {
	mu sync.Mutex

	ID                 string
	ClusterName        string
	DatacenterID       string
	State              JobState
	InventoryClusterID string // populated on success
	Error              string
	Steps              []Step
	StartedAt          time.Time
	EndedAt            time.Time
}

// JobSnapshot is a point-in-time JSON-safe copy of a Job, returned to HTTP
// clients. No mutex, no methods — just data.
type JobSnapshot struct {
	JobID              string    `json:"job_id"`
	ClusterName        string    `json:"cluster_name"`
	DatacenterID       string    `json:"datacenter_id"`
	State              JobState  `json:"state"`
	InventoryClusterID string    `json:"inventory_cluster_id,omitempty"`
	Error              string    `json:"error,omitempty"`
	Steps              []Step    `json:"steps"`
	StartedAt          time.Time `json:"started_at"`
	EndedAt            time.Time `json:"ended_at,omitempty"`
}

// Snapshot returns a safe copy for handing to HTTP handlers.
func (j *Job) Snapshot() JobSnapshot {
	j.mu.Lock()
	defer j.mu.Unlock()
	steps := make([]Step, len(j.Steps))
	copy(steps, j.Steps)
	return JobSnapshot{
		JobID:              j.ID,
		ClusterName:        j.ClusterName,
		DatacenterID:       j.DatacenterID,
		State:              j.State,
		InventoryClusterID: j.InventoryClusterID,
		Error:              j.Error,
		Steps:              steps,
		StartedAt:          j.StartedAt,
		EndedAt:            j.EndedAt,
	}
}

// setStep locates (or appends) a step for (hostID, phase) and applies the
// mutation inside the job lock.
func (j *Job) setStep(hostID string, phase StepPhase, mutate func(*Step)) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for i := range j.Steps {
		s := &j.Steps[i]
		if s.HostID == hostID && s.Phase == phase {
			mutate(s)
			return
		}
	}
	// Shouldn't happen — the orchestrator seeds all expected steps up-front.
	// Append defensively so we don't lose the signal.
	step := Step{HostID: hostID, Phase: phase, State: StepStatePending}
	mutate(&step)
	j.Steps = append(j.Steps, step)
}

// startStep transitions a step to running with a start timestamp.
func (j *Job) startStep(hostID string, phase StepPhase) {
	j.setStep(hostID, phase, func(s *Step) {
		s.State = StepStateRunning
		s.StartedAt = time.Now()
		s.Error = ""
	})
}

// succeedStep marks a step complete.
func (j *Job) succeedStep(hostID string, phase StepPhase, msg string) {
	j.setStep(hostID, phase, func(s *Step) {
		s.State = StepStateSucceeded
		s.Message = msg
		s.EndedAt = time.Now()
	})
}

// failStep marks a step failed with an error message. Does NOT set the
// overall Job.State — the orchestrator decides whether to bail.
func (j *Job) failStep(hostID string, phase StepPhase, err string) {
	j.setStep(hostID, phase, func(s *Step) {
		s.State = StepStateFailed
		s.Error = err
		s.EndedAt = time.Now()
	})
}

// setStepMessage updates a step's latest status message without changing state.
func (j *Job) setStepMessage(hostID string, phase StepPhase, msg string) {
	j.setStep(hostID, phase, func(s *Step) {
		s.Message = msg
	})
}

// setStepUPID records the PVE task UPID for a step (for debugging / log links).
func (j *Job) setStepUPID(hostID string, phase StepPhase, upid string) {
	j.setStep(hostID, phase, func(s *Step) {
		s.UPID = upid
	})
}

// markFailed sets the job's terminal error state.
func (j *Job) markFailed(errMsg string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.State = JobStateFailed
	j.Error = errMsg
	j.EndedAt = time.Now()
}

// markSucceeded sets the job's terminal success state with the new inventory
// cluster ID so the frontend can refresh the tree.
func (j *Job) markSucceeded(inventoryClusterID string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.State = JobStateSucceeded
	j.InventoryClusterID = inventoryClusterID
	j.EndedAt = time.Now()
}
