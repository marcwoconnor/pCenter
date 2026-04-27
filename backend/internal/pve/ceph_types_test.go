package pve

import (
	"encoding/json"
	"testing"
)

// TestIntBool_UnmarshalJSON guards every shape PVE has been observed emitting
// for boolean-ish fields (`quorum`, `value`, `standby_replay`, ...). PVE's
// perl JSON encoder uses integers for booleans throughout the Ceph API; a
// regression here silently empties whichever list the affected struct
// belongs to, because Go's json package fails the entire decode on the
// first int → bool mismatch.
func TestIntBool_UnmarshalJSON(t *testing.T) {
	type tc struct {
		input string
		want  bool
	}
	truthy := []tc{
		{`true`, true},
		{`1`, true},
		{`"true"`, true},
		{`"1"`, true},
	}
	falsy := []tc{
		{`false`, false},
		{`0`, false},
		{`"false"`, false},
		{`"0"`, false},
		{`null`, false},
	}
	for _, c := range append(truthy, falsy...) {
		var got intBool
		if err := json.Unmarshal([]byte(c.input), &got); err != nil {
			t.Errorf("Unmarshal(%s) errored: %v", c.input, err)
			continue
		}
		if bool(got) != c.want {
			t.Errorf("Unmarshal(%s) = %v, want %v", c.input, bool(got), c.want)
		}
	}

	// Garbage shapes are real errors — silent acceptance would mask
	// schema drift in PVE.
	for _, bad := range []string{`"hello"`, `2`, `[]`, `{}`} {
		var got intBool
		if err := json.Unmarshal([]byte(bad), &got); err == nil {
			t.Errorf("Unmarshal(%s) accepted, expected error", bad)
		}
	}
}

// TestCephMON_QuorumIntShape pins the regression: PVE returns
// `"quorum":1`, not `"quorum":true`. With the field typed as plain bool,
// json.Unmarshal errored on the first MON and the entire list came back
// empty — the Monitors tab rendered "None." even on healthy clusters.
func TestCephMON_QuorumIntShape(t *testing.T) {
	body := `[
		{"name":"a","host":"a","rank":0,"quorum":1,"state":"leader"},
		{"name":"b","host":"b","rank":1,"quorum":0,"state":"peon"}
	]`
	var mons []CephMON
	if err := json.Unmarshal([]byte(body), &mons); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(mons) != 2 {
		t.Fatalf("got %d MONs, want 2", len(mons))
	}
	if !mons[0].Quorum || mons[1].Quorum {
		t.Errorf("quorum mis-decoded: %+v", mons)
	}
}

// TestCephMDS_StandbyReplayIntShape mirrors the MON regression for MDSs;
// PVE serializes standby_replay as an integer too.
func TestCephMDS_StandbyReplayIntShape(t *testing.T) {
	body := `[
		{"name":"a","host":"a","state":"up:active","rank":0,"standby_replay":0},
		{"name":"b","host":"b","state":"up:standby-replay","standby_replay":1}
	]`
	var mdss []CephMDS
	if err := json.Unmarshal([]byte(body), &mdss); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(mdss) != 2 {
		t.Fatalf("got %d MDSs, want 2", len(mdss))
	}
	if mdss[0].Standby || !mdss[1].Standby {
		t.Errorf("standby_replay mis-decoded: %+v", mdss)
	}
}
