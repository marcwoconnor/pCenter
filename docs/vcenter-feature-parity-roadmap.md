# vCenter Feature Parity Roadmap

Comparison of VMware vCenter Server features vs pCenter current implementation, with prioritized roadmap for feature development.

**Last Updated:** 2026-04-10

---

## Feature Comparison Matrix

### Legend
- ✅ Implemented
- 🟡 Partial
- 🔴 Missing
- N/A Not Applicable (platform-specific)

---

## Cluster & Infrastructure Management

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Multi-cluster management | ✅ | Full support |
| Cluster discovery | ✅ | Auto-discovers nodes from discovery node |
| Datacenter hierarchy | ✅ | Datacenters → Clusters → Hosts → Guests |
| Folder organization | ✅ | Separate host/VM trees, drag-and-drop |
| Host onboarding wizard | ✅ | Test → SSH → Deploy Agent → Activate |
| Maintenance mode | ✅ | Preflight checks + automated evacuation |
| QDevice monitoring | ✅ | Quorum device status tracking |
| Host profiles | 🔴 | Config templates for hosts |
| Update Manager | 🔴 | Patch/update hosts from UI |
| Rolling updates | 🔴 | Update hosts sequentially |

---

## VM & Container Operations

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Create/Delete VMs | ✅ | Full CRUD with config |
| Create/Delete Containers | ✅ | Full LXC lifecycle (vCenter has no equivalent) |
| Start/Stop/Restart/Shutdown | ✅ | All power states |
| VM Configuration editing | ✅ | CPU/RAM/disk/network with conflict detection |
| Console (VNC) | ✅ | noVNC integration |
| Container terminal | ✅ | LXC shell access |

---

## Snapshots

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Create snapshot | ✅ | Point-in-time VM/CT state |
| Snapshot with memory | ✅ | Include RAM state |
| Snapshot tree/hierarchy | ✅ | Parent/child visualization |
| Revert to snapshot | ✅ | Rollback to previous state |
| Delete snapshot | ✅ | Consolidate disk changes |
| Scheduled snapshots | 🔴 | Automatic periodic snapshots |

---

## Cloning & Templates

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Full clone | ✅ | Creates independent copy |
| Linked clone | ✅ | Space-efficient clone |
| Clone from template | ✅ | Clone existing VM/CT |
| Target node selection | ✅ | Choose destination during clone |
| Convert VM to template | 🔴 | Mark VM as template (first-class object) |
| Template library | 🔴 | Centralized template management |
| Content library | 🔴 | Share templates/ISOs across clusters |
| OVF/OVA import/export | 🔴 | Virtual appliance portability |

---

## Live Migration

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Live migration (vMotion) | ✅ | Online with progress tracking |
| Offline migration | ✅ | With guest shutdown |
| Migration monitoring | ✅ | Active migrations dashboard, UPID tracking |
| Migration targets API | ✅ | Available target node listing |
| Storage vMotion | 🔴 | Move disks between storage pools while running |
| Cross-cluster migration | 🔴 | Migrate VMs between Proxmox clusters |
| Migration scheduling | 🔴 | Schedule migrations for maintenance windows |

---

## DRS (Distributed Resource Scheduler)

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Load analysis | ✅ | CPU/memory std deviation analysis |
| Migration recommendations | ✅ | With apply/dismiss actions |
| Manual DRS mode | ✅ | Recommendations only |
| Semi-automatic DRS | ✅ | Auto-place new VMs |
| Fully automatic DRS | ✅ | Auto-execute migrations |
| Configurable thresholds | ✅ | CPU/memory load thresholds |
| VM-VM affinity rules | 🔴 | Keep VMs together on same host |
| VM-VM anti-affinity rules | 🔴 | Keep VMs apart on different hosts |
| VM-Host affinity rules | 🔴 | Pin VMs to specific hosts |
| DRS groups | 🔴 | Group VMs/hosts for rule targeting |
| DPM (Distributed Power Mgmt) | 🔴 | Power off idle hosts to save energy |

---

## High Availability

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| HA enable/disable | ✅ | Per-VM/CT with policies |
| HA status display | ✅ | Badges throughout UI |
| HA groups | ✅ | Group configuration |
| Recovery policies | ✅ | Max restart, max relocate |
| Admission control | 🔴 | Reserve capacity for failover |
| Host isolation response | 🔴 | Action when host isolated from network |
| VM restart priority | 🔴 | Order of VM restarts after failure |
| VM monitoring | 🔴 | Restart unresponsive VMs via heartbeat |
| Proactive HA | 🔴 | Pre-emptive migration on hardware sensor alerts |

---

## Fault Tolerance

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Continuous availability | N/A | VMware FT is proprietary — no Proxmox equivalent |
| Zero downtime failover | N/A | Use HA with fast restart as alternative |

---

## Storage Management

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Storage overview | ✅ | All pools listed with usage |
| Storage content browse | ✅ | ISOs, templates, backups, images |
| File upload | ✅ | Upload ISOs/templates to storage |
| Ceph integration | ✅ | Health monitoring + whitelisted commands |
| SMART monitoring | ✅ | Disk health + temperature tracking |
| Storage profiles/policies | 🔴 | Policy-based VM storage placement |
| Storage DRS | 🔴 | Auto-balance storage across pools |
| Datastore clusters | 🔴 | Group storage pools |

---

## Networking

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Network overview | ✅ | Per-node interface listing |
| Network topology | ✅ | Visual representation |
| SDN zones | ✅ | Proxmox SDN support |
| VNets | ✅ | Virtual networks |
| Subnets | ✅ | Subnet definitions |
| Guest NIC display | ✅ | Parsed from config endpoints |
| Distributed switch | 🔴 | Centralized virtual switch across hosts |
| Port groups | 🔴 | Network policies per port group |
| Traffic shaping | 🔴 | Bandwidth limits |
| Network I/O control | 🔴 | QoS for VM network traffic |

---

## Backup & Recovery

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Backup scheduling | 🔴 | Schedule VM backups (vzdump or PBS) |
| Backup jobs | 🔴 | Manage backup tasks |
| Backup retention | 🔴 | Retention policies |
| Restore from backup | 🔴 | Recovery operations |
| File-level restore | 🔴 | Restore individual files |
| Backup verification | 🔴 | Verify backup integrity |
| Replication | 🔴 | VM replication to DR site |

---

## Monitoring & Alerting

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Performance charts | ✅ | CPU/RAM/disk/network time-series |
| Metrics retention | ✅ | Raw→Hourly→Daily→Weekly→Monthly rollup |
| Real-time metrics | ✅ | 30s collection, WebSocket push |
| Activity/audit log | ✅ | Filterable by resource, action, cluster |
| SMART monitoring | ✅ | Disk health tracking |
| Alarms (threshold alerts) | 🔴 | CPU > 90% for 5min → notification |
| Alarm actions | 🔴 | Email/webhook/Slack on alarm |
| Alarm acknowledgment | 🔴 | Track alarm handling workflow |
| Custom alarm conditions | 🔴 | User-defined thresholds |
| Active alerts dashboard | 🔴 | Triggered alarm overview |

---

## Permissions & Access Control

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Local users | ✅ | Create/delete users |
| Admin role | ✅ | Full privileges |
| 2FA/TOTP | ✅ | With recovery codes |
| Session management | ✅ | View/revoke sessions |
| Trusted IPs | ✅ | Skip 2FA on known networks |
| Account lockout | ✅ | Progressive brute-force protection |
| Auth event logging | ✅ | Login/logout/failure audit trail |
| Rate limiting | ✅ | Login attempt throttling |
| Custom roles | 🔴 | Define named permission sets |
| Object-level permissions | 🔴 | Per-VM/folder/cluster permissions |
| Permission inheritance | 🔴 | Propagate through folder hierarchy |
| LDAP/AD integration | 🔴 | Directory service authentication |
| SSO | 🔴 | Single sign-on |

---

## Tags & Organization

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Folder trees | ✅ | Host tree + VM tree |
| Drag-and-drop organization | ✅ | Move resources between folders |
| Resource assignment to folders | ✅ | Add/remove folder members |
| VM tags | 🔴 | Free-form tagging on any object |
| Tag categories | 🔴 | Organize tags by type |
| Tag-based search | 🔴 | Find resources by tag |
| Custom attributes | 🔴 | User-defined metadata |

---

## Scheduling & Automation

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Scheduled tasks | 🔴 | Timed operations |
| Power schedules | 🔴 | Auto start/stop VMs on schedule |
| Scheduled snapshots | 🔴 | Periodic snapshots |
| Scheduled migrations | 🔴 | Maintenance window migrations |

---

## API & Integration

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| REST API | ✅ | 90+ endpoints |
| WebSocket API | ✅ | Real-time state push |
| CORS support | ✅ | Configurable origins |
| Session auth | ✅ | Cookie-based sessions |
| CSRF protection | ✅ | On state-changing operations |
| Agent system | ✅ | Push-based pve-agent architecture |
| OpenAPI spec | 🔴 | Published API documentation |
| CLI tool | 🔴 | PowerCLI-equivalent scripting |
| Webhooks | 🔴 | Event notifications to external systems |
| Client SDK | 🔴 | Go/Python/JS client libraries |

---

## TODO: Prioritized Implementation Roadmap

### Phase 1 — High Value, Close the Biggest Gaps

| # | Feature | Effort | Value | Dependencies |
|---|---------|--------|-------|--------------|
| 1 | **RBAC (roles + object permissions)** | High | Critical | Extends auth.db |
| 2 | **Alerting & notifications** | Medium | High | Uses existing metrics |
| 3 | **Tags & custom attributes** | Low | High | New SQLite table |
| 4 | **Affinity/anti-affinity rules** | Medium | High | Extends DRS engine |
| 5 | **Scheduled tasks** | Medium | High | New scheduler + SQLite |

#### 1. RBAC
- [ ] `roles` table: name, description, permissions bitmask
- [ ] `role_assignments` table: user_id, role_id, object_type, object_id
- [ ] Built-in roles: Admin, Operator, VM-Admin, Read-Only
- [ ] Custom role creation UI
- [ ] Permission check middleware (replace simple admin check)
- [ ] Permission inheritance through folder hierarchy
- [ ] UI: permission editor on object detail panels

#### 2. Alerting & Notifications
- [ ] `alarm_definitions` table: metric, condition, threshold, duration, severity
- [ ] `alarm_state` table: current state per alarm per resource
- [ ] Alarm evaluator goroutine (runs every 30s against metrics)
- [ ] States: Normal → Warning → Critical (with hysteresis)
- [ ] Notification channels: email, webhook, Slack
- [ ] Active alarms dashboard panel
- [ ] Alarm acknowledgment workflow
- [ ] Per-object alarm assignment (or global defaults)

#### 3. Tags & Custom Attributes
- [ ] `tags` table: id, category, name, color
- [ ] `tag_assignments` table: tag_id, object_type, object_id
- [ ] Tag CRUD API endpoints
- [ ] Tag categories (Environment, Owner, Purpose, etc.)
- [ ] Tag picker component in object detail panels
- [ ] Tag-based search/filtering in all list views
- [ ] Bulk tag operations

#### 4. Affinity/Anti-Affinity Rules
- [ ] `drs_rules` table: type (affinity/anti-affinity/host-pin), members
- [ ] `drs_groups` table: group VMs or hosts for rule targeting
- [ ] Rule CRUD API endpoints
- [ ] DRS engine respects rules when generating recommendations
- [ ] Rule conflict detection (affinity + anti-affinity on same VMs)
- [ ] Rules UI in cluster settings
- [ ] Rule violations shown in DRS panel

#### 5. Scheduled Tasks
- [ ] `scheduled_tasks` table: type, target, cron_expression, params, enabled
- [ ] `task_history` table: execution log
- [ ] Go cron scheduler goroutine
- [ ] Supported task types: power ops, snapshots, migrations
- [ ] Schedule builder UI (cron expression helper)
- [ ] Task execution history view
- [ ] Enable/disable/delete schedules

---

### Phase 2 — Power User Features

| # | Feature | Effort | Value | Dependencies |
|---|---------|--------|-------|--------------|
| 6 | **Template library** | Medium | Medium | New inventory concept |
| 7 | **Backup management** | High | Medium | PBS integration |
| 8 | **Resource pools** | Medium | Medium | Proxmox pools API |
| 9 | **Scheduled snapshots** | Low | Medium | Phase 1 scheduler |
| 10 | **Power schedules** | Low | Medium | Phase 1 scheduler |

#### 6. Template Library
- [ ] Convert VM/CT to template (Proxmox API `POST .../template`)
- [ ] Template listing with metadata (OS, description, size)
- [ ] Clone from template with customization (name, resources)
- [ ] Template categories and search
- [ ] Cross-cluster template visibility

#### 7. Backup Management
- [ ] Proxmox Backup Server connection config
- [ ] Backup job scheduling (vzdump wrapper)
- [ ] Backup retention policies
- [ ] Backup listing and browsing
- [ ] Restore from backup UI
- [ ] Backup job status and history

#### 8. Resource Pools
- [ ] List Proxmox pools (`GET /pools`)
- [ ] Create/edit/delete pools
- [ ] Assign VMs/CTs to pools
- [ ] Pool resource limits display
- [ ] Pool hierarchy (nested pools)

#### 9-10. Scheduled Snapshots & Power Schedules
- [ ] Snapshot schedule: target, frequency, retention count
- [ ] Auto-delete old snapshots beyond retention
- [ ] Power schedule: start/stop times per day of week
- [ ] Holiday/exception handling

---

### Phase 3 — Enterprise Features

| # | Feature | Effort | Value | Dependencies |
|---|---------|--------|-------|--------------|
| 11 | **LDAP/AD integration** | High | High | Extends auth |
| 12 | **Storage vMotion** | High | Medium | Proxmox API support |
| 13 | **Content library** | Medium | Medium | Phase 2 templates |
| 14 | **Webhooks** | Medium | Medium | Activity system |
| 15 | **OpenAPI spec** | Low | Medium | Documentation |
| 16 | **Cross-cluster migration** | High | Medium | Shared storage required |

#### 11. LDAP/AD Integration
- [ ] LDAP connection config (server, base DN, bind user)
- [ ] LDAP authentication backend (go-ldap library)
- [ ] Group-to-role mapping
- [ ] User sync (periodic or on-login)
- [ ] Mixed auth (local + LDAP users)

#### 12. Storage vMotion
- [ ] API for online disk migration between storage pools
- [ ] Progress tracking via UPID
- [ ] Storage load balancing recommendations

#### 13. Content Library
- [ ] Centralized ISO/template/OVA repository
- [ ] Sync templates across clusters
- [ ] Version tracking

#### 14. Webhooks
- [ ] `webhook_endpoints` table: URL, secret, event filters
- [ ] Fire webhooks from activity log events
- [ ] Retry with backoff on failure
- [ ] Webhook test/ping endpoint

#### 15. OpenAPI Spec
- [ ] Generate OpenAPI 3.0 spec from route definitions
- [ ] Swagger UI endpoint (`/api/docs`)
- [ ] Keep spec in sync with code

---

### Phase 4 — Nice-to-Have

| # | Feature | Effort | Notes |
|---|---------|--------|-------|
| 17 | CLI tool (pcenterctl) | Medium | Scripting interface |
| 18 | Distributed Power Mgmt | Medium | Power off idle hosts |
| 19 | Host profiles | High | Config templates |
| 20 | Proactive HA | High | Hardware sensor integration |
| 21 | Network I/O control | Medium | QoS for VM traffic |
| 22 | OVF/OVA import/export | Medium | Appliance portability |
| 23 | Distributed switch | High | Centralized network config |

---

## Proxmox API Endpoints for Key Features

**Resource Pools:**
```
GET    /pools
POST   /pools
PUT    /pools/{poolid}
DELETE /pools/{poolid}
```

**Backup (vzdump):**
```
POST /nodes/{node}/vzdump
GET  /nodes/{node}/tasks/{upid}/status
GET  /storage/{storage}/content?content=backup
```

**Templates:**
```
POST /nodes/{node}/qemu/{vmid}/template    # Convert to template
POST /nodes/{node}/lxc/{vmid}/template
```

## Architecture Notes

1. **RBAC:** Extend auth.db with `roles`, `permissions`, `role_assignments`. Check in API middleware. Propagate through folder hierarchy.

2. **Alerting:** Separate SQLite table. Evaluator goroutine polls metrics every 30s. Use hysteresis to avoid flapping (e.g., trigger at 90%, clear at 80%).

3. **Scheduled Tasks:** Use Go cron library (robfig/cron). Persist definitions in SQLite. Log execution history.

4. **LDAP:** go-ldap/ldap library. Support simple bind + search. Cache group memberships.

5. **Webhooks:** Fire async from activity logger. Use exponential backoff on failure. HMAC signature for verification.
