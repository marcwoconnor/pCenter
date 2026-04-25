package pvecluster

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/moconnor/pcenter/internal/activity"
	"github.com/moconnor/pcenter/internal/config"
	"github.com/moconnor/pcenter/internal/inventory"
	"github.com/moconnor/pcenter/internal/pve"
)

// runJob is the long-running orchestration goroutine. Every error path is
// responsible for calling job.markFailed() with a manual-recovery-friendly
// message before returning.
func (m *Manager) runJob(ctx context.Context, job *Job, hosts []*inventory.InventoryHost, req StartJobRequest) {
	founder := hosts[0]
	joiners := hosts[1:]

	slog.Info("pvecluster job starting",
		"job", job.ID, "cluster", req.ClusterName,
		"founder", founder.NodeName, "joiners", len(joiners))

	// PVE's /cluster/config endpoints reject API tokens with "Permission
	// check failed (user != root@pam)". We authenticate the founder with
	// root@pam + password up-front and reuse that ticket for create +
	// fetch-join-info. Task polling still uses the existing API-token client.
	founderAuth, err := pve.AuthenticateWithPassword(ctx, founder.Address, "root@pam", req.FounderPassword, founder.Insecure)
	if err != nil {
		m.fail(job, req.ClusterName,
			fmt.Sprintf("founder %s: root@pam authentication failed: %v", founder.NodeName, err))
		return
	}
	founderClient, err := buildClient(founder)
	if err != nil {
		m.fail(job, req.ClusterName, fmt.Sprintf("build founder client: %v", err))
		return
	}

	// Phase 1: Create the cluster on the founder (password-auth). If a
	// previous failed attempt left corosync.conf behind, /cluster/config
	// returns 500 with "already exists". We auto-recover by SSH'ing in
	// and running the canonical PVE standalone-revert sequence, then
	// retrying create once.
	createOpts := pve.ClusterCreateOptions{
		ClusterName: req.ClusterName,
		Link0:       req.FounderLink0,
	}
	job.startStep(founder.ID, PhaseCreateCluster)
	upid, err := pve.ClusterCreateWithPassword(ctx, founder.Address, founderAuth, createOpts, founder.Insecure)
	if err != nil && isCorosyncAlreadyExists(err) {
		slog.Warn("founder has leftover cluster config; cleaning up and retrying",
			"node", founder.NodeName, "error", err)
		job.setStepMessage(founder.ID, PhaseCreateCluster, "leftover config detected — cleaning up")
		// Insert a cleanup step so the user sees what's happening.
		job.startStep(founder.ID, PhaseCleanupCorosync)
		out, cleanErr := cleanupCorosyncOverSSH(ctx, founder.Address, req.FounderPassword)
		if cleanErr != nil {
			job.failStep(founder.ID, PhaseCleanupCorosync,
				fmt.Sprintf("%v\n--- script output ---\n%s", cleanErr, truncate(out, 600)))
			job.failStep(founder.ID, PhaseCreateCluster, "leftover cluster config; auto-cleanup failed")
			m.fail(job, req.ClusterName,
				fmt.Sprintf("founder %s: leftover cluster config and auto-cleanup over SSH failed: %v. Manual recovery: SSH to %s and run: systemctl stop pve-cluster corosync && pmxcfs -l && rm -f /etc/pve/corosync.conf /etc/corosync/corosync.conf && killall pmxcfs && systemctl start pve-cluster",
					founder.NodeName, cleanErr, founder.NodeName))
			return
		}
		job.succeedStep(founder.ID, PhaseCleanupCorosync, "removed leftover corosync config")

		// pve-cluster restarted → ticket may be invalid. Re-auth.
		freshAuth, authErr := pve.AuthenticateWithPassword(ctx, founder.Address, "root@pam", req.FounderPassword, founder.Insecure)
		if authErr != nil {
			job.failStep(founder.ID, PhaseCreateCluster,
				fmt.Sprintf("re-auth after cleanup failed: %v", authErr))
			m.fail(job, req.ClusterName,
				fmt.Sprintf("founder %s: cleanup ok but re-authentication failed: %v",
					founder.NodeName, authErr))
			return
		}
		founderAuth = freshAuth

		// Retry create.
		job.setStepMessage(founder.ID, PhaseCreateCluster, "retrying after cleanup")
		upid, err = pve.ClusterCreateWithPassword(ctx, founder.Address, founderAuth, createOpts, founder.Insecure)
	}
	if err != nil {
		job.failStep(founder.ID, PhaseCreateCluster, err.Error())
		m.fail(job, req.ClusterName,
			fmt.Sprintf("founder %s: create cluster failed: %v", founder.NodeName, err))
		return
	}
	job.setStepUPID(founder.ID, PhaseCreateCluster, upid)
	job.setStepMessage(founder.ID, PhaseCreateCluster, "waiting for task "+shortUPID(upid))

	createCtx, cancelCreate := context.WithTimeout(ctx, 90*time.Second)
	_, err = founderClient.WaitForTask(createCtx, upid, 2*time.Second)
	cancelCreate()
	if err != nil {
		job.failStep(founder.ID, PhaseCreateCluster, err.Error())
		m.fail(job, req.ClusterName,
			fmt.Sprintf("founder %s: cluster creation task failed: %v", founder.NodeName, err))
		return
	}
	job.succeedStep(founder.ID, PhaseCreateCluster, "cluster formed")

	// Phase 2: Fetch join info (password-auth). Corosync may still be
	// finalizing; retry briefly. We also re-authenticate at the start of
	// each retry — cluster creation rewrites /etc/pve and frequently
	// invalidates the existing ticket.
	job.startStep(founder.ID, PhaseFetchJoinInfo)
	var joinInfo *pve.ClusterJoinInfo
	var lastErr error
	for attempt := 1; attempt <= 8; attempt++ {
		// Refresh the ticket so we don't keep using a cookie pmxcfs has
		// dropped. Cheap (one HTTPS round-trip) and removes a whole class
		// of "ticket invalid after cluster create" failures.
		freshAuth, authErr := pve.AuthenticateWithPassword(ctx, founder.Address, "root@pam", req.FounderPassword, founder.Insecure)
		if authErr != nil {
			lastErr = fmt.Errorf("re-auth: %w", authErr)
		} else {
			founderAuth = freshAuth
			joinInfo, err = pve.GetClusterJoinInfoWithPassword(ctx, founder.Address, founderAuth, founder.NodeName, founder.Insecure)
			if err == nil && joinInfo != nil && joinInfo.Fingerprint != "" {
				break
			}
			if err != nil {
				lastErr = err
			} else {
				lastErr = fmt.Errorf("join info returned without fingerprint")
			}
		}
		msg := fmt.Sprintf("retry %d/8: %v", attempt, lastErr)
		job.setStepMessage(founder.ID, PhaseFetchJoinInfo, msg)
		select {
		case <-ctx.Done():
			m.fail(job, req.ClusterName, "cancelled during join-info fetch")
			return
		case <-time.After(3 * time.Second):
		}
	}
	if joinInfo == nil || joinInfo.Fingerprint == "" {
		msg := "join info never populated"
		if lastErr != nil {
			msg = lastErr.Error()
		}
		job.failStep(founder.ID, PhaseFetchJoinInfo, msg)
		m.fail(job, req.ClusterName,
			fmt.Sprintf("founder %s: could not fetch join info: %s. Manual recovery: SSH to %s and run 'pvecm expected 1', verify with 'pvecm status', then either keep the cluster (use it from PVE web UI) or run 'systemctl stop pve-cluster && rm /etc/pve/corosync.conf /etc/corosync/corosync.conf && systemctl start pve-cluster' to revert to standalone before retrying.",
				founder.NodeName, msg, founder.NodeName))
		return
	}
	job.succeedStep(founder.ID, PhaseFetchJoinInfo,
		fmt.Sprintf("fingerprint %s", truncate(joinInfo.Fingerprint, 20)))

	// Phase 3: Join each joiner serially.
	founderIP := hostPart(founder.Address)
	tokenUpdates := map[string]struct{ TokenID, TokenSecret string }{}
	for _, joinerReq := range req.Joiners {
		joiner := findHost(joiners, joinerReq.HostID)
		if joiner == nil {
			m.fail(job, req.ClusterName,
				fmt.Sprintf("internal error: joiner %s not in hosts list", joinerReq.HostID))
			return
		}
		if err := m.joinOne(ctx, job, founder, founderIP, joiner, joinerReq, joinInfo); err != nil {
			// joinOne has already marked its own steps failed.
			m.fail(job, req.ClusterName,
				fmt.Sprintf("joiner %s: %v. Manual recovery: on %s run 'pvecm delnode %s' to drop any half-joined state, 'pvecm status' to verify quorum, then retry from pcenter after cleanup.",
					joiner.NodeName, err, founder.NodeName, joiner.NodeName))
			return
		}
		// Grab credential update for the joiner if joinOne minted a fresh token.
		if h, _ := m.deps.Inventory.GetHost(ctx, joiner.ID); h != nil {
			// joinOne wrote directly, but we also need to tell FormClusterFromHosts
			// so the DB update is coherent. We pass the latest token through the
			// tokenUpdates map using the values we just persisted.
			tokenUpdates[joiner.ID] = struct {
				TokenID, TokenSecret string
			}{TokenID: h.TokenID, TokenSecret: h.TokenSecret}
		}
	}

	// Phase 4: pcenter inventory update.
	job.startStep(founder.ID, PhaseUpdateInventor)
	hostIDs := []string{founder.ID}
	for _, j := range req.Joiners {
		hostIDs = append(hostIDs, j.HostID)
	}
	cluster, err := m.deps.Inventory.FormClusterFromHosts(ctx, inventory.FormClusterFromHostsRequest{
		Name:         req.ClusterName,
		AgentName:    req.ClusterName,
		DatacenterID: req.DatacenterID,
		HostIDs:      hostIDs,
		TokenUpdates: tokenUpdates,
	})
	if err != nil {
		job.failStep(founder.ID, PhaseUpdateInventor, err.Error())
		m.fail(job, req.ClusterName,
			fmt.Sprintf("PVE cluster formed successfully but pcenter inventory update failed: %v. You can fix this manually: delete the standalone host rows in pcenter and rescan — auto-discovery will pick up the new cluster.",
				err))
		return
	}
	job.succeedStep(founder.ID, PhaseUpdateInventor,
		fmt.Sprintf("inventory cluster %s created", cluster.ID))

	// Phase 5: Poller reconfiguration. Add the new cluster, remove the old
	// per-host standalone pollers. Errors here are logged but non-fatal —
	// the poller will catch up on next reconcile regardless.
	if m.deps.Poller != nil {
		m.deps.Poller.AddCluster(config.ClusterConfig{
			Name:          req.ClusterName,
			DiscoveryNode: founder.Address,
			TokenID:       founder.TokenID,
			TokenSecret:   founder.TokenSecret,
			Insecure:      founder.Insecure,
		})
		for _, id := range hostIDs {
			m.deps.Poller.RemoveCluster("standalone:" + id)
		}
	}

	job.markSucceeded(cluster.ID)
	slog.Info("pvecluster job succeeded", "job", job.ID, "cluster", req.ClusterName)
	if m.deps.Activity != nil {
		m.deps.Activity.Log(activity.Entry{
			Action:       "pve_cluster_create_success",
			ResourceType: "cluster",
			ResourceID:   cluster.ID,
			ResourceName: req.ClusterName,
			Cluster:      req.ClusterName,
			Details:      fmt.Sprintf("%d hosts", len(hostIDs)),
		})
	}
}

// runJoinHostsJob is the orchestration goroutine for adding new member nodes
// to an already-existing PVE cluster. Compared to runJob, it skips the
// founder-create + password-auth-fetch-join-info phases — instead it uses
// the existing cluster's stored API token to fetch join info, then runs the
// per-joiner flow identically.
func (m *Manager) runJoinHostsJob(
	ctx context.Context,
	job *Job,
	cluster *inventory.Cluster,
	sourceHost *inventory.InventoryHost,
	joiners []*inventory.InventoryHost,
	req StartJoinHostsJobRequest,
) {
	slog.Info("pvecluster join job starting",
		"job", job.ID, "cluster", cluster.Name,
		"source", sourceHost.NodeName, "joiners", len(joiners))

	// Phase 1: Fetch join info from the existing member (token auth).
	sourceClient, err := buildClient(sourceHost)
	if err != nil {
		m.fail(job, cluster.Name, fmt.Sprintf("build source-host client: %v", err))
		return
	}
	job.startStep(sourceHost.ID, PhaseFetchJoinInfo)
	var joinInfo *pve.ClusterJoinInfo
	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		joinInfo, err = sourceClient.GetClusterJoinInfo(ctx, sourceHost.NodeName)
		if err == nil && joinInfo != nil && joinInfo.Fingerprint != "" {
			break
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("join info returned without fingerprint")
		}
		job.setStepMessage(sourceHost.ID, PhaseFetchJoinInfo,
			fmt.Sprintf("retry %d/5: %v", attempt, lastErr))
		select {
		case <-ctx.Done():
			m.fail(job, cluster.Name, "cancelled during join-info fetch")
			return
		case <-time.After(2 * time.Second):
		}
	}
	if joinInfo == nil || joinInfo.Fingerprint == "" {
		msg := "join info never populated"
		if lastErr != nil {
			msg = lastErr.Error()
		}
		job.failStep(sourceHost.ID, PhaseFetchJoinInfo, msg)
		m.fail(job, cluster.Name,
			fmt.Sprintf("source %s: could not fetch join info: %s", sourceHost.NodeName, msg))
		return
	}
	job.succeedStep(sourceHost.ID, PhaseFetchJoinInfo,
		fmt.Sprintf("fingerprint %s", truncate(joinInfo.Fingerprint, 20)))

	// Phase 2: Join each new node. Same per-joiner flow as the create path.
	sourceIP := hostPart(sourceHost.Address)
	tokenUpdates := map[string]struct{ TokenID, TokenSecret string }{}
	for _, joinerReq := range req.Joiners {
		joiner := findHost(joiners, joinerReq.HostID)
		if joiner == nil {
			m.fail(job, cluster.Name,
				fmt.Sprintf("internal error: joiner %s not in hosts list", joinerReq.HostID))
			return
		}
		if err := m.joinOne(ctx, job, sourceHost, sourceIP, joiner, joinerReq, joinInfo); err != nil {
			m.fail(job, cluster.Name,
				fmt.Sprintf("joiner %s: %v. Manual recovery: on %s run 'pvecm delnode %s' to drop any half-joined state, 'pvecm status' to verify quorum, then retry from pcenter after cleanup.",
					joiner.NodeName, err, sourceHost.NodeName, joiner.NodeName))
			return
		}
		if h, _ := m.deps.Inventory.GetHost(ctx, joiner.ID); h != nil {
			tokenUpdates[joiner.ID] = struct{ TokenID, TokenSecret string }{
				TokenID: h.TokenID, TokenSecret: h.TokenSecret,
			}
		}
	}

	// Phase 3: pcenter inventory update. Move the new hosts into the
	// existing cluster row (no new cluster created).
	job.startStep(sourceHost.ID, PhaseUpdateInventor)
	hostIDs := make([]string, 0, len(req.Joiners))
	for _, j := range req.Joiners {
		hostIDs = append(hostIDs, j.HostID)
	}
	if err := m.deps.Inventory.AddHostsToCluster(ctx, inventory.AddHostsToClusterRequest{
		ClusterID:    cluster.ID,
		HostIDs:      hostIDs,
		TokenUpdates: tokenUpdates,
	}); err != nil {
		job.failStep(sourceHost.ID, PhaseUpdateInventor, err.Error())
		m.fail(job, cluster.Name,
			fmt.Sprintf("PVE join completed but pcenter inventory update failed: %v. Recover by removing the standalone host rows in pcenter and rescanning — the cluster will rediscover them.", err))
		return
	}
	job.succeedStep(sourceHost.ID, PhaseUpdateInventor,
		fmt.Sprintf("%d host(s) added to %s", len(hostIDs), cluster.Name))

	// Phase 4: Drop the obsolete per-host standalone pollers; the cluster
	// itself is already polled, no need to AddCluster.
	if m.deps.Poller != nil {
		for _, id := range hostIDs {
			m.deps.Poller.RemoveCluster("standalone:" + id)
		}
	}

	job.markSucceeded(cluster.ID)
	slog.Info("pvecluster join job succeeded", "job", job.ID, "cluster", cluster.Name)
	if m.deps.Activity != nil {
		m.deps.Activity.Log(activity.Entry{
			Action:       "pve_cluster_join_success",
			ResourceType: "cluster",
			ResourceID:   cluster.ID,
			ResourceName: cluster.Name,
			Cluster:      cluster.Name,
			Details:      fmt.Sprintf("%d host(s) added", len(hostIDs)),
		})
	}
}

// joinOne handles the per-joiner flow: authenticate with root@pam password,
// POST /cluster/config/join, watch the founder's /cluster/status for
// membership, then update the joiner's stored credentials. Updates the
// joiner's job steps along the way.
//
// IMPORTANT: after a successful PVE join, /etc/pve is shared across all
// cluster members via pmxcfs — including /etc/pve/priv/token.cfg. Calling
// CreateAPIToken on the joiner here would delete-and-recreate the `pcenter`
// token cluster-wide, invalidating the founder/source's stored secret too.
// Instead we copy the source's existing token into the joiner's record:
// after join the joiner can authenticate with that same token.
func (m *Manager) joinOne(
	ctx context.Context,
	job *Job,
	founder *inventory.InventoryHost,
	founderIP string,
	joiner *inventory.InventoryHost,
	req JoinerSpec,
	joinInfo *pve.ClusterJoinInfo,
) error {
	job.startStep(joiner.ID, PhaseJoin)

	link0 := req.Link0
	if link0 == "" {
		link0 = hostPart(joiner.Address)
	}

	auth, err := pve.AuthenticateWithPassword(ctx, joiner.Address, "root@pam", req.Password, joiner.Insecure)
	if err != nil {
		job.failStep(joiner.ID, PhaseJoin, err.Error())
		return fmt.Errorf("authenticate: %w", err)
	}

	job.setStepMessage(joiner.ID, PhaseJoin, "submitting join request")
	joinCtx, cancelJoin := context.WithTimeout(ctx, 240*time.Second)
	upid, err := pve.ClusterJoin(joinCtx, joiner.Address, auth, pve.ClusterJoinRequest{
		Hostname:    founderIP,
		Fingerprint: joinInfo.Fingerprint,
		Password:    req.Password,
		Link0:       link0,
	}, joiner.Insecure)
	cancelJoin()
	if err != nil && !looksLikeProxyRestart(err) {
		job.failStep(joiner.ID, PhaseJoin, err.Error())
		return fmt.Errorf("join: %w", err)
	}
	if upid != "" {
		job.setStepUPID(joiner.ID, PhaseJoin, upid)
	}
	job.setStepMessage(joiner.ID, PhaseJoin, "waiting for node to appear in cluster")

	// Real success signal: the founder's /cluster/status lists the joiner
	// as type=node, online=1. pveproxy on the joiner bounces during join,
	// so we watch from the founder side.
	if err := waitForMembership(ctx, founder, joiner.NodeName, 180*time.Second); err != nil {
		job.failStep(joiner.ID, PhaseJoin, err.Error())
		return fmt.Errorf("waiting for membership: %w", err)
	}
	job.succeedStep(joiner.ID, PhaseJoin, "joined cluster")

	// Phase: update the joiner's stored credentials to use the source's
	// existing token. Now that /etc/pve is shared, that same token works
	// against the joiner. We don't recreate — see the function comment.
	job.startStep(joiner.ID, PhaseReauthToken)
	if founder.TokenID == "" || founder.TokenSecret == "" {
		job.failStep(joiner.ID, PhaseReauthToken, "source host has no stored token")
		return fmt.Errorf("source host %s has no stored API token to copy", founder.NodeName)
	}
	if err := m.deps.Inventory.UpdateHost(ctx, joiner.ID, inventory.UpdateHostRequest{
		Address:     joiner.Address,
		TokenID:     founder.TokenID,
		TokenSecret: founder.TokenSecret,
		Insecure:    joiner.Insecure,
	}); err != nil {
		job.failStep(joiner.ID, PhaseReauthToken, err.Error())
		return fmt.Errorf("persist token: %w", err)
	}
	// Mirror the in-memory record so subsequent steps see the new creds.
	joiner.TokenID = founder.TokenID
	joiner.TokenSecret = founder.TokenSecret
	job.succeedStep(joiner.ID, PhaseReauthToken, "shared cluster token applied")

	return nil
}

// waitForMembership polls the founder's /cluster/status until joinerNode
// appears as type=node with online=1, or ctx deadline.
func waitForMembership(parent context.Context, founder *inventory.InventoryHost, joinerNode string, timeout time.Duration) error {
	client, err := buildClient(founder)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		nodes, err := client.DiscoverClusterNodes(ctx)
		if err == nil {
			for _, n := range nodes {
				if n.Name == joinerNode && n.Online == 1 {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for %s to appear in /cluster/status", joinerNode)
		case <-ticker.C:
		}
	}
}

// fail centralizes the "mark job failed + log activity" tail.
func (m *Manager) fail(job *Job, clusterName, msg string) {
	job.markFailed(msg)
	slog.Error("pvecluster job failed", "job", job.ID, "cluster", clusterName, "error", msg)
	if m.deps.Activity != nil {
		m.deps.Activity.Log(activity.Entry{
			Action:       "pve_cluster_create_fail",
			ResourceType: "cluster",
			ResourceName: clusterName,
			Cluster:      clusterName,
			Details:      msg,
		})
	}
}

// --- helpers ---

// buildClient constructs a pve.Client from a stored InventoryHost's creds.
func buildClient(h *inventory.InventoryHost) (*pve.Client, error) {
	if h.TokenID == "" || h.TokenSecret == "" {
		return nil, fmt.Errorf("host %s has no stored token", h.Address)
	}
	c := pve.NewClientFromClusterConfig(config.ClusterConfig{
		Name:          "pvecluster:" + h.ID,
		DiscoveryNode: h.Address,
		TokenID:       h.TokenID,
		TokenSecret:   h.TokenSecret,
		Insecure:      h.Insecure,
	})
	if h.NodeName != "" {
		c.SetNodeName(h.NodeName)
	}
	return c, nil
}

// hostPart strips the port from "10.0.0.1:8006" → "10.0.0.1".
func hostPart(addrPort string) string {
	if i := strings.LastIndex(addrPort, ":"); i > 0 {
		// Be careful with IPv6 literals — but for now the codebase assumes v4.
		return addrPort[:i]
	}
	return addrPort
}

// shortUPID trims a UPID to something the user won't visually choke on.
func shortUPID(upid string) string {
	if len(upid) <= 24 {
		return upid
	}
	return upid[:12] + "…" + upid[len(upid)-8:]
}

// truncate trims a string to n chars with an ellipsis if needed.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// findHost looks up a joiner inventory record by ID within the slice the
// manager seeded. Returns nil if missing (shouldn't happen — StartJob
// validated the IDs).
func findHost(hosts []*inventory.InventoryHost, id string) *inventory.InventoryHost {
	for _, h := range hosts {
		if h.ID == id {
			return h
		}
	}
	return nil
}

// isCorosyncAlreadyExists matches the PVE 500 we get when a previous failed
// attempt left /etc/pve/corosync.conf or /etc/corosync/corosync.conf behind.
// Triggers the auto-cleanup flow.
func isCorosyncAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "corosync.conf") &&
		strings.Contains(msg, "already exists")
}

// looksLikeProxyRestart heuristically identifies the "pveproxy restarted
// mid-response during join" case, where the HTTP client reports a read error
// but the PVE side is actually fine. Callers proceed to the membership
// watcher on /cluster/status rather than failing here.
func looksLikeProxyRestart(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	for _, needle := range []string{
		"EOF",
		"connection reset",
		"broken pipe",
		"unexpected EOF",
	} {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return errors.Is(err, context.DeadlineExceeded)
}
