package pvecluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/moconnor/pcenter/internal/activity"
	"github.com/moconnor/pcenter/internal/inventory"
	"github.com/moconnor/pcenter/internal/poller"
)

// Deps are the collaborators the orchestration flow needs. Passed into the
// Manager once at construction so StartJob doesn't take a dozen args.
type Deps struct {
	Inventory *inventory.Service
	Poller    *poller.Poller
	Activity  *activity.Service // may be nil
}

// Manager owns the registry of in-flight and recently-completed Jobs and
// supplies the orchestration entrypoint.
//
// State lives in memory — an explicit non-goal for v1 is crash survival. On
// pcenter restart all jobs are lost; the frontend is expected to poll and
// receive a 404 → "restarted, verify manually" message.
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

// StartJobRequest is the full input needed to kick off a cluster-formation run.
type StartJobRequest struct {
	ClusterName      string
	DatacenterID     string
	FounderHostID    string
	FounderPassword  string
	FounderLink0     string // optional
	Joiners          []JoinerSpec
	MajorVersionHint string // optional: cached from preflight for the activity log
}

// JoinerSpec is one joining node's params.
type JoinerSpec struct {
	HostID   string
	Password string
	Link0    string // optional
}

// StartJob validates, seeds a Job, and kicks off the orchestration goroutine.
// Returns the Job ID immediately; callers poll GetJob for progress.
func (m *Manager) StartJob(ctx context.Context, req StartJobRequest) (string, error) {
	if req.ClusterName == "" {
		return "", fmt.Errorf("cluster_name is required")
	}
	if req.FounderHostID == "" {
		return "", fmt.Errorf("founder_host_id is required")
	}
	// Joiners are optional: Proxmox allows forming a 1-node cluster and
	// adding members later.
	if req.FounderPassword == "" {
		return "", fmt.Errorf("founder_password is required")
	}
	for i, j := range req.Joiners {
		if j.HostID == "" || j.Password == "" {
			return "", fmt.Errorf("joiner[%d] missing host_id or password", i)
		}
	}

	// Reject dup cluster name up-front; we'll re-check atomically later.
	existing, err := m.deps.Inventory.GetClusterByName(ctx, req.ClusterName)
	if err != nil {
		return "", fmt.Errorf("check cluster name: %w", err)
	}
	if existing != nil {
		return "", fmt.Errorf("cluster %q already exists in pcenter", req.ClusterName)
	}

	founder, err := m.deps.Inventory.GetHost(ctx, req.FounderHostID)
	if err != nil || founder == nil {
		return "", fmt.Errorf("founder host not found")
	}
	hosts := []*inventory.InventoryHost{founder}
	for _, j := range req.Joiners {
		h, err := m.deps.Inventory.GetHost(ctx, j.HostID)
		if err != nil || h == nil {
			return "", fmt.Errorf("joiner host %s not found", j.HostID)
		}
		hosts = append(hosts, h)
	}

	// Seed the job with one row per expected step so the frontend's progress
	// view renders the full plan from the first poll.
	job := &Job{
		ID:           uuid.New().String(),
		ClusterName:  req.ClusterName,
		DatacenterID: req.DatacenterID,
		State:        JobStateRunning,
		StartedAt:    time.Now(),
	}
	job.Steps = append(job.Steps, Step{
		HostID: founder.ID, Address: founder.Address, NodeName: founder.NodeName,
		Role: RoleFounder, Phase: PhaseCreateCluster, State: StepStatePending,
	})
	job.Steps = append(job.Steps, Step{
		HostID: founder.ID, Address: founder.Address, NodeName: founder.NodeName,
		Role: RoleFounder, Phase: PhaseFetchJoinInfo, State: StepStatePending,
	})
	for _, j := range req.Joiners {
		h := mustFind(hosts, j.HostID)
		job.Steps = append(job.Steps, Step{
			HostID: h.ID, Address: h.Address, NodeName: h.NodeName,
			Role: RoleJoiner, Phase: PhaseJoin, State: StepStatePending,
		})
		job.Steps = append(job.Steps, Step{
			HostID: h.ID, Address: h.Address, NodeName: h.NodeName,
			Role: RoleJoiner, Phase: PhaseReauthToken, State: StepStatePending,
		})
	}
	job.Steps = append(job.Steps, Step{
		HostID: founder.ID, Address: founder.Address, NodeName: founder.NodeName,
		Role: RoleFounder, Phase: PhaseUpdateInventor, State: StepStatePending,
	})

	m.mu.Lock()
	m.jobs[job.ID] = job
	m.mu.Unlock()

	if m.deps.Activity != nil {
		m.deps.Activity.Log(activity.Entry{
			Action:       "pve_cluster_create_start",
			ResourceType: "cluster",
			ResourceName: req.ClusterName,
			Cluster:      req.ClusterName,
			Details:      fmt.Sprintf("founder=%s joiners=%d", founder.NodeName, len(req.Joiners)),
		})
	}

	// Detach from the request ctx — the caller returns as soon as StartJob
	// returns, and we want the orchestration to survive. Bound the run.
	jobCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	go func() {
		defer cancel()
		m.runJob(jobCtx, job, hosts, req)
	}()

	return job.ID, nil
}

// GetJob returns a safe snapshot of the given job, or ok=false if unknown.
func (m *Manager) GetJob(id string) (JobSnapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok {
		return JobSnapshot{}, false
	}
	return job.Snapshot(), true
}

// mustFind is a tiny helper used while seeding job steps.
func mustFind(hosts []*inventory.InventoryHost, id string) *inventory.InventoryHost {
	for _, h := range hosts {
		if h.ID == id {
			return h
		}
	}
	return &inventory.InventoryHost{ID: id}
}
