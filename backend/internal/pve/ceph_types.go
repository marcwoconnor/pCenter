package pve

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// Types in this file model the read surface of /nodes/{node}/ceph/* beyond
// /ceph/status (which lives in types.go alongside the older Ceph health types).
// Field names track PVE's REST response keys verbatim — when extending, run
// `pvesh get /nodes/{node}/ceph/...` against a real cluster and mirror the keys
// rather than translating, so changes in PVE surface as test failures rather
// than silent drift.

// CephOSD is one OSD daemon as returned by /nodes/{node}/ceph/osd. PVE returns
// a CRUSH tree there; CephOSD is the leaf shape, flattened across the tree by
// the client. Only fields the day-2 UI actually displays are modeled.
type CephOSD struct {
	ID          int     `json:"id"`           // numeric OSD id, e.g. 3
	Name        string  `json:"name"`         // "osd.3"
	Type        string  `json:"type"`         // "osd"
	Host        string  `json:"host"`         // hostname owning the OSD (populated by client from tree parent)
	DeviceClass string  `json:"device_class"` // "hdd" | "ssd" | "nvme"
	Status      string  `json:"status"`       // "up" | "down"
	In          bool    `json:"in"`           // true if reweight > 0
	CrushWeight float64 `json:"crush_weight"`
	Reweight    float64 `json:"reweight"`
	BytesUsed   int64   `json:"used_bytes,omitempty"`
	BytesAvail  int64   `json:"avail_bytes,omitempty"`
	BytesTotal  int64   `json:"total_bytes,omitempty"`
}

// CephMON is one Ceph monitor daemon from /nodes/{node}/ceph/mon.
//
// PVE's perl JSON encoder serializes booleans as integers (`"quorum":1` not
// `"quorum":true`), so Quorum is typed as intBool — a plain bool would fail
// the entire decode and the whole MON list would render empty.
type CephMON struct {
	Name      string  `json:"name"`     // mon hostname
	Addr      string  `json:"addr"`     // ip:port[/nonce]
	Host      string  `json:"host"`     // PVE node owning the MON
	Rank      int     `json:"rank"`     // monmap rank
	Quorum    intBool `json:"quorum"`   // currently in quorum
	State     string  `json:"state"`    // "leader" | "peon" | "synchronizing" | ...
	CephVer   string  `json:"ceph_version,omitempty"`
	Direction string  `json:"direction,omitempty"` // PVE-specific: "in"/"out"
}

// CephMGR is one Ceph manager daemon from /nodes/{node}/ceph/mgr.
type CephMGR struct {
	Name    string `json:"name"`
	Host    string `json:"host"`
	State   string `json:"state"`              // "active" | "standby"
	Addr    string `json:"addr,omitempty"`
	CephVer string `json:"ceph_version,omitempty"`
}

// CephMDS is one Ceph metadata server daemon from /nodes/{node}/ceph/mds.
// Standby is intBool for the same reason as CephMON.Quorum — PVE serializes
// booleans as integers.
type CephMDS struct {
	Name    string  `json:"name"`
	Host    string  `json:"host"`
	Addr    string  `json:"addr,omitempty"`
	State   string  `json:"state"`             // "up:active" | "up:standby" | ...
	Rank    int     `json:"rank,omitempty"`
	Standby intBool `json:"standby_replay,omitempty"`
}

// CephPool is a Ceph storage pool from /nodes/{node}/ceph/pool.
type CephPool struct {
	ID            int     `json:"id"`             // pool id
	Name          string  `json:"pool_name"`      // PVE returns "pool_name", not "name"
	Size          int     `json:"size"`           // replica count
	MinSize       int     `json:"min_size"`       // min replicas for I/O
	PGNum         int     `json:"pg_num"`
	PGNumMin      int     `json:"pg_num_min,omitempty"`
	PGAutoscale   string  `json:"pg_autoscale_mode,omitempty"` // "on" | "off" | "warn"
	CrushRule     int     `json:"crush_rule"`
	CrushRuleName string  `json:"crush_rule_name,omitempty"` // populated by client lookup
	Application   string  `json:"application,omitempty"`     // "rbd" | "cephfs" | "rgw"
	BytesUsed     int64   `json:"bytes_used,omitempty"`
	MaxAvail      int64   `json:"max_avail,omitempty"`
	PercentUsed   float64 `json:"percent_used,omitempty"`
	Type          string  `json:"type,omitempty"` // "replicated" | "erasure"
}

// CephRule is one CRUSH rule from /nodes/{node}/ceph/rules.
type CephRule struct {
	ID         int    `json:"rule_id"`
	Name       string `json:"rule_name"`
	Ruleset    int    `json:"ruleset,omitempty"`
	Type       int    `json:"type,omitempty"` // 1 = replicated, 3 = erasure
	StepCount  int    `json:"steps_count,omitempty"`
}

// CephFSEntry is one CephFS instance from /nodes/{node}/ceph/fs.
type CephFSEntry struct {
	Name         string   `json:"name"`
	MetadataPool string   `json:"metadata_pool,omitempty"`
	DataPools    []string `json:"data_pools,omitempty"`
}

// CephFlags reflects cluster-wide OSD flags. Each field is true when the
// flag is set. PVE exposes these via "ceph osd dump" output and via the
// /cluster/ceph/flags endpoint (PVE 7+).
type CephFlags struct {
	NoOut       bool `json:"noout"`
	NoIn        bool `json:"noin"`
	NoUp        bool `json:"noup"`
	NoDown      bool `json:"nodown"`
	NoBackfill  bool `json:"nobackfill"`
	NoRebalance bool `json:"norebalance"`
	NoRecover   bool `json:"norecover"`
	NoScrub     bool `json:"noscrub"`
	NoDeepScrub bool `json:"nodeep-scrub"`
	Pause       bool `json:"pause"`
}

// CephCluster aggregates the cluster-wide topology + status into a single
// snapshot the poller publishes and handlers consume. The poller fetches each
// list from any healthy node (cluster-wide data is identical from every MON).
type CephCluster struct {
	Status      *CephStatus   `json:"status,omitempty"`
	Version     string        `json:"version,omitempty"`
	MONs        []CephMON     `json:"mons"`
	MGRs        []CephMGR     `json:"mgrs"`
	MDSs        []CephMDS     `json:"mdss"`
	OSDs        []CephOSD     `json:"osds"`
	Pools       []CephPool    `json:"pools"`
	Rules       []CephRule    `json:"rules"`
	FS          []CephFSEntry `json:"fs"`
	Flags       CephFlags     `json:"flags"`
	LastUpdated time.Time     `json:"last_updated"`
}

// CephOSDTreeNode is the raw shape returned by GET /nodes/{node}/ceph/osd —
// PVE returns a CRUSH tree where each node has children. The client walks
// this tree to produce []CephOSD; this type is exported so callers that want
// the raw tree (e.g. a future CRUSH editor UI) can consume it directly.
//
// PVE returns the OSD id as a JSON STRING, not a number ("id":"2", not
// "id":2), so ID is typed as flexInt to accept both shapes — a plain int
// would silently fail the whole tree decode and the OSD list would be
// empty even on a healthy cluster.
type CephOSDTreeNode struct {
	ID          flexInt           `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"` // "root" | "host" | "osd"
	TypeID      flexInt           `json:"type_id,omitempty"`
	Status      string            `json:"status,omitempty"`
	Host        string            `json:"host,omitempty"`
	DeviceClass string            `json:"device_class,omitempty"`
	CrushWeight float64           `json:"crush_weight,omitempty"`
	Reweight    float64           `json:"reweight,omitempty"`
	Children    []CephOSDTreeNode `json:"children,omitempty"`
}

// flexInt unmarshals a JSON value that PVE may return as either a number
// or a quoted string (e.g. "id":"2" vs "id":2). Stored as a plain int
// after parse; callers should use int(v) when assigning to typed-int
// fields they expose externally.
type flexInt int

// UnmarshalJSON accepts a JSON number, a quoted decimal string, or null
// (which decodes to 0). Any other shape is a real error.
func (f *flexInt) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*f = 0
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		if s == "" {
			*f = 0
			return nil
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("flexInt: %q is not a decimal integer: %w", s, err)
		}
		*f = flexInt(n)
		return nil
	}
	var n int
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*f = flexInt(n)
	return nil
}

// CephOSDListResponse is what GET /nodes/{node}/ceph/osd actually returns
// (the data field, after generic unwrapping). Includes the CRUSH tree under
// "root" plus flat per-OSD stats which we splice in when flattening.
type CephOSDListResponse struct {
	Root  CephOSDTreeNode `json:"root"`
	Flags string          `json:"flags,omitempty"`
}

// intBool unmarshals a JSON value that PVE may return as a number (1/0), a
// boolean (true/false), or a quoted string ("1"/"0"/"true"/"false"). PVE's
// perl JSON encoder emits booleans as integers throughout the Ceph API
// (`"quorum":1`, `"value":0`, `"standby_replay":1`, ...). A plain Go `bool`
// rejects the integer with `cannot unmarshal number into Go struct field of
// type bool`, which fails the *entire* enclosing decode — so a single
// `quorum:1` in the array silently empties the whole MON list. Using
// intBool keeps the symmetric "PVE shape may be loose, accept what we
// know to mean true/false" stance that flexInt already takes for ints.
//
// The underlying type is bool, so callers read it just like a bool
// (`if mon.Quorum { ... }`); no explicit conversion is needed.
type intBool bool

// UnmarshalJSON accepts true/false, 1/0, "1"/"0", "true"/"false", or null
// (which decodes to false). Any other shape is a real error.
func (b *intBool) UnmarshalJSON(data []byte) error {
	s := string(data)
	switch s {
	case "true", "1", `"true"`, `"1"`:
		*b = true
		return nil
	case "false", "0", `"false"`, `"0"`, "null", "":
		*b = false
		return nil
	default:
		return fmt.Errorf("intBool: cannot interpret %q as boolean", s)
	}
}
