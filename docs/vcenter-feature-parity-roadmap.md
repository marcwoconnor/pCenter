# vCenter Feature Parity Roadmap

Comparison of VMware vCenter Server features vs pCenter current implementation, with prioritized roadmap for feature development.

**Last Updated:** 2026-04-17 (Phase 2 complete: all of items 6-10 shipped)

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
| VM-VM affinity rules | ✅ | Keep VMs together on same host |
| VM-VM anti-affinity rules | ✅ | Keep VMs apart on different hosts |
| VM-Host affinity rules | ✅ | Pin VMs to specific hosts |
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
| Ceph integration (read-only monitor) | ✅ | Health monitoring + whitelisted commands |
| Ceph day-2 management (OSD/MON/pool/flags CRUD) | ✅ | PR 1 of `docs/ceph-lifecycle-plan.md` — `/ceph` page tabs |
| Ceph CephFS + MDS + CRUSH viewer | ✅ | PR 2 — CephFS + MDS lifecycle, CRUSH viewer (editing intentionally out of scope) |
| Ceph install wizard | ✅ | PR 3 — `cephcluster` package; SSH `pveceph install` + REST init/MON/MGR |
| Ceph destroy / purge | ✅ | PR 4 — typed-confirmation destroy with continue-past-failure |
| SMART monitoring | ✅ | Disk health + temperature tracking |
| Storage profiles/policies | 🔴 | Policy-based VM storage placement |
| Storage DRS | 🔴 | Auto-balance storage across pools |
| Datastore clusters | 🔴 | Group storage pools |

---

## Certificates & ACME

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Per-node certificate info | ✅ | Tab on node detail: subject, issuer, SAN, expiry, fingerprint |
| ACME cert renewal trigger | ✅ | Per-node Renew button |
| Cert expiry warning | ✅ | Home banner <30d warning, <7d critical |
| ACME account CRUD | ✅ | Register/update/delete with directory picker + ToS accept |
| DNS plugin CRUD | ✅ | Schema-driven form renders any provider from `/cluster/acme/challenge-schema` |
| Node ACME domain editor | ✅ | Per-node domain list with plugin assignment |
| Custom certificate upload | 🔴 | Non-ACME cert+key upload (Phase 2b) |
| Bulk cert renewal | 🔴 | Renew across all nodes in a cluster (Phase 2b) |
| Alarm integration for expiry | 🔴 | Hook into alarms subsystem (Phase 2b) |
| Scheduled auto-renewal | 🔴 | Reuse scheduler subsystem (Phase 3) |
| Cert chain/PEM viewer | 🔴 | Decoded leaf + chain display (Phase 3) |

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
| Alarms (threshold alerts) | ✅ | Alarm definitions with thresholds |
| Alarm actions | ✅ | Notification channels (email/webhook/Slack) |
| Alarm acknowledgment | ✅ | Track alarm handling workflow |
| Custom alarm conditions | ✅ | User-defined thresholds per metric |
| Active alerts dashboard | ✅ | Triggered alarm overview with badge |

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
| Custom roles | ✅ | Built-in + custom roles with granular permissions |
| Object-level permissions | ✅ | Per-VM/folder/cluster/datacenter scoping |
| Permission inheritance | ✅ | Propagate through hierarchy (VM→Node→Cluster→DC→Root) |
| LDAP/AD integration | 🔴 | Directory service authentication |
| SSO | 🔴 | Single sign-on |

---

## Tags & Organization

| vCenter Feature | pCenter | Notes |
|-----------------|---------|-------|
| Folder trees | ✅ | Host tree + VM tree |
| Drag-and-drop organization | ✅ | Move resources between folders |
| Resource assignment to folders | ✅ | Add/remove folder members |
| VM tags | ✅ | Free-form tagging on any object |
| Tag categories | ✅ | Organize tags by type |
| Tag-based search | ✅ | Find resources by tag |
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
| Agent system | ✅ | Push-based pve-agent with 25 commands, agent-first routing |
| OpenAPI spec | 🔴 | Published API documentation |
| CLI tool | 🔴 | PowerCLI-equivalent scripting |
| Webhooks | 🔴 | Event notifications to external systems |
| Client SDK | 🔴 | Go/Python/JS client libraries |

---

## TODO: Prioritized Implementation Roadmap

### Phase 1 — High Value, Close the Biggest Gaps

| # | Feature | Effort | Value | Status |
|---|---------|--------|-------|--------|
| 1 | **RBAC (roles + object permissions)** | High | Critical | ✅ Done |
| 2 | **Alerting & notifications** | Medium | High | ✅ Done |
| 3 | **Tags & custom attributes** | Low | High | ✅ Done |
| 4 | **Affinity/anti-affinity rules** | Medium | High | ✅ Done |
| 5 | **Scheduled tasks** | Medium | High | ✅ Done |

#### 1. RBAC ✅
- [x] `roles` table with permissions JSON array
- [x] `role_assignments` table: user_id, role_id, object_type, object_id, propagate
- [x] Built-in roles: Admin, Operator, VM-Admin, Read-Only
- [x] Custom role creation UI (Settings → Roles & Permissions)
- [x] Permission check middleware with hierarchy traversal
- [x] Permission inheritance through object hierarchy (VM→Node→Cluster→DC→Root)
- [x] Role assignment management in Settings UI

#### 2. Alerting & Notifications ✅
- [x] `alarm_definitions` table with metric, condition, threshold, duration
- [x] `alarm_instances` table with per-resource state tracking
- [x] Alarm evaluator goroutine (configurable interval)
- [x] States: Normal → Warning → Critical with hysteresis
- [x] Notification channels: email, webhook, Slack
- [x] Active alarms badge in nav + dashboard panel
- [x] Alarm acknowledgment workflow
- [x] Alarm management UI in Settings

#### 3. Tags & Custom Attributes ✅
- [x] `tags` table: id, category, name, color
- [x] `tag_assignments` table: tag_id, object_type, object_id
- [x] Tag CRUD API endpoints + bulk assign
- [x] Tag categories (user-defined)
- [x] Tag picker component on VM/CT summary panels
- [x] Tag dots on compact VM/CT grid tiles
- [x] Bulk tag operations

#### 4. Affinity/Anti-Affinity Rules ✅
- [x] `drs_rules` table: type (affinity/anti-affinity/host-pin), members
- [x] Rule CRUD API endpoints
- [x] DRS engine respects rules when generating recommendations
- [x] Rule conflict/violation detection
- [x] Rules UI in cluster settings
- [x] Rule violations shown in DRS panel

#### 5. Scheduled Tasks ✅
- [x] `scheduled_tasks` table: type, target, cron_expression, params, enabled
- [x] `task_history` table: execution log
- [x] Go cron scheduler goroutine
- [x] Supported task types: power ops, snapshots, migrations
- [x] Schedule builder UI (cron expression helper)
- [x] Task execution history view
- [x] Enable/disable/delete schedules

---

### Phase 2 — Power User Features

| # | Feature | Effort | Value | Status |
|---|---------|--------|-------|--------|
| 6 | **Template library** | Medium | Medium | ✅ Done (via Content Library + convert-to-template) |
| 7 | **Backup management** | High | Medium | ✅ Done (MVP: context-menu + schedule; restore deferred) |
| 8 | **Resource pools** | Medium | Medium | ✅ Done (cluster-level CRUD + members view) |
| 9 | **Scheduled snapshots** | Low | Medium | ✅ Done (`snapshot_rotate` task type with retention) |
| 10 | **Power schedules** | Low | Medium | ✅ Done (existing `power_on`/`power_off` scheduler tasks) |

#### 6. Template Library
- [x] Convert VM/CT to template (Proxmox API `POST .../template`)
- [x] Template listing with metadata (OS, description, size)
- [x] Clone from template with customization (name, resources)
- [x] Template categories and search
- [x] Cross-cluster template visibility

#### 7. Backup Management
- [ ] Proxmox Backup Server connection config (tracked as #71)
- [x] Backup job scheduling (vzdump wrapper)
- [x] Backup retention policies
- [x] Backup listing and browsing
- [ ] Restore from backup UI (deferred — separate UX concern, target VMID + overwrite semantics)
- [x] Backup job status and history

#### 8. Resource Pools
- [x] List Proxmox pools (`GET /pools`)
- [x] Create/edit/delete pools
- [x] Assign VMs/CTs to pools
- [x] Pool resource limits display
- [x] Pool hierarchy (nested pools)

#### 9-10. Scheduled Snapshots & Power Schedules
- [x] Snapshot schedule: target, frequency, retention count
- [x] Auto-delete old snapshots beyond retention
- [x] Power schedule: start/stop times per day of week
- [ ] Holiday/exception handling (deferred — out of scope; cron-driven schedules cover the common case)

---

### Phase 3 — Enterprise Features

| # | Feature | Effort | Value | Dependencies |
|---|---------|--------|-------|--------------|
| 11 | **LDAP/AD integration** | High | High | Extends auth (tracked as #29) |
| 12 | **Storage vMotion** | High | Medium | ✅ Done (per-disk + per-volume; #28) |
| 13 | **Content library** | Medium | Medium | ✅ Done (Phase 2 #6) |
| 14 | **Webhooks** | Medium | Medium | ✅ Done (HMAC, retry, wildcards, auto-disable) |
| 15 | **OpenAPI spec** | Low | Medium | ✅ Done (hand-authored, drift-test guarded; #26 + #35) |
| 16 | **Cross-cluster migration** | High | Medium | Shared storage required (tracked as #30) |
| — | **Ceph full lifecycle** | High | High | ✅ Done (install/day-2/CephFS/destroy via `/ceph` page) |
| — | **PVE cluster formation** | Medium | High | ✅ Done (Create + Add-Member wizards; see `docs/PVE_CLUSTER_FORMATION.md`) |

#### 11. LDAP/AD Integration
- [ ] LDAP connection config (server, base DN, bind user)
- [ ] LDAP authentication backend (go-ldap library)
- [ ] Group-to-role mapping
- [ ] User sync (periodic or on-login)
- [ ] Mixed auth (local + LDAP users)

#### 12. Storage vMotion
- [x] API for online disk migration between storage pools (`POST /clusters/{c}/vms/{id}/disk/move`)
- [x] LXC volume move (`POST /clusters/{c}/containers/{id}/volume/move` — offline; PVE limitation)
- [x] Progress tracking via UPID (`GET /api/disk-moves`)
- [x] Frontend "Move Storage…" dialog with disk picker, target storage filter, format conversion
- [ ] Storage load balancing recommendations (deferred — DRS-style automation, separate scope)

#### 13. Content Library
- [x] Centralized ISO/template/OVA repository
- [x] Sync templates across clusters
- [x] Version tracking

#### 14. Webhooks
- [x] `webhook_endpoints` table: URL, secret, event filters
- [x] Fire webhooks from activity log events
- [x] Retry with backoff on failure (5s / 30s / 2min)
- [x] Webhook test/ping endpoint
- [x] Per-component wildcard event filters (`vm.*`, `*.migrate`, `*.*`)
- [x] Auto-disable after consecutive failures (10-strike threshold)
- [x] HMAC-SHA256 signed deliveries (Stripe-style `t=<unix>,v1=<hex>`)

#### 15. OpenAPI Spec
- [x] Hand-authored OpenAPI 3.0 spec (embedded at compile time)
- [x] Swagger UI endpoint (`/api/docs`) — assets vendored for air-gap deploys
- [x] Keep spec in sync with code (CI test: `TestOpenAPINoDrift` against an explicit allowlist)
- [ ] Expand spec to remaining route groups (tracked as #32 — currently 18/192 documented; allowlist tracks the rest)

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
