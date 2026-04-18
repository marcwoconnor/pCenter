#!/usr/bin/env bash
# packaging/test-release.sh [version]
#
# Phase 2 of #48 — release smoke test against the nested-PVE harness.
#
# Flow:
#   1. Reset nested cluster to golden snapshot via PVE API on pve05.
#   2. Wait for corosync quorum on the nested nodes.
#   3. Provision a fresh Ubuntu LXC on pve05 (VLAN 502), install pCenter
#      from the public APT repo — matches README Quick Install path.
#   4. Run API smoke: register admin, add cluster via inventory API,
#      wait for poller, LXC lifecycle, webhook w/ HMAC verification, alarms.
#   5. Teardown the pCenter LXC (nested VMs left running for inspection).
#
# Prereqs (fail-fast if missing):
#   - Source proxmox-admin/.env first (PROXMOX_USERNAME, PROXMOX_PASSWORD, PVE05_HOST)
#   - export PVE_TEST_TOKEN_SECRET='<pcenter@pve!harness secret>'
#   - Local jq + curl + openssl + ssh
#   - An LXC template (alpine/ubuntu) pre-staged on the nested cluster's
#     local storage and baked into the golden snapshot.
#     See docs/test-harness.md "Rebuilding from zero".
#
# Usage:
#   source /home/moconnor/projects/proxmox-admin/.env
#   export PVE_TEST_TOKEN_SECRET='...'
#   ./packaging/test-release.sh v0.1.11
#
# Exit codes:
#   0 = all smoke tests passed
#   1 = smoke failure
#   2 = prereq/config error

set -euo pipefail

# ===== Args / version =====
VERSION="${1:-$(git -C "$(dirname "$(readlink -f "$0")")/.." describe --tags --always 2>/dev/null || echo dev)}"

# ===== Config (override via env) =====
PVE_HOST="${PVE05_HOST:?PVE05_HOST not set — source proxmox-admin/.env}"
PVE_NODE="${PVE_NODE:-pve05}"

NESTED_NODE1_VMID="${NESTED_NODE1_VMID:-109}"
NESTED_NODE2_VMID="${NESTED_NODE2_VMID:-110}"
NESTED_NODE1_IP="${NESTED_NODE1_IP:-10.31.12.102}"
NESTED_NODE1_NAME="${NESTED_NODE1_NAME:-pve-test-1}"
NESTED_TOKEN_ID="${NESTED_TOKEN_ID:-pcenter@pve!harness}"
: "${PVE_TEST_TOKEN_SECRET:?PVE_TEST_TOKEN_SECRET not set}"
: "${PROXMOX_USERNAME:?source proxmox-admin/.env}"
: "${PROXMOX_PASSWORD:?source proxmox-admin/.env}"

PCENTER_LXC_HOSTNAME="pcenter-smoke-$$"
PCENTER_BRIDGE="${PCENTER_BRIDGE:-vmbr1}"
PCENTER_VLAN="${PCENTER_VLAN:-502}"
PCENTER_ADMIN_USER="smoke"
PCENTER_ADMIN_PASS="Smoke-$$-Pa55word!"
WEBHOOK_RECEIVER_PORT=9999

# Runtime state
PCENTER_LXC_VMID=""
PCENTER_IP=""
PCENTER_URL=""
CSRF_TOKEN=""
LXC_OSTEMPLATE=""
COOKIE_JAR="$(mktemp)"

# ===== Logging =====
log()  { printf '\e[1;34m[%(%H:%M:%S)T]\e[0m %s\n' -1 "$*" >&2; }
pass() { printf '\e[1;32mPASS\e[0m  %s\n' "$*" >&2; }
fail() { printf '\e[1;31mFAIL\e[0m  %s\n' "$*" >&2; exit 1; }
die()  { printf '\e[1;31mFATAL\e[0m %s\n' "$*" >&2; exit 2; }

# ===== Cleanup (teardown even on failure) =====
cleanup() {
  local rc=$?
  rm -f "$COOKIE_JAR"
  if [[ -n "$PCENTER_LXC_VMID" && "${KEEP_LXC:-0}" != "1" ]]; then
    log "teardown: destroying pCenter LXC $PCENTER_LXC_VMID"
    pve_api POST "/nodes/$PVE_NODE/lxc/$PCENTER_LXC_VMID/status/stop" >/dev/null 2>&1 || true
    sleep 3
    pve_api DELETE "/nodes/$PVE_NODE/lxc/$PCENTER_LXC_VMID?purge=1&destroy-unreferenced-disks=1" \
      >/dev/null 2>&1 || true
    [[ -n "$PCENTER_IP" ]] && ssh-keygen -R "$PCENTER_IP" >/dev/null 2>&1 || true
  fi
  exit $rc
}
trap cleanup EXIT

# ===== PVE API (pve05, ticket+CSRF auth) =====
PVE_TICKET=""
PVE_CSRF=""
pve_login() {
  local resp
  resp=$(curl -sfk -d "username=${PROXMOX_USERNAME}&password=${PROXMOX_PASSWORD}" \
    "https://${PVE_HOST}:8006/api2/json/access/ticket") || die "pve login failed"
  PVE_TICKET=$(jq -r '.data.ticket' <<<"$resp")
  PVE_CSRF=$(jq -r '.data.CSRFPreventionToken' <<<"$resp")
}
pve_api() {
  local method="$1" path="$2"; shift 2
  curl -sfk -X "$method" \
    --cookie "PVEAuthCookie=$PVE_TICKET" \
    -H "CSRFPreventionToken: $PVE_CSRF" \
    "$@" "https://${PVE_HOST}:8006/api2/json${path}"
}

# ===== Nested-cluster API (token auth) =====
nested_api() {
  local method="$1" path="$2"; shift 2
  curl -sfk -X "$method" \
    -H "Authorization: PVEAPIToken=${NESTED_TOKEN_ID}=${PVE_TEST_TOKEN_SECRET}" \
    "$@" "https://${NESTED_NODE1_IP}:8006/api2/json${path}"
}

# ===== pCenter API (cookie jar + CSRF) =====
pc_api() {
  local method="$1" path="$2"; shift 2
  local args=(-sfk -X "$method" -b "$COOKIE_JAR" -c "$COOKIE_JAR"
              -H "Content-Type: application/json")
  [[ -n "$CSRF_TOKEN" ]] && args+=(-H "X-CSRF-Token: $CSRF_TOKEN")
  curl "${args[@]}" "$@" "${PCENTER_URL}${path}"
}

# ===== Wait helper — fast-poll with hard deadline =====
# Design choice: 3s interval, per-stage deadline. Typical nested cluster
# reconvergence is ~60s; we give quorum 150s to absorb a cold-pve05 case.
# Exits the moment the condition is met (vs. a fixed sleep that always
# pays the worst case).
wait_until() {
  local desc="$1" deadline="$2" interval="$3" check="$4"
  local start now
  start=$(date +%s)
  while true; do
    if eval "$check" >/dev/null 2>&1; then
      now=$(date +%s)
      log "  $desc: ready after $((now - start))s"
      return 0
    fi
    now=$(date +%s)
    if (( now - start >= deadline )); then
      fail "$desc: not ready within ${deadline}s"
    fi
    sleep "$interval"
  done
}

# ============================================================
# Stage 1 — rollback nested cluster to golden, start
# ============================================================
stage_rollback() {
  log "stage 1: rollback nested cluster to golden"
  for vmid in "$NESTED_NODE1_VMID" "$NESTED_NODE2_VMID"; do
    pve_api POST "/nodes/$PVE_NODE/qemu/$vmid/status/stop" >/dev/null 2>&1 || true
  done
  sleep 5
  for vmid in "$NESTED_NODE1_VMID" "$NESTED_NODE2_VMID"; do
    log "  rollback qemu/$vmid -> golden"
    pve_api POST "/nodes/$PVE_NODE/qemu/$vmid/snapshot/golden/rollback" >/dev/null \
      || fail "rollback $vmid failed"
  done
  sleep 10  # snapshot restore task is async; 10s is enough for LVM-thin CoW
  for vmid in "$NESTED_NODE1_VMID" "$NESTED_NODE2_VMID"; do
    pve_api POST "/nodes/$PVE_NODE/qemu/$vmid/status/start" >/dev/null \
      || fail "start $vmid failed"
  done
  pass "rollback + start issued"
}

# ============================================================
# Stage 2 — wait for nested cluster quorum
# ============================================================
stage_wait_quorum() {
  log "stage 2: wait for nested cluster API + quorum"
  wait_until "nested API responds" 90 3 \
    "nested_api GET /version"
  wait_until "cluster quorate" 150 3 \
    "nested_api GET /cluster/status | jq -e '.data[] | select(.type==\"cluster\") | select(.quorate==1)' >/dev/null"
  pass "nested cluster quorate"
}

# ============================================================
# Stage 3 — confirm LXC template pre-staged in golden
# ============================================================
stage_check_template() {
  log "stage 3: verify LXC template on nested local storage"
  local templates
  templates=$(nested_api GET "/nodes/${NESTED_NODE1_NAME}/storage/local/content?content=vztmpl" \
              | jq -r '.data[].volid' 2>/dev/null || true)
  if [[ -z "$templates" ]]; then
    die "no LXC template on ${NESTED_NODE1_NAME}:local.
Fix: SSH to pve-test-1 and run
  pveam update && pveam download local alpine-3.19-default_...
then shutdown VMs 109+110 and re-snapshot 'golden'.
See docs/test-harness.md 'Rebuilding from zero'."
  fi
  LXC_OSTEMPLATE=$(head -n1 <<<"$templates")
  log "  template: $LXC_OSTEMPLATE"
}

# ============================================================
# Stage 4 — provision pCenter-under-test LXC on pve05, VLAN 502
# ============================================================
stage_provision_lxc() {
  log "stage 4: provision pCenter LXC on $PVE_NODE (VLAN $PCENTER_VLAN)"
  PCENTER_LXC_VMID=$(pve_api GET "/cluster/nextid" | jq -r '.data')
  # Static IP outside pfSense DHCP range (100-200) to avoid lease collisions
  PCENTER_IP="10.31.12.$((200 + RANDOM % 50))"
  log "  VMID=$PCENTER_LXC_VMID  IP=$PCENTER_IP"

  pve_api POST "/nodes/$PVE_NODE/lxc" \
    --data-urlencode "vmid=$PCENTER_LXC_VMID" \
    --data-urlencode "hostname=$PCENTER_LXC_HOSTNAME" \
    --data-urlencode "ostemplate=local:vztmpl/ubuntu-24.04-standard_24.04-2_amd64.tar.zst" \
    --data-urlencode "storage=local-lvm" \
    --data-urlencode "rootfs=local-lvm:4" \
    --data-urlencode "cores=1" \
    --data-urlencode "memory=512" \
    --data-urlencode "swap=512" \
    --data-urlencode "net0=name=eth0,bridge=${PCENTER_BRIDGE},tag=${PCENTER_VLAN},ip=${PCENTER_IP}/24,gw=10.31.12.1" \
    --data-urlencode "unprivileged=1" \
    --data-urlencode "features=nesting=0" \
    --data-urlencode "start=1" \
    --data-urlencode "password=smoke-$$-pw" \
    --data-urlencode "ssh-public-keys=$(cat ~/.ssh/id_rsa.pub)" \
    --data-urlencode "ostype=ubuntu" >/dev/null \
    || fail "LXC create failed"

  wait_until "SSH on pCenter LXC" 120 3 \
    "ssh -o ConnectTimeout=3 -o BatchMode=yes -o StrictHostKeyChecking=accept-new root@${PCENTER_IP} true"
  pass "pCenter LXC up at $PCENTER_IP"
}

# ============================================================
# Stage 5 — install pCenter from APT (README Quick Install path)
# ============================================================
stage_install_pcenter() {
  if [[ -n "${LOCAL_DEB:-}" ]]; then
    log "stage 5: install pCenter from LOCAL_DEB=$LOCAL_DEB"
    [[ -f "$LOCAL_DEB" ]] || die "LOCAL_DEB path does not exist: $LOCAL_DEB"
    scp -o BatchMode=yes "$LOCAL_DEB" "root@${PCENTER_IP}:/tmp/pcenter.deb" >/dev/null \
      || fail "scp of local .deb failed"
  else
    log "stage 5: install pCenter $VERSION via APT + start"
  fi
  # Cluster goes into config.yaml at install time. This IS the happy path —
  # at startup, pCenter migrates config.yaml clusters into the inventory DB
  # (including token_secret, fixed in #46). The poller reads from inventory.
  ssh -o BatchMode=yes "root@${PCENTER_IP}" \
    "NESTED_NODE1_IP='${NESTED_NODE1_IP}' NESTED_TOKEN_ID='${NESTED_TOKEN_ID}' PVE_TEST_TOKEN_SECRET='${PVE_TEST_TOKEN_SECRET}' LOCAL_DEB='${LOCAL_DEB:-}' bash" <<'INSTALL'
set -e
# #43 workaround: Ubuntu 24.04 LXC template lacks curl+gpg
apt-get update -qq
apt-get install -y -qq curl gpg ca-certificates python3

if [[ -n "$LOCAL_DEB" ]]; then
  DEBIAN_FRONTEND=noninteractive apt-get install -y /tmp/pcenter.deb
else
  curl -fsSL https://marcwoconnor.github.io/pCenter/pcenter.gpg.key \
    | gpg --dearmor -o /usr/share/keyrings/pcenter.gpg
  echo "deb [signed-by=/usr/share/keyrings/pcenter.gpg] https://marcwoconnor.github.io/pCenter stable main" \
    > /etc/apt/sources.list.d/pcenter.list
  apt-get update -qq
  DEBIAN_FRONTEND=noninteractive apt-get install -y pcenter
fi

cat > /etc/pcenter/config.yaml <<CFG
clusters:
  - name: test
    discovery_node: "${NESTED_NODE1_IP}:8006"
    token_id: "${NESTED_TOKEN_ID}"
    token_secret: "\${PVE_TEST_TOKEN_SECRET}"
    insecure: true
server:
  port: 8080
auth:
  enabled: true
  database_path: /opt/pcenter/data/auth.db
poller:
  enabled: true
metrics:
  enabled: true
  database_path: /opt/pcenter/data/metrics.db
activity:
  database_path: /opt/pcenter/data/activity.db
folders:
  database_path: /opt/pcenter/data/folders.db
inventory:
  database_path: /opt/pcenter/data/inventory.db
library:
  enabled: true
  database_path: /opt/pcenter/data/library.db
drs:
  enabled: true
  mode: manual
CFG

cat > /etc/pcenter/env <<ENV
PVE_TEST_TOKEN_SECRET=${PVE_TEST_TOKEN_SECRET}
ENV
chmod 600 /etc/pcenter/env

systemctl start pcenter
INSTALL

  PCENTER_URL="http://${PCENTER_IP}:8080"
  wait_until "pCenter /health" 60 2 \
    "curl -sf ${PCENTER_URL}/health | jq -e '.status==\"ok\"' >/dev/null"
  pass "pCenter $VERSION healthy at $PCENTER_URL"
}

# ============================================================
# Stage 6 — bootstrap admin user
# ============================================================
stage_register_admin() {
  log "stage 6: register first admin via /api/auth/register"
  local resp
  resp=$(pc_api POST "/api/auth/register" \
    -d "$(jq -n --arg u "$PCENTER_ADMIN_USER" --arg p "$PCENTER_ADMIN_PASS" \
          '{username:$u, password:$p, email:"smoke@example.com"}')")
  CSRF_TOKEN=$(jq -r '.csrf_token' <<<"$resp")
  [[ -n "$CSRF_TOKEN" && "$CSRF_TOKEN" != "null" ]] || fail "register: no csrf_token"
  pass "admin user '$PCENTER_ADMIN_USER' registered"
}

# Stage 7 (inventory API add-cluster) removed — see #46. The poller
# picks up the cluster from config.yaml at boot instead. When #46 is
# fixed, restore this stage to validate the inventory path.

# ============================================================
# Stage 8 — wait for poller to discover nested nodes
# ============================================================
stage_poller_ready() {
  log "stage 8: wait for poller to see both nested nodes online"
  wait_until "pCenter reports 2 online nodes in test cluster" 90 3 \
    "pc_api GET /api/clusters | jq -e '.clusters[] | select(.name==\"test\") | .summary.OnlineNodes == 2' >/dev/null"
  pass "poller sees both nested nodes online"
}

# ============================================================
# Stage 9 — LXC lifecycle through pCenter
# ============================================================
stage_lxc_lifecycle() {
  log "stage 9: create + delete LXC via pCenter"
  local vmid upid
  vmid=$(pc_api GET "/api/clusters/test/nextid" | jq -r '.vmid // .data // empty')
  [[ -n "$vmid" ]] || fail "nextid returned empty"
  log "  allocating vmid=$vmid"

  upid=$(pc_api POST "/api/clusters/test/nodes/${NESTED_NODE1_NAME}/containers" \
    -d "$(jq -n \
      --arg tmpl "$LXC_OSTEMPLATE" \
      --argjson vmid "$vmid" \
      '{vmid:$vmid, hostname:"smoke-ct", ostemplate:$tmpl, storage:"local-lvm", disk_size:2, cores:1, memory:256, password:"smoke"}')" \
    | jq -r '.upid // empty')
  [[ -n "$upid" ]] || fail "container create returned no UPID"

  wait_until "ct $vmid visible in pCenter" 90 3 \
    "pc_api GET /api/containers | jq -e '.[] | select(.vmid==$vmid)' >/dev/null"

  pc_api POST "/api/clusters/test/containers/$vmid/stop" >/dev/null || true
  sleep 3
  pc_api DELETE "/api/clusters/test/containers/$vmid?purge=1" >/dev/null \
    || fail "container delete failed"
  pass "LXC $vmid created + deleted via pCenter"
}

# ============================================================
# Stage 10 — webhook: create, deliver, verify HMAC
# ============================================================
# Approach: run a tiny Python HTTP listener on the pCenter LXC's loopback
# (port 9999). Configure a webhook pointed at http://127.0.0.1:9999/.
# Trigger /test, wait for delivery, verify the X-Webhook-Signature HMAC
# matches the captured body using the secret we got at creation time.
stage_webhook() {
  log "stage 10: webhook create + deliver + verify HMAC"

  # Start receiver in the LXC (background, killed on teardown of LXC)
  ssh -o BatchMode=yes "root@${PCENTER_IP}" bash <<'RECV'
cat > /tmp/webhook_recv.py <<'PY'
import http.server, json
class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        n = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(n)
        # Headers are case-insensitive — emit all of them to simplify debug
        hdrs = {k: v for k, v in self.headers.items()}
        with open('/tmp/webhook_capture.json', 'w') as f:
            json.dump({'headers': hdrs, 'body': body.decode('utf-8', errors='replace')}, f)
        self.send_response(200)
        self.end_headers()
    def log_message(self, *a, **k): pass
http.server.HTTPServer(('127.0.0.1', 9999), H).serve_forever()
PY
rm -f /tmp/webhook_capture.json
nohup python3 /tmp/webhook_recv.py >/dev/null 2>&1 &
sleep 1
RECV

  # Create webhook — the response returns the HMAC secret once (write-only after).
  local resp wid secret
  resp=$(pc_api POST "/api/webhooks" \
    -d '{"name":"smoke","url":"http://127.0.0.1:9999/","events":["webhook.test"],"enabled":true}')
  wid=$(jq -r '.endpoint.id // .id' <<<"$resp")
  secret=$(jq -r '.secret' <<<"$resp")
  [[ -n "$wid" && "$wid" != "null" ]]   || fail "webhook create: no id"
  [[ -n "$secret" && "$secret" != "null" ]] || fail "webhook create: no secret (crypto broken?)"

  # Trigger synthetic test event and wait for receiver capture
  pc_api POST "/api/webhooks/$wid/test" >/dev/null || fail "webhook test trigger failed"
  wait_until "webhook delivered" 30 1 \
    "ssh -o BatchMode=yes root@${PCENTER_IP} test -f /tmp/webhook_capture.json"

  # Pull capture + verify HMAC locally.
  # Header is X-pCenter-Signature. Format is Stripe-style: t=<unix>,v1=<hex>
  # where the HMAC input is `<unix>.<body>` (literal dot, no separator bytes).
  local capture sig body ts v1 signed_payload expected
  capture=$(ssh -o BatchMode=yes "root@${PCENTER_IP}" cat /tmp/webhook_capture.json)
  sig=$(jq -r '.headers["X-pCenter-Signature"] // .headers["X-Pcenter-Signature"] // empty' <<<"$capture")
  body=$(jq -r '.body' <<<"$capture")
  [[ -n "$sig" ]] || { log "  captured headers: $(jq -c .headers <<<"$capture")"; fail "no X-pCenter-Signature header on delivery"; }

  ts=$(sed -n 's/^t=\([0-9]*\),.*/\1/p' <<<"$sig")
  v1=$(sed -n 's/.*v1=\([0-9a-f]*\).*/\1/p' <<<"$sig")
  [[ -n "$ts" && -n "$v1" ]] || fail "signature parse failed: $sig"

  signed_payload="${ts}.${body}"
  expected=$(printf '%s' "$signed_payload" | openssl dgst -sha256 -hmac "$secret" -binary | xxd -p -c 256)
  if [[ "$v1" != "$expected" ]]; then
    log "  got v1: $v1"
    log "  exp v1: $expected"
    log "  signed_payload: $signed_payload"
    fail "HMAC signature mismatch"
  fi

  pc_api DELETE "/api/webhooks/$wid" >/dev/null || true
  pass "webhook delivered + HMAC-SHA256 verified"
}

# ============================================================
# Stage 11 — alarms API reachable
# ============================================================
stage_alarms() {
  log "stage 11: alarms API"
  pc_api GET "/api/alarms/definitions" | jq -e 'type=="array" or type=="object"' >/dev/null \
    || fail "alarms definitions endpoint failed"
  pc_api GET "/api/alarms" | jq -e 'type=="array" or type=="object"' >/dev/null \
    || fail "alarms list endpoint failed"
  pass "alarms API ok"
}

# ============================================================
# Main
# ============================================================
main() {
  for tool in jq curl openssl ssh xxd; do
    command -v "$tool" >/dev/null || die "$tool not installed"
  done

  log "=== pCenter release smoke: version $VERSION ==="
  pve_login
  stage_rollback
  stage_wait_quorum
  stage_check_template
  stage_provision_lxc
  stage_install_pcenter
  stage_register_admin
  stage_poller_ready
  stage_lxc_lifecycle
  stage_webhook
  stage_alarms
  log "=== ALL SMOKE TESTS PASSED for $VERSION ==="
}

main "$@"
