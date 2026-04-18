# Nested-PVE Test Harness

> **Status:** Phase 1 (infrastructure) complete. Phases 2 (smoke suite) and 3 (CI integration) pending. Tracked as [#48](https://github.com/marcwoconnor/pCenter/issues/48).

A throwaway 2-node Proxmox VE cluster that exists to answer one question per release:

> *"Can a fresh pCenter install manage a real Proxmox cluster without regressing?"*

## Why nested Proxmox vs a mock

Unit tests already run on every push. What they miss is behavioural: how pCenter handles live PVE task UPIDs, cluster quorum transitions, real cert renewal flows, VM migration side-effects. A hand-written PVE mock would cover the HTTP shape but none of the dynamics, and maintaining the mock as PVE evolves is a second codebase. Running real PVE in nested KVM VMs gets us real dynamics for the cost of ~8 GB RAM idle on a host we already own.

## Architecture

```
                                     internet
                                        │
                                    pfSense firewall
                                  (10.31.10.1 mgmt,
                                   10.31.12.1 VLAN502 gw)
                                        │
                              Ruckus ICX 6610-48P switch
                              (VLAN 502 trunked to pve05 port 1/3/2)
                                        │
                         ┌──── pve05 (prod hypervisor) ────┐
                         │                                  │
                         │   Bridge: vmbr1 (VLAN-aware)     │
                         │   ├── VM 109 "pve-test-1"        │
                         │   │   (4c/4G/16G, cpu=host)      │
                         │   │   10.31.12.102 (DHCP)        │
                         │   └── VM 110 "pve-test-2"        │
                         │       (4c/4G/16G, cpu=host)      │
                         │       10.31.12.104 (DHCP)        │
                         └──────────────────────────────────┘
                                     │
                                 test-cluster
                                (quorate, 2 votes)
```

### Why each piece exists

| Piece | Purpose |
|---|---|
| VLAN 502 | Isolates test traffic from production VLANs; NAT via pfSense gives internet for webhook/ACME tests; no DHCP conflict with other VLANs. |
| pfSense opt2 rules | Two pass rules — to Proxmox prod `10.31.10.0/24` and to `any`. Matches permissive posture of existing VLANs; harness can reach prod for comparison tests but can also be dropped later for stricter lateral isolation. |
| Switch VLAN 502 trunk | Added to `1/3/1` (pve04) and `1/3/2` (pve05); lets the nested VMs' tagged traffic reach pfSense. |
| `cpu=host` on the VMs | Exposes `vmx` (Intel VT-x) to the guest kernel — required for the nested PVE to actually run KVM guests of its own inside tests. |
| `boot=order=scsi0;ide2` | HDD first, CD fallback. An empty HDD at first boot falls through to the installer on CD; once installed, HDD always wins and the installer never re-runs. See "install loop trap" below. |
| Golden snapshot | LVM-thin copy-on-write snapshot captured after cluster bootstrap + clean shutdown. Rollback takes seconds. This is what each test run resets to. |

## What's provisioned (Phase 1 complete, 2026-04-18)

- PVE 9.1.1 running on both nested nodes, clustered as `test-cluster`.
- Admin API token: `pcenter@pve!harness` with role `Administrator` — its secret is stored in the operator's private notes, not this repo.
- Both VMs have a snapshot named `golden` captured from a clean powered-off state.
- pfSense DHCP on opt2 issuing leases in `10.31.12.100–200`.
- Firewall rules on opt2 for Proxmox-prod access + internet egress.
- Switch ports `1/3/1` and `1/3/2` trunking VLAN 502.
- Customized auto-install ISO (`proxmox-ve_9.1-1-auto.iso`) on pve05's `local` storage with an answer file baked in.

## Using the harness

### Restore to clean state
From pve05:

```bash
qm rollback 109 golden && qm rollback 110 golden
qm start 109 && qm start 110
# Wait ~60s for corosync reconvergence + pveproxy cert reload, then:
curl -sk -H 'Authorization: PVEAPIToken=pcenter@pve!harness=<SECRET>' \
  https://10.31.12.102:8006/api2/json/cluster/status
# Poll until data[0].quorate is true.
```

The `<SECRET>` is in the operator's private memory file `reference_test_harness.md` — never commit it.

### Connect pCenter to the test cluster
In a deployed pCenter's `config.yaml`:

```yaml
clusters:
  - name: test
    discovery_node: 10.31.12.102:8006
    token_id: pcenter@pve!harness
    token_secret: ${PVE_TEST_TOKEN_SECRET}
    insecure: true
```

The `insecure: true` is fine here — the nested nodes' certs are self-signed on the unique `test-cluster` name, and we're crossing pfSense-NAT, so no real trust chain would apply anyway.

### Spin a guest inside the nested cluster
Same API the real pCenter uses — useful for harness smoke tests:

```bash
curl -sk -H 'Authorization: PVEAPIToken=pcenter@pve!harness=<SECRET>' \
  -X POST "https://10.31.12.102:8006/api2/json/nodes/pve-test-1/lxc" \
  -d 'vmid=200' -d 'ostemplate=local:vztmpl/ubuntu-24.04-standard_24.04-2_amd64.tar.zst' \
  -d 'storage=local-lvm' -d 'hostname=smoke-1' ...
```

## Rebuilding from zero

See [`reference_test_harness.md`](../../../.claude/projects/-home-moconnor-projects-pCenter/memory/reference_test_harness.md) (operator private) for the step-by-step with exact commands and credentials. The short version:

1. Prereqs that stay provisioned: VLAN 502 firewall rules + DHCP + switch trunk + the custom auto-install ISO on pve05.
2. Provision two VMs on pve05 (`cpu=host`, `boot=order=scsi0;ide2`, net on `vmbr1 tag=502`) booted off the custom ISO.
3. Wait ~7 min for the unattended installer.
4. `hostnamectl set-hostname` each, fix `/etc/hosts`, clean stale `/etc/pve/nodes/pve-test` orphans, reboot.
5. Exchange root SSH keys both directions, then `pvecm create` on node 1 and `pvecm add` on node 2.
6. Create `pcenter@pve` user + `harness` API token.
7. Shutdown, `qm snapshot <id> golden`, start.

## Sharp edges learned during Phase 1

- **Install loop trap** (hours of pain). Default Proxmox VM config is `boot=order=ide2;scsi0`. The PVE auto-installer ISO is always bootable, so after install the VM reboots and re-runs the installer forever. Fix: `boot=order=scsi0;ide2` from the start — empty HDD falls through to CD for the first install, after that HDD always wins.
- **Dual `/etc/pve/nodes/` dirs after hostname rename**. PVE's pmxcfs creates `/etc/pve/nodes/<hostname>/` on first install. When you `hostnamectl set-hostname` later and reboot, pmxcfs creates the new-name dir alongside the orphan. Must `rm -rf /etc/pve/nodes/<old-name>` before clustering or `pvecm create` will have conflicting node entries.
- **`pvecm add --use_ssh` needs passwordless SSH both directions**. `sshpass` doesn't intercept pvecm's internal ssh/scp forks. Exchange root SSH pubkeys manually first (generate with `ssh-keygen -t rsa` if missing).
- **PVE 8.4+ uses kebab-case answer-file keys**: `root-password`, `root-ssh-keys`, `disk-list`. Underscore keys still parse but emit deprecation warnings and the ISO builder marks the answer file as "Found issues".

## What's next (Phases 2 + 3)

### Phase 2 — smoke suite (`effort-m`)
A single script `packaging/test-release.sh <version>` that:
1. Rolls back both VMs to `golden` and starts them.
2. Provisions a fresh pCenter LXC (see [`release_deploy_recipe.md`](../../../.claude/projects/-home-moconnor-projects-pCenter/memory/release_deploy_recipe.md)) pointed at the nested cluster.
3. Runs an API-driven smoke suite: login flow, cluster discovery, VM create/migrate/delete, webhook signature verification, alarm trigger.
4. Posts pass/fail to stdout + exits non-zero on fail.

### Phase 3 — CI integration (`effort-s`)
A GitHub Actions release workflow step that SSHs into a self-hosted runner with PVE API access, invokes `test-release.sh <tag>`, and comments the result on the GitHub release. Strict gate posture: **post result, do not block** during the initial rollout period.

## References

- Issue [#48](https://github.com/marcwoconnor/pCenter/issues/48) — tracking, phased scope, and open questions.
- Issue [#42](https://github.com/marcwoconnor/pCenter/issues/42) — frontend lint debt (unrelated but adjacent cleanup).
- `docs/vcenter-feature-parity-roadmap.md` — broader roadmap this harness supports.
- Operator-private memory files (not in repo) for secrets + step-by-step with credentials:
  - `reference_test_harness.md` — this harness specifically
  - `reference_lab_switch.md` — Ruckus ICX access for port config changes
  - `release_deploy_recipe.md` — the LXC-provisioning recipe for the pCenter-under-test
