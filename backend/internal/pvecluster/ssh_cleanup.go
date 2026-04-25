package pvecluster

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// cleanupCorosyncOverSSH reverts a Proxmox node back to standalone state by
// removing leftover corosync config from a previous (failed) cluster-create
// attempt. Runs the canonical PVE recovery sequence over SSH using root@pam
// password auth — the same password the user already entered for the join
// flow, so no extra prompt.
//
// addrPort is the host's PVE address (e.g. "10.0.0.11:8006"); we use only
// the host part and connect on port 22.
//
// Returns the script output (always — even on error, useful for debugging).
func cleanupCorosyncOverSSH(ctx context.Context, addrPort, password string) (string, error) {
	host := hostPart(addrPort)
	if host == "" {
		return "", fmt.Errorf("could not parse host from %q", addrPort)
	}
	sshAddr := net.JoinHostPort(host, "22")

	cfg := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{ssh.Password(password)},
		// First-touch trust: cluster formation already requires the user
		// trust this node's password. Persisting host keys is a separate
		// concern handled by the host-add SSH-setup flow.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := dialSSHWithContext(ctx, sshAddr, cfg)
	if err != nil {
		return "", fmt.Errorf("ssh connect: %w", err)
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("ssh session: %w", err)
	}
	defer sess.Close()

	// Canonical Proxmox standalone-revert sequence. `pmxcfs -l` mounts
	// /etc/pve locally so we can delete corosync.conf even when the
	// cluster service won't start. The trailing `systemctl start
	// pve-cluster` brings pmxcfs back up in normal (cluster-aware) mode.
	script := `set -x
systemctl stop pve-cluster 2>/dev/null || true
systemctl stop corosync 2>/dev/null || true
pmxcfs -l
rm -f /etc/pve/corosync.conf /etc/corosync/corosync.conf
killall pmxcfs 2>/dev/null || true
sleep 1
systemctl start pve-cluster
sleep 2
echo CLEANUP_DONE`

	out, err := sess.CombinedOutput(script)
	output := string(out)
	if err != nil {
		return output, fmt.Errorf("cleanup script failed: %w", err)
	}
	if !strings.Contains(output, "CLEANUP_DONE") {
		return output, fmt.Errorf("cleanup script did not complete (no DONE marker)")
	}
	return output, nil
}

// dialSSHWithContext mirrors the helper in internal/api: a ctx-aware SSH dial
// (the x/crypto/ssh package's Dial doesn't take a context, so we build it from
// net.Dialer + ssh.NewClientConn so a cancelled ctx aborts a hung handshake).
func dialSSHWithContext(ctx context.Context, addr string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		conn.Close()
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	return ssh.NewClient(c, chans, reqs), nil
}
