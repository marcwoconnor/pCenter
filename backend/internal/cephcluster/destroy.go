package cephcluster

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/moconnor/pcenter/internal/pve"
)

// DestroyRequest is the input to StartDestroy. Confirm must equal the cluster
// name verbatim — a deliberate guard against fat-finger destruction. The UI
// presents a typed-confirmation field that produces this value.
type DestroyRequest struct {
	Cluster string
	Confirm string // must equal Cluster
}

// StartDestroy validates, seeds a destroy Job, and kicks off the orchestration.
func (m *Manager) StartDestroy(ctx context.Context, req DestroyRequest) (string, error) {
	if req.Cluster == "" {
		return "", fmt.Errorf("cluster is required")
	}
	if req.Confirm != req.Cluster {
		return "", fmt.Errorf("confirm value must equal cluster name verbatim")
	}

	// Snapshot the cluster's clients up-front so the job orchestration
	// doesn't race with poller updates.
	allClients := m.deps.Poller.GetClusterClients(req.Cluster)
	if len(allClients) == 0 {
		return "", fmt.Errorf("no PVE clients available for cluster %q", req.Cluster)
	}

	job := &Job{
		ID:        uuid.NewString(),
		Kind:      JobKindDestroy,
		Cluster:   req.Cluster,
		State:     JobStateRunning,
		StartedAt: time.Now(),
	}

	// Seed top-level steps. Per-node ceph_purge steps get added at runtime
	// since the node list comes from the live client map.
	for _, phase := range destroyClusterPhases() {
		job.Steps = append(job.Steps, Step{Phase: phase, State: StepStatePending})
	}
	for nodeName := range allClients {
		job.Steps = append(job.Steps, Step{Host: nodeName, Phase: PhaseCephPurge, State: StepStatePending})
	}

	m.mu.Lock()
	m.jobs[job.ID] = job
	m.mu.Unlock()

	go m.runDestroy(context.Background(), job, req, allClients)

	return job.ID, nil
}

// destroyClusterPhases is the canonical, ordered list of cluster-scoped
// destroy phases. Per-node ceph_purge steps are appended after these in
// the seeded Steps slice. Extracted so tests can assert ordering without
// re-running the full job — `pveceph purge` aborts when MONs or OSDs are
// still configured, so PhaseDeleteOSD and PhaseDeleteMON MUST stay before
// the implicit PhaseCephPurge tail.
func destroyClusterPhases() []StepPhase {
	return []StepPhase{
		PhaseDestroyPreflight,
		PhaseSetNoout,
		PhaseDeleteFS,
		PhaseDeletePools,
		PhaseDeleteMDS,
		PhaseDeleteOSD,
		PhaseDeleteMGR,
		PhaseDeleteMON,
	}
}

// runDestroy is the long-running orchestration goroutine for a destroy Job.
// Order matters: pools/fs MUST go before MDS (FS needs MDSs to drain);
// OSDs after MDS but before MGR/MON (MGR helps OSD destroy issue
// safe-to-destroy checks promptly; MONs must outlive OSD destroy because
// each `ceph osd destroy` writes the OSDmap); MGR before MON (so the
// orchestration MGR can still publish cluster updates as MONs go away);
// pveceph purge LAST (it removes /etc/ceph and the keyrings — anything
// that needs Ceph auth must complete first, and `pveceph purge` itself
// aborts when MONs or OSDs are still configured, so the prior phases
// must finish before it can succeed).
//
// Bias: continue past per-resource failures and surface them in step.error
// rather than bail. An operator destroying Ceph wants the cluster to end
// up gone; one stuck pool shouldn't leave them with half a cluster.
func (m *Manager) runDestroy(
	ctx context.Context,
	job *Job,
	req DestroyRequest,
	clients map[string]*pve.Client,
) {
	slog.Info("ceph destroy job starting",
		"job", job.ID, "cluster", req.Cluster, "nodes", len(clients))

	// Pick a "lead" client — the one we'll route cluster-wide REST calls
	// through. Any client works; PVE handles the cluster-wide ops itself.
	var lead *pve.Client
	for _, c := range clients {
		lead = c
		break
	}

	// --- Phase 1: Preflight. Confirm Ceph is installed. If not, succeed
	// the job immediately so re-running on an already-clean cluster is OK.
	job.startStep("", PhaseDestroyPreflight)
	status, err := lead.GetCephStatus(ctx)
	if err != nil || status == nil {
		job.succeedStep("", PhaseDestroyPreflight, "Ceph not installed; nothing to destroy")
		// Still run pveceph purge per node — leftover packages / configs
		// from a previous half-destroy are common.
		m.runPurgeAcrossNodes(ctx, job, clients)
		m.finalizeDestroyResult(job)
		return
	}
	job.succeedStep("", PhaseDestroyPreflight, "Ceph detected; proceeding")

	// --- Phase 2: Set noout. Prevent CRUSH rebalancing during teardown
	// (a moot point since we're about to delete everything, but it stops
	// background work that could slow down DELETE responses).
	job.startStep("", PhaseSetNoout)
	if err := lead.SetCephFlag(ctx, "noout", true); err != nil {
		job.failStep("", PhaseSetNoout, fmt.Sprintf("could not set noout: %v (continuing anyway)", err))
	} else {
		job.succeedStep("", PhaseSetNoout, "noout set")
	}

	// --- Phase 3: Delete CephFS (BEFORE pools so the pool-delete phase
	// doesn't trip over FS-owned pools).
	job.startStep("", PhaseDeleteFS)
	fsList, _ := lead.ListCephFS(ctx)
	fsErrs := []string{}
	for _, fs := range fsList {
		// remove_pools=false here — we'll handle pools in the next phase
		// so the per-pool error reporting is finer-grained.
		if _, err := lead.DeleteCephFS(ctx, fs.Name, pve.CephFSDeleteOptions{RemoveStorages: true}); err != nil {
			fsErrs = append(fsErrs, fmt.Sprintf("%s: %v", fs.Name, err))
		}
	}
	if len(fsErrs) > 0 {
		job.failStep("", PhaseDeleteFS, fmt.Sprintf("%d/%d FS deletes failed: %v", len(fsErrs), len(fsList), fsErrs))
	} else {
		job.succeedStep("", PhaseDeleteFS, fmt.Sprintf("%d filesystem(s) deleted", len(fsList)))
	}

	// --- Phase 4: Delete pools (with remove_storages=true so PVE Storage
	// entries don't outlive the pool).
	job.startStep("", PhaseDeletePools)
	pools, _ := lead.ListCephPools(ctx)
	poolErrs := []string{}
	for _, p := range pools {
		if _, err := lead.DeleteCephPool(ctx, p.Name, pve.CephPoolDeleteOptions{Force: true, RemoveStorages: true}); err != nil {
			poolErrs = append(poolErrs, fmt.Sprintf("%s: %v", p.Name, err))
		}
	}
	if len(poolErrs) > 0 {
		job.failStep("", PhaseDeletePools, fmt.Sprintf("%d/%d pool deletes failed: %v", len(poolErrs), len(pools), poolErrs))
	} else {
		job.succeedStep("", PhaseDeletePools, fmt.Sprintf("%d pool(s) deleted", len(pools)))
	}

	// --- Phase 5: Delete MDSs.
	job.startStep("", PhaseDeleteMDS)
	mdss, _ := lead.ListCephMDSs(ctx)
	mdsErrs := []string{}
	for _, m := range mdss {
		c, ok := clients[m.Host]
		if !ok {
			c = lead
		}
		if _, err := c.DeleteCephMDS(ctx, m.Name); err != nil {
			mdsErrs = append(mdsErrs, fmt.Sprintf("%s on %s: %v", m.Name, m.Host, err))
		}
	}
	if len(mdsErrs) > 0 {
		job.failStep("", PhaseDeleteMDS, fmt.Sprintf("%d/%d MDS deletes failed: %v", len(mdsErrs), len(mdss), mdsErrs))
	} else {
		job.succeedStep("", PhaseDeleteMDS, fmt.Sprintf("%d MDS(s) deleted", len(mdss)))
	}

	// --- Phase 5b: Delete OSDs. `pveceph purge` aborts when OSDs are
	// configured, so this MUST land before phase 8 or the cluster ends up
	// half-torn-down. cleanup=true wipes the LVM so the underlying disks
	// are reusable without manual zap. Each destroy returns a UPID; we
	// wait for it so the next phase doesn't race the OSDmap update. Run
	// serially — concurrent OSD destroys on the same host can deadlock
	// on ceph-volume's LVM lock and there's no value in fanning out
	// across hosts at the cost of harder-to-read failure output.
	job.startStep("", PhaseDeleteOSD)
	osds, osdListErr := lead.ListCephOSDs(ctx)
	if osdListErr != nil {
		job.failStep("", PhaseDeleteOSD,
			fmt.Sprintf("ListCephOSDs failed: %v — pveceph purge will abort if OSDs remain. Recovery: SSH each node and run `pveceph osd destroy <id> --cleanup` for any leftover OSDs before retrying.", osdListErr))
	} else {
		osdErrs := []string{}
		for _, o := range osds {
			c, ok := clients[o.Host]
			if !ok {
				c = lead
			}
			upid, err := c.DeleteCephOSD(ctx, o.ID, true)
			if err != nil {
				osdErrs = append(osdErrs, fmt.Sprintf("osd.%d on %s: %v", o.ID, o.Host, err))
				continue
			}
			// 5 minutes is generous — most OSD destroys finish in <60s; SSDs
			// with large LVM stacks can take longer to wipe.
			if _, werr := c.WaitForTask(ctx, upid, 5*time.Minute); werr != nil {
				osdErrs = append(osdErrs, fmt.Sprintf("osd.%d on %s: task wait failed: %v", o.ID, o.Host, werr))
				continue
			}
		}
		if len(osdErrs) > 0 {
			job.failStep("", PhaseDeleteOSD,
				fmt.Sprintf("%d/%d OSD destroy(s) failed: %v. Recovery: SSH the affected node(s) and run `pveceph osd destroy <id> --cleanup` manually — `pveceph purge` will abort while OSDs remain.",
					len(osdErrs), len(osds), osdErrs))
		} else {
			job.succeedStep("", PhaseDeleteOSD, fmt.Sprintf("%d OSD(s) destroyed", len(osds)))
		}
	}

	// --- Phase 6: Delete MGRs.
	job.startStep("", PhaseDeleteMGR)
	mgrs, _ := lead.ListCephMGRs(ctx)
	mgrErrs := []string{}
	for _, m := range mgrs {
		c, ok := clients[m.Host]
		if !ok {
			c = lead
		}
		if _, err := c.DeleteCephMGR(ctx, m.Name); err != nil {
			mgrErrs = append(mgrErrs, fmt.Sprintf("%s on %s: %v", m.Name, m.Host, err))
		}
	}
	if len(mgrErrs) > 0 {
		job.failStep("", PhaseDeleteMGR, fmt.Sprintf("%d/%d MGR deletes failed: %v", len(mgrErrs), len(mgrs), mgrErrs))
	} else {
		job.succeedStep("", PhaseDeleteMGR, fmt.Sprintf("%d MGR(s) deleted", len(mgrs)))
	}

	// --- Phase 7: Delete MONs. The last MON is special — PVE refuses to
	// remove the only remaining MON unless the cluster is being torn down.
	// `pveceph mon destroy` handles this with the right flags; the API
	// equivalent works too in a destroy context.
	job.startStep("", PhaseDeleteMON)
	mons, _ := lead.ListCephMONs(ctx)
	monErrs := []string{}
	for _, m := range mons {
		c, ok := clients[m.Host]
		if !ok {
			c = lead
		}
		if _, err := c.DeleteCephMON(ctx, m.Name); err != nil {
			monErrs = append(monErrs, fmt.Sprintf("%s on %s: %v", m.Name, m.Host, err))
		}
	}
	if len(monErrs) > 0 {
		job.failStep("", PhaseDeleteMON, fmt.Sprintf("%d/%d MON deletes failed: %v", len(monErrs), len(mons), monErrs))
	} else {
		job.succeedStep("", PhaseDeleteMON, fmt.Sprintf("%d MON(s) deleted", len(mons)))
	}

	// --- Phase 8: pveceph purge per node, in parallel.
	m.runPurgeAcrossNodes(ctx, job, clients)

	m.finalizeDestroyResult(job)
}

// finalizeDestroyResult decides the terminal Job state. Soft rule:
// per-resource teardown failures (pool/FS/MDS/OSD/MGR/MON) surface as step
// errors but don't fail the job — the operator can re-run destroy and
// pveceph purge will mop up. The job is only failed when EVERY per-node
// `pveceph purge` step failed, which is the signal that the cluster is
// genuinely still installed on every host. A purge succeeding on even one
// node means the orchestration did its job; the operator can SSH the
// remaining nodes and run purge by hand.
func (m *Manager) finalizeDestroyResult(job *Job) {
	job.mu.Lock()
	purgeAttempted, purgeSucceeded := 0, 0
	for _, s := range job.Steps {
		if s.Phase != PhaseCephPurge {
			continue
		}
		purgeAttempted++
		if s.State == StepStateSucceeded {
			purgeSucceeded++
		}
	}
	job.mu.Unlock()

	if purgeAttempted > 0 && purgeSucceeded == 0 {
		job.markFailed(fmt.Sprintf(
			"destroy did not complete: pveceph purge failed on all %d node(s). The cluster is in a partially-torn-down state. Recovery: SSH each node and run `pveceph purge --crash --logs` manually to inspect, then re-run destroy once the underlying cause (typically leftover OSDs or MONs) is fixed.",
			purgeAttempted))
		return
	}
	job.markSucceeded()
}

// runPurgeAcrossNodes runs `pveceph purge --crash --logs` on every node
// in parallel and records per-host outcomes. apt locks per-host so the
// fan-out is safe. Each invocation has a 5-minute hard timeout.
func (m *Manager) runPurgeAcrossNodes(ctx context.Context, job *Job, clients map[string]*pve.Client) {
	var wg sync.WaitGroup
	for nodeName, c := range clients {
		nodeName := nodeName
		c := c
		host := stripPort(c.Host())
		job.startStep(nodeName, PhaseCephPurge)

		wg.Add(1)
		go func() {
			defer wg.Done()
			cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			out, err := pve.RunSSHCommand(cmdCtx, host, "pveceph purge --crash --logs 2>&1 | tail -50")
			if err != nil {
				job.failStep(nodeName, PhaseCephPurge,
					fmt.Sprintf("pveceph purge failed: %v\nlast output: %s\nRecovery: SSH the node and run `pveceph purge --crash --logs` manually to inspect.", err, lastLine(out)))
				return
			}
			job.succeedStep(nodeName, PhaseCephPurge, "pveceph purge completed")
		}()
	}
	wg.Wait()
}
