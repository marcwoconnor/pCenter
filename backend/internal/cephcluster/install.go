package cephcluster

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/moconnor/pcenter/internal/pve"
)

// healthyTimeout is how long PhaseWaitHealthy will wait for the cluster to
// reach HEALTH_OK or HEALTH_WARN before giving up. Generous because first
// MON+MGR initialization on slow disks can take a couple minutes.
const healthyTimeout = 5 * time.Minute

// runInstall is the long-running orchestration goroutine. Every error path is
// responsible for calling job.markFailed() with a recovery-friendly message
// before returning. The Manager spawns this with a fresh background context;
// the HTTP request that triggered StartInstall has long since completed.
func (m *Manager) runInstall(ctx context.Context, job *Job, req InstallRequest) {
	founder := req.Nodes[0]

	slog.Info("ceph install job starting",
		"job", job.ID, "cluster", req.Cluster,
		"founder", founder, "joiners", len(req.Nodes)-1,
		"network", req.Network)

	// Resolve PVE clients for the target nodes. We need them for REST calls
	// (init, MON, MGR) AND for SSH host derivation (Host()).
	allClients := m.deps.Poller.GetClusterClients(req.Cluster)
	clients := make(map[string]*pve.Client, len(req.Nodes))
	for _, node := range req.Nodes {
		c, ok := allClients[node]
		if !ok {
			m.fail(job, node, PhaseInstallPreflight,
				fmt.Sprintf("no PVE client for node %s — is it part of cluster %q?", node, req.Cluster))
			return
		}
		clients[node] = c
	}

	// --- Phase 1: Preflight (re-check at start; the operator might have
	// added a host to an existing Ceph install between preflight and submit).
	if !m.runInstallPreflightPhase(ctx, job, req, clients) {
		return
	}

	// --- Phase 2: pveceph install on each node, in parallel. This is the
	// longest phase (apt) — easily 1-3 min per node.
	if !m.runInstallPackagesPhase(ctx, job, req, clients) {
		return
	}

	// --- Phase 3: pveceph init on the founder. REST POST /ceph/init.
	if !m.runCephInitPhase(ctx, job, req, clients[founder]) {
		return
	}

	// --- Phase 4: Create MON on each node (founder first, then joiners
	// serially — concurrent MON creation can race on monmap updates).
	if !m.runCreateMONsPhase(ctx, job, req, clients) {
		return
	}

	// --- Phase 5: Create MGR on the founder. Operators add additional
	// standby MGRs day-2 from the Monitors tab.
	if !m.runCreateMGRPhase(ctx, job, founder, clients[founder]) {
		return
	}

	// --- Phase 6: Wait for HEALTH_OK or HEALTH_WARN (clock-skew during
	// fresh init is a HEALTH_WARN that resolves on its own; don't fail on it).
	if !m.runWaitHealthyPhase(ctx, job, founder, clients[founder]) {
		return
	}

	job.markSucceeded()
	slog.Info("ceph install job succeeded", "job", job.ID, "cluster", req.Cluster)
}

func (m *Manager) runInstallPreflightPhase(
	ctx context.Context,
	job *Job,
	req InstallRequest,
	clients map[string]*pve.Client,
) bool {
	for _, node := range req.Nodes {
		job.startStep(node, PhaseInstallPreflight)
	}

	pre := RunInstallPreflight(ctx, func(node string) (*pve.Client, bool) {
		c, ok := clients[node]
		return c, ok
	}, req.Nodes)

	for _, hr := range pre.Hosts {
		if len(hr.Blockers) > 0 {
			job.failStep(hr.Node, PhaseInstallPreflight, strings.Join(hr.Blockers, "; "))
		} else {
			msg := fmt.Sprintf("PVE %s, ready", hr.PVEVersion)
			job.succeedStep(hr.Node, PhaseInstallPreflight, msg)
		}
	}

	if !pre.CanProceed {
		m.fail(job, "", "", "preflight failed: "+pre.Message)
		return false
	}
	return true
}

func (m *Manager) runInstallPackagesPhase(
	ctx context.Context,
	job *Job,
	req InstallRequest,
	clients map[string]*pve.Client,
) bool {
	// Parallel apt installs. apt locks per-host (dpkg lock) so cross-host
	// parallelism is safe; same-host concurrent installs would block.
	var wg sync.WaitGroup
	results := make(map[string]error, len(req.Nodes))
	outputs := make(map[string]string, len(req.Nodes))
	var resultsMu sync.Mutex

	for _, node := range req.Nodes {
		node := node
		c := clients[node]
		host := stripPort(c.Host())
		job.startStep(node, PhaseInstallPackages)

		wg.Add(1)
		go func() {
			defer wg.Done()
			// 10-minute hard timeout per node (apt + ceph package downloads).
			cmdCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()

			// We can't rely on `pveceph install -y` — PVE 9 dropped the -y
			// option and its internal apt-get invocation doesn't pass -y
			// either, so it aborts at the "Do you want to continue?" prompt.
			// Two-step approach: run pveceph install (best-effort) so the
			// repo gets configured if it isn't already, then drive the
			// actual package install with apt-get -y. The final test for
			// /usr/bin/ceph-mon is the source of truth — without it, the
			// step fails regardless of how the install path exited.
			//
			// `set -o pipefail` matters: piping through `tail` for output
			// trimming used to mask the underlying exit code, so apt
			// failures looked like successes (#GH issue context: the
			// install_packages step reported success while no daemon
			// packages were ever placed on the node).
			// Install both `ceph` (the meta-package that pulls daemons —
			// ceph-mon, ceph-osd, ceph-mgr, ceph-mds) AND `ceph-volume`
			// explicitly. ceph-volume is the LVM-based OSD provisioning
			// tool that `pveceph osd create` shells out to; without it
			// you can complete the install but never bring up an OSD.
			// It Depends on ceph-osd (not the reverse), so the meta-pkg
			// alone doesn't pull it under --no-install-recommends.
			//
			// Verifying both binaries afterward catches a partial install
			// before the wizard advances and leaves a useless cluster.
			cmd := "set -o pipefail; " +
				"pveceph install --repository no-subscription 2>&1 | tail -200; " +
				"DEBIAN_FRONTEND=noninteractive apt-get install -y " +
				"--no-install-recommends --allow-downgrades ceph ceph-volume 2>&1 | tail -50; " +
				"test -x /usr/bin/ceph-mon && test -x /usr/sbin/ceph-volume"
			out, err := pve.RunSSHCommand(cmdCtx, host, cmd)
			resultsMu.Lock()
			results[node] = err
			outputs[node] = out
			job.setStepMessage(node, PhaseInstallPackages, lastLine(out))
			resultsMu.Unlock()
		}()
	}
	wg.Wait()

	// Mark steps based on results.
	failed := []string{}
	for _, node := range req.Nodes {
		if err := results[node]; err != nil {
			detail := err.Error()
			if last := lastLine(outputs[node]); last != "" {
				detail = fmt.Sprintf("%s (last line: %s)", detail, last)
			}
			job.failStep(node, PhaseInstallPackages, detail)
			failed = append(failed, node)
		} else {
			job.succeedStep(node, PhaseInstallPackages, "ceph daemons installed")
		}
	}
	if len(failed) > 0 {
		m.fail(job, "", "",
			fmt.Sprintf("ceph package install failed on: %s. Recovery: SSH each failed node and run `apt-get install -y --no-install-recommends ceph` to see the apt error directly, then retry the wizard once apt is unblocked. Common causes: enterprise repo selected without a subscription (use --repository no-subscription), version skew between an existing ceph-common and the daemon packages, or a held package preventing the install.",
				strings.Join(failed, ", ")))
		return false
	}
	return true
}

func (m *Manager) runCephInitPhase(
	ctx context.Context,
	job *Job,
	req InstallRequest,
	founder *pve.Client,
) bool {
	host := founder.NodeName()
	job.startStep(host, PhaseCephInit)

	if err := founder.InitCephCluster(ctx, pve.InitCephClusterOptions{
		Network:        req.Network,
		ClusterNetwork: req.ClusterNetwork,
		Size:           req.PoolSize,
		MinSize:        req.MinSize,
	}); err != nil {
		job.failStep(host, PhaseCephInit, err.Error())
		m.fail(job, "", "",
			fmt.Sprintf("pveceph init failed on %s: %v. Recovery: SSH the founder and run `pveceph purge` to fully reset, then re-run the wizard.", host, err))
		return false
	}
	job.succeedStep(host, PhaseCephInit,
		fmt.Sprintf("network=%s size=%d min_size=%d", req.Network, req.PoolSize, req.MinSize))
	return true
}

func (m *Manager) runCreateMONsPhase(
	ctx context.Context,
	job *Job,
	req InstallRequest,
	clients map[string]*pve.Client,
) bool {
	// Serial — concurrent MON creation can race on monmap updates.
	for _, node := range req.Nodes {
		c := clients[node]
		job.startStep(node, PhaseCreateMON)
		upid, err := c.CreateCephMON(ctx, "")
		if err != nil {
			job.failStep(node, PhaseCreateMON, err.Error())
			m.fail(job, "", "",
				fmt.Sprintf("MON create on %s failed: %v. Recovery: pveceph mon destroy %s on the founder, then retry the wizard.", node, err, node))
			return false
		}
		job.setStepUPID(node, PhaseCreateMON, upid)
		// Wait for the create task to finish so the next MON sees a stable
		// monmap. WaitForTask polls /tasks/{upid}/status.
		if _, err := c.WaitForTask(ctx, upid, 5*time.Minute); err != nil {
			job.failStep(node, PhaseCreateMON, fmt.Sprintf("task wait failed: %v", err))
			m.fail(job, "", "",
				fmt.Sprintf("MON create on %s timed out: %v", node, err))
			return false
		}
		job.succeedStep(node, PhaseCreateMON, "MON joined quorum")
	}
	return true
}

func (m *Manager) runCreateMGRPhase(
	ctx context.Context,
	job *Job,
	founder string,
	c *pve.Client,
) bool {
	job.startStep(founder, PhaseCreateMGR)
	upid, err := c.CreateCephMGR(ctx)
	if err != nil {
		job.failStep(founder, PhaseCreateMGR, err.Error())
		m.fail(job, "", "",
			fmt.Sprintf("MGR create on %s failed: %v. Recovery: pveceph mgr destroy %s and retry, or add the MGR via the Monitors tab once the install completes.", founder, err, founder))
		return false
	}
	job.setStepUPID(founder, PhaseCreateMGR, upid)
	if _, err := c.WaitForTask(ctx, upid, 5*time.Minute); err != nil {
		job.failStep(founder, PhaseCreateMGR, fmt.Sprintf("task wait failed: %v", err))
		m.fail(job, "", "", fmt.Sprintf("MGR create on %s timed out: %v", founder, err))
		return false
	}
	job.succeedStep(founder, PhaseCreateMGR, "MGR active; add standbys day-2 from the Monitors tab")
	return true
}

func (m *Manager) runWaitHealthyPhase(
	ctx context.Context,
	job *Job,
	founder string,
	c *pve.Client,
) bool {
	_ = founder // step is cluster-scoped (Host="")
	job.startStep("", PhaseWaitHealthy)

	deadline := time.Now().Add(healthyTimeout)
	pollCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	for {
		if pollCtx.Err() != nil {
			job.failStep("", PhaseWaitHealthy,
				fmt.Sprintf("cluster did not reach a healthy state within %s — visit the Status tab to inspect", healthyTimeout))
			m.fail(job, "", "",
				"timed out waiting for cluster health. Install probably succeeded; check the Status tab.")
			return false
		}
		status, err := c.GetCephStatus(pollCtx)
		if err == nil && status != nil {
			h := status.Health.Status
			if h == "HEALTH_OK" || h == "HEALTH_WARN" {
				job.succeedStep("", PhaseWaitHealthy, "cluster reached "+h)
				return true
			}
		}
		select {
		case <-pollCtx.Done():
			// Loop top will handle it.
		case <-time.After(5 * time.Second):
		}
	}
}

// stripPort removes the :port suffix from a host:port string, leaving just
// the host (or IP) for SSH. Returns the input unchanged if no colon.
func stripPort(hostPort string) string {
	if i := strings.LastIndex(hostPort, ":"); i >= 0 {
		return hostPort[:i]
	}
	return hostPort
}

// lastLine returns the last non-empty line of out, or out itself if it has no
// newlines. Used to surface the "interesting bit" of multi-line command output
// in step messages without dumping a 50-line apt log.
func lastLine(out string) string {
	out = strings.TrimRight(out, "\n")
	if i := strings.LastIndex(out, "\n"); i >= 0 {
		return out[i+1:]
	}
	return out
}
