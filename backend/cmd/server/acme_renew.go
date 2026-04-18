package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/moconnor/pcenter/internal/poller"
)

// clusterACMERenew triggers ACME renewal on every online node in the cluster.
// Returns a composite UPID-like string summarizing per-node results so the
// scheduler history shows what happened. Partial failures do not abort.
func clusterACMERenew(ctx context.Context, p *poller.Poller, cluster string) (string, error) {
	if p == nil {
		return "", fmt.Errorf("poller not available (agent-only mode); ACME renew not supported yet")
	}
	clients := p.GetClusterClients(cluster)
	if len(clients) == 0 {
		return "", fmt.Errorf("no clients for cluster %q", cluster)
	}

	type result struct {
		node string
		upid string
		err  error
	}

	var wg sync.WaitGroup
	ch := make(chan result, len(clients))
	for nodeName, client := range clients {
		wg.Add(1)
		go func(n string, c interface {
			RenewNodeACMECertificate(ctx context.Context) (string, error)
		}) {
			defer wg.Done()
			upid, err := c.RenewNodeACMECertificate(ctx)
			ch <- result{node: n, upid: upid, err: err}
		}(nodeName, client)
	}
	wg.Wait()
	close(ch)

	var okNodes, failNodes []string
	var firstUPID string
	for r := range ch {
		if r.err != nil {
			failNodes = append(failNodes, r.node+":"+r.err.Error())
			slog.Warn("scheduled ACME renew: node failed", "cluster", cluster, "node", r.node, "error", r.err)
		} else {
			okNodes = append(okNodes, r.node)
			if firstUPID == "" {
				firstUPID = r.upid
			}
			slog.Info("scheduled ACME renew: node ok", "cluster", cluster, "node", r.node, "upid", r.upid)
		}
	}

	summary := fmt.Sprintf("ok=[%s]", strings.Join(okNodes, ","))
	if len(failNodes) > 0 {
		summary += fmt.Sprintf(" failed=[%s]", strings.Join(failNodes, "; "))
		// Report as error if every node failed; otherwise treat as partial success.
		if len(okNodes) == 0 {
			return "", fmt.Errorf("%s", summary)
		}
	}
	// Return the first successful UPID so task history has a clickable reference;
	// append summary so full status is preserved even though scheduler only stores one UPID.
	if firstUPID != "" {
		return firstUPID + " (" + summary + ")", nil
	}
	return summary, nil
}
