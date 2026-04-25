package cephcluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/moconnor/pcenter/internal/poller"
)

// Deps are collaborators the orchestration flow needs. Passed in once at
// construction so StartInstall doesn't take a dozen args.
type Deps struct {
	Poller *poller.Poller // required for picking PVE clients during orchestration
}

// Manager owns the registry of in-flight + recently-completed Jobs and is
// the entrypoint for starting new install/destroy runs.
//
// State lives in memory — explicit non-goal for v1 is crash survival. On
// pcenter restart all jobs are lost; the frontend polls and on 404 falls
// back to the topology view (the install either landed or the operator
// can re-run it).
type Manager struct {
	mu   sync.Mutex
	jobs map[string]*Job
	deps Deps
}

// NewManager constructs a Manager.
func NewManager(deps Deps) *Manager {
	return &Manager{
		jobs: make(map[string]*Job),
		deps: deps,
	}
}

// InstallRequest is the input to StartInstall.
//
// Network is the public-network CIDR (e.g. "10.0.0.0/24") passed to
// `pveceph init --network`. ClusterNetwork is optional — when set, OSD
// replication uses that network instead.
//
// Nodes is the ordered list of PVE nodes to install Ceph on. The first
// entry is the founder — it gets pveceph init + the first MON + the
// first MGR. The rest are joiners that get pveceph install + an
// additional MON each.
//
// v1 relies on pcenter's existing SSH key trust to root@<node> for
// `pveceph install` (same path SetCephNoout uses today) and on the
// configured PVE API token for /ceph/init + MON/MGR creation. No
// per-job passwords — keep the auth model consistent with the rest of
// pCenter. If a future PVE release rejects token auth on /ceph/init we
// can add a password-collection step then.
//
// PoolSize / MinSize seed the global Ceph defaults (size=N, min_size=M);
// can be overridden per-pool day-2.
type InstallRequest struct {
	Cluster        string
	Nodes          []string
	Network        string
	ClusterNetwork string
	PoolSize       int // default 3 if zero
	MinSize        int // default 2 if zero
}

// StartInstall validates the request, seeds a Job with all expected steps,
// and kicks off the orchestration goroutine. Returns the Job ID immediately;
// callers poll GetJob for progress.
func (m *Manager) StartInstall(ctx context.Context, req InstallRequest) (string, error) {
	if req.Cluster == "" {
		return "", fmt.Errorf("cluster is required")
	}
	if len(req.Nodes) == 0 {
		return "", fmt.Errorf("at least one node is required")
	}
	if req.Network == "" {
		return "", fmt.Errorf("network CIDR is required")
	}
	for i, n := range req.Nodes {
		if n == "" {
			return "", fmt.Errorf("node[%d] is empty", i)
		}
	}
	if req.PoolSize == 0 {
		req.PoolSize = 3
	}
	if req.MinSize == 0 {
		req.MinSize = 2
	}

	job := &Job{
		ID:        uuid.NewString(),
		Kind:      JobKindInstall,
		Cluster:   req.Cluster,
		State:     JobStateRunning,
		StartedAt: time.Now(),
	}

	// Seed all expected steps up-front so the UI can render the full plan
	// from the first poll, not just whatever has started.
	for _, n := range req.Nodes {
		job.Steps = append(job.Steps, Step{Host: n, Phase: PhaseInstallPreflight, State: StepStatePending})
	}
	for _, n := range req.Nodes {
		job.Steps = append(job.Steps, Step{Host: n, Phase: PhaseInstallPackages, State: StepStatePending})
	}
	founder := req.Nodes[0]
	job.Steps = append(job.Steps, Step{Host: founder, Phase: PhaseCephInit, State: StepStatePending})
	for _, n := range req.Nodes {
		job.Steps = append(job.Steps, Step{Host: n, Phase: PhaseCreateMON, State: StepStatePending})
	}
	job.Steps = append(job.Steps, Step{Host: founder, Phase: PhaseCreateMGR, State: StepStatePending})
	job.Steps = append(job.Steps, Step{Host: "", Phase: PhaseWaitHealthy, State: StepStatePending})

	m.mu.Lock()
	m.jobs[job.ID] = job
	m.mu.Unlock()

	// Detach: ctx from the HTTP request would cancel as soon as the handler
	// returns, but the install must keep running. Use a fresh background
	// context with the Manager's lifetime.
	go m.runInstall(context.Background(), job, req)

	return job.ID, nil
}

// GetJob returns a snapshot of a job by ID, or (nil, false) if unknown.
func (m *Manager) GetJob(id string) (JobSnapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return JobSnapshot{}, false
	}
	snap := j.Snapshot()
	return snap, true
}

// ListJobs returns snapshots of all known jobs (running + recently completed),
// sorted by StartedAt descending. v1 keeps everything in memory; eviction is
// future work — restart wipes the registry.
func (m *Manager) ListJobs() []JobSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]JobSnapshot, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, j.Snapshot())
	}
	// Sort manually to avoid pulling in sort.Slice for one call (cheap; ~tens of jobs).
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1].StartedAt.Before(out[j].StartedAt); j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// fail is a small helper that markFailed's the job AND sets a final-step
// error so the UI shows a clear "this is where it broke" line.
func (m *Manager) fail(job *Job, host string, phase StepPhase, errMsg string) {
	if host != "" {
		job.failStep(host, phase, errMsg)
	}
	job.markFailed(errMsg)
}
