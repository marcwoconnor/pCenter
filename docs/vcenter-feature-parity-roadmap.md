# vCenter Feature Parity Roadmap

Comparison of VMware vCenter Server features vs pCenter current implementation, with prioritized roadmap for feature development.

**Last Updated:** 2026-01-18

---

## Feature Comparison Matrix

### Legend
- ✅ Implemented
- 🟡 Partial
- 🔴 Missing
- 🚧 In Progress
- N/A Not Applicable (platform-specific)

---

## Cluster & Infrastructure Management

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Multi-cluster management | ✅ | Full support |
| Cluster discovery | ✅ | Auto-discovers nodes |
| Datacenter hierarchy | ✅ | Inventory datacenters |
| Folder organization | ✅ | Custom folder structure |
| Host profiles | 🔴 | Config templates for hosts |
| Update Manager | 🔴 | Patch/update hosts |

---

## VM & Container Operations

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Create/Delete VMs | ✅ | Full CRUD |
| Start/Stop/Restart | ✅ | All power states |
| Suspend/Resume | ✅ | Pause VMs |
| VM Configuration | ✅ | CPU/RAM/disk/network |
| Console (VNC) | ✅ | noVNC integration |
| Container terminal | ✅ | LXC shell access |
| Guest agent integration | ✅ | qemu-guest-agent |

---

## Live Migration

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| vMotion (live migration) | ✅ | With progress tracking |
| Storage vMotion | 🔴 | Move disks between storage pools |
| Cross-cluster migration | 🔴 | Migrate VMs between clusters |
| Migration scheduling | 🔴 | Schedule migrations for maintenance windows |

---

## DRS (Distributed Resource Scheduler)

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Load analysis | ✅ | CPU/memory imbalance detection |
| Migration recommendations | ✅ | Manual approval mode |
| Manual DRS mode | ✅ | Recommendations only |
| Partially automated DRS | 🔴 | Auto-place new VMs |
| Fully automated DRS | 🔴 | Auto-execute all migrations |
| VM-VM affinity rules | 🔴 | Keep VMs together on same host |
| VM-VM anti-affinity rules | 🔴 | Keep VMs apart on different hosts |
| VM-Host affinity rules | 🔴 | Pin VMs to specific hosts |
| DPM (Distributed Power Mgmt) | 🔴 | Power off idle hosts to save energy |
| DRS groups | 🔴 | Group VMs/hosts for rules |

---

## High Availability

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| HA enable/disable | ✅ | Per-VM control |
| HA status display | ✅ | Visual badges |
| HA groups | 🟡 | Basic support (Proxmox 8+) |
| Admission control | 🔴 | Reserve capacity for failover |
| Host isolation response | 🔴 | Action when host isolated |
| VM restart priority | 🔴 | Order of VM restarts |
| VM monitoring | 🔴 | Restart unresponsive VMs |
| Proactive HA | 🔴 | Pre-emptive migration on hardware alerts |

---

## Fault Tolerance

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Continuous availability | 🔴 | Shadow VM replication (VMware-specific) |
| Zero downtime failover | 🔴 | FT is VMware-proprietary |

> **Note:** Proxmox doesn't have an equivalent to VMware FT. Consider HA with fast restart as alternative.

---

## Templates & Cloning

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Convert VM to template | 🔴 | Mark VM as template |
| Clone from template | 🟡 | Basic clone support exists |
| Full clone | 🟡 | Creates independent copy |
| Linked clone | 🔴 | Space-efficient clone sharing base disk |
| Template library | 🔴 | Centralized template management |
| Content library | 🔴 | Share templates across clusters |
| OVF/OVA import | 🔴 | Import virtual appliances |
| OVF/OVA export | 🔴 | Export VMs as appliances |

---

## Snapshots

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Create snapshot | 🔴 | Point-in-time VM state |
| Snapshot with memory | 🔴 | Include RAM state |
| Snapshot without memory | 🔴 | Disk-only snapshot |
| Snapshot tree/hierarchy | 🔴 | Multiple snapshot branches |
| Revert to snapshot | 🔴 | Rollback to previous state |
| Delete snapshot | 🔴 | Consolidate disk changes |
| Snapshot manager UI | 🔴 | Visual snapshot management |
| Scheduled snapshots | 🔴 | Automatic periodic snapshots |

---

## Resource Management

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Resource pools | 🔴 | Group and allocate resources |
| CPU reservations | 🔴 | Guarantee minimum CPU |
| Memory reservations | 🔴 | Guarantee minimum RAM |
| CPU limits | 🔴 | Cap maximum CPU |
| Memory limits | 🔴 | Cap maximum RAM |
| Shares (priority) | 🔴 | Relative resource priority |
| Expandable reservations | 🔴 | Borrow from parent pool |
| Resource pool hierarchy | 🔴 | Nested resource pools |

---

## Storage Management

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Storage overview | ✅ | All pools listed |
| Storage content browse | ✅ | Volumes, ISOs, templates |
| Ceph integration | ✅ | Health monitoring, commands |
| SMART monitoring | ✅ | Disk health alerts |
| File upload (ISO/templates) | ✅ | Upload to storage |
| Storage profiles/policies | 🔴 | Policy-based VM placement |
| Storage DRS | 🔴 | Auto-balance storage (VMware-specific) |
| Datastore clusters | 🔴 | Group storage pools |

---

## Networking

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Network overview | ✅ | Interface listing |
| SDN zones | ✅ | Proxmox SDN support |
| VNets (VLANs/overlays) | ✅ | Virtual networks |
| Subnets | ✅ | Subnet definitions |
| Distributed switch | 🔴 | Centralized virtual switch |
| Port groups | 🔴 | Network policies per group |
| Traffic shaping | 🔴 | Bandwidth limits |
| Network I/O control | 🔴 | QoS for network traffic |
| Private VLANs | 🔴 | Micro-segmentation |

---

## Backup & Recovery

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Backup scheduling | 🔴 | Schedule VM backups |
| Backup jobs | 🔴 | Manage backup tasks |
| Backup retention | 🔴 | Retention policies |
| Restore from backup | 🔴 | Recovery operations |
| File-level restore | 🔴 | Restore individual files |
| Backup verification | 🔴 | Verify backup integrity |
| Backup to remote | 🔴 | Off-site backup targets |
| Replication | 🔴 | VM replication to DR site |

---

## Monitoring & Alerting

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Performance charts | ✅ | CPU/RAM/disk/network |
| Metrics retention | ✅ | Multi-resolution (raw→monthly) |
| Real-time metrics | ✅ | WebSocket streaming |
| Events/tasks log | ✅ | Activity tracking |
| SMART monitoring | ✅ | Disk health |
| Alarms | 🔴 | Threshold-based alerts |
| Alarm actions | 🔴 | Email/script on alarm |
| Alarm acknowledgment | 🔴 | Track alarm handling |
| Custom alarms | 🔴 | User-defined conditions |
| Triggered alarm list | 🔴 | Active alerts dashboard |

---

## Permissions & Access Control

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Local users | ✅ | Full support |
| Admin role | ✅ | Full privileges |
| 2FA/TOTP | ✅ | Two-factor auth |
| Session management | ✅ | View/revoke sessions |
| Trusted IPs | ✅ | Skip 2FA on trusted networks |
| Account lockout | ✅ | Brute-force protection |
| Custom roles | 🔴 | Define permission sets |
| Object-level permissions | 🔴 | Per-VM/folder permissions |
| Permission inheritance | 🔴 | Hierarchical permissions |
| LDAP/AD integration | 🔴 | Directory service auth |
| SSO | 🔴 | Single sign-on |
| Audit logging | 🟡 | Auth events logged, need full audit |

---

## Tags & Organization

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| VM tags | 🟡 | Basic tag display |
| Create/edit tags | 🔴 | Tag management |
| Tag categories | 🔴 | Organize tags by type |
| Tag-based search | 🔴 | Find VMs by tag |
| Tag-based policies | 🔴 | Automation based on tags |
| Custom attributes | 🔴 | User-defined VM metadata |

---

## Scheduling & Automation

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Scheduled tasks | 🔴 | Timed operations |
| Power schedules | 🔴 | Auto start/stop VMs |
| Scheduled snapshots | 🔴 | Periodic snapshots |
| Scheduled migrations | 🔴 | Maintenance window migrations |

---

## Maintenance Operations

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Maintenance mode | ✅ | With guest evacuation |
| Pre-flight checks | ✅ | Validate before maintenance |
| Evacuation progress | ✅ | Track migration progress |
| QDevice monitoring | ✅ | Quorum status |
| Rolling updates | 🔴 | Update hosts sequentially |
| Host remediation | 🔴 | Apply patches/updates |

---

## API & Integration

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| REST API | ✅ | Full HTTP/JSON API |
| WebSocket API | ✅ | Real-time updates |
| CORS support | ✅ | Configurable origins |
| API authentication | ✅ | Session-based |
| PowerCLI equivalent | 🔴 | CLI/scripting interface |
| SDK | 🔴 | Client libraries |
| Webhooks | 🔴 | Event notifications |

---

## Prioritized Roadmap

### Phase 1: Core Gaps (High Value, High Impact)

These features are expected by most users and represent significant gaps.

| Feature | Effort | Business Value | Technical Complexity |
|---------|--------|----------------|---------------------|
| **Snapshots** | Medium | High | Medium |
| **Alarms/Alerts** | Medium | High | Medium |
| **Backup scheduling** | High | High | High |
| **Full RBAC** | High | High | High |

#### 1.1 Snapshots
- Create/delete snapshots
- Snapshot with/without memory
- Revert to snapshot
- Snapshot tree visualization
- **API:** Proxmox already supports via `/nodes/{node}/qemu/{vmid}/snapshot`

#### 1.2 Alarms & Alerting
- Define threshold conditions (CPU > 90% for 5min)
- Alarm states: Normal, Warning, Critical
- Notification channels: Email, webhook, Slack
- Alarm dashboard showing active alerts
- Alarm acknowledgment workflow

#### 1.3 Backup Scheduling
- Schedule backup jobs (daily, weekly, custom cron)
- Retention policies (keep last N, keep for N days)
- Backup storage targets
- Restore UI
- **Note:** Proxmox Backup Server integration is ideal path

#### 1.4 Full RBAC
- Custom roles with granular permissions
- Object-level permission assignment
- Permission inheritance through folder hierarchy
- Role templates (VM Admin, Read-Only, Operator)

---

### Phase 2: Power User Features (Medium Value)

Features that improve efficiency for experienced users.

| Feature | Effort | Business Value | Technical Complexity |
|---------|--------|----------------|---------------------|
| **Affinity/Anti-affinity rules** | Medium | Medium | Medium |
| **Automated DRS** | Medium | Medium | Medium |
| **Resource pools** | Medium | Medium | Medium |
| **Template library** | Medium | Medium | Low |
| **Scheduled tasks** | Medium | Medium | Low |
| **Tag management** | Low | Medium | Low |

#### 2.1 DRS Affinity Rules
- VM-VM affinity (keep together)
- VM-VM anti-affinity (keep apart)
- VM-Host affinity (pin to hosts)
- Rule conflict detection
- DRS respects rules in recommendations

#### 2.2 Automated DRS
- Partially automated: auto-place new VMs
- Fully automated: execute migrations automatically
- Configurable thresholds
- Migration rate limiting

#### 2.3 Resource Pools
- Create/edit/delete pools
- Assign VMs to pools
- Set reservations, limits, shares
- Pool hierarchy

#### 2.4 Template Library
- Convert VM to template
- Clone from template
- Template metadata (description, version)
- Template categories

#### 2.5 Scheduled Tasks
- Schedule power operations
- Schedule migrations
- Schedule snapshots
- Cron-like scheduling UI

---

### Phase 3: Enterprise Features (Strategic Value)

Features for large-scale and enterprise deployments.

| Feature | Effort | Business Value | Technical Complexity |
|---------|--------|----------------|---------------------|
| **LDAP/AD integration** | High | High | Medium |
| **Storage vMotion** | High | Medium | High |
| **Cross-cluster migration** | High | Medium | High |
| **Distributed switch** | High | Low | High |
| **Content library** | Medium | Medium | Medium |
| **Webhooks/Events** | Medium | Medium | Low |

#### 3.1 LDAP/AD Integration
- Connect to Active Directory or LDAP
- Group-based role assignment
- SSO support
- Sync users/groups

#### 3.2 Storage vMotion
- Move VM disks between storage pools
- Online disk migration
- Progress tracking
- Storage load balancing

#### 3.3 Cross-Cluster Migration
- Migrate VMs between Proxmox clusters
- Requires shared or replicated storage
- Network considerations

---

### Phase 4: Nice-to-Have (Lower Priority)

| Feature | Notes |
|---------|-------|
| Distributed Power Management | Power off idle hosts |
| Network I/O Control | QoS for VM traffic |
| OVF/OVA import/export | Virtual appliance portability |
| PowerCLI equivalent | CLI scripting |
| Proactive HA | Pre-emptive migration on hardware alerts |

---

## Implementation Notes

### Proxmox API Endpoints for Key Features

**Snapshots:**
```
POST /nodes/{node}/qemu/{vmid}/snapshot
GET  /nodes/{node}/qemu/{vmid}/snapshot
POST /nodes/{node}/qemu/{vmid}/snapshot/{snapname}/rollback
DELETE /nodes/{node}/qemu/{vmid}/snapshot/{snapname}
```

**Backup:**
```
POST /nodes/{node}/vzdump
GET  /nodes/{node}/tasks/{upid}/status
GET  /storage/{storage}/content (type=backup)
```

**Resource Pools:**
```
GET  /pools
POST /pools
PUT  /pools/{poolid}
DELETE /pools/{poolid}
```

### Architecture Considerations

1. **Alarms:** Consider using a separate SQLite table for alarm definitions and state. Poll metrics and evaluate conditions on a configurable interval (e.g., every 30 seconds).

2. **RBAC:** Extend auth.db with `roles`, `permissions`, and `role_assignments` tables. Check permissions in API middleware.

3. **Scheduled Tasks:** Use a scheduler (e.g., cron-like library in Go) with persistence in SQLite. Store job definitions and execution history.

4. **LDAP:** Use Go LDAP library (go-ldap/ldap). Support both simple bind and SASL authentication.

---

## References

- [VMware DRS vs HA Explained](https://www.nakivo.com/blog/vmware-vsphere-ha-and-drs-compared-and-explained/)
- [vCenter Roles and Permissions](https://kb.expedient.com/docs/vmware-vcenter-roles-and-permissions)
- [VMware DRS Configuration](https://www.nakivo.com/blog/vmware-cluster-drs-configuration/)
- [Proxmox VE API Documentation](https://pve.proxmox.com/pve-docs/api-viewer/)
