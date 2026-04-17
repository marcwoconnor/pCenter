package pve

import (
	"encoding/json"
	"testing"
)

// TestACMEPluginUnmarshal_StringData covers the real PVE response shape where
// `data` is a newline-separated k=v string, not a JSON object.
func TestACMEPluginUnmarshal_StringData(t *testing.T) {
	body := []byte(`{"digest":"abc","type":"dns","plugin":"cloudflare","api":"cf","data":"CF_Account_ID=abc123\nCF_Email=x@y.com\nCF_Token=secret"}`)
	var p ACMEPlugin
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Plugin != "cloudflare" || p.API != "cf" || p.Type != "dns" {
		t.Fatalf("basic fields wrong: %+v", p)
	}
	if got, want := p.Data["CF_Account_ID"], "abc123"; got != want {
		t.Errorf("CF_Account_ID = %q, want %q", got, want)
	}
	if got, want := p.Data["CF_Email"], "x@y.com"; got != want {
		t.Errorf("CF_Email = %q, want %q", got, want)
	}
	if got, want := p.Data["CF_Token"], "secret"; got != want {
		t.Errorf("CF_Token = %q, want %q", got, want)
	}
}

// TestACMEPluginUnmarshal_Standalone covers the standalone plugin which has
// no `data` field at all.
func TestACMEPluginUnmarshal_Standalone(t *testing.T) {
	body := []byte(`{"digest":"xyz","type":"standalone","plugin":"standalone"}`)
	var p ACMEPlugin
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Plugin != "standalone" || p.Type != "standalone" {
		t.Fatalf("basic fields wrong: %+v", p)
	}
	if p.Data != nil {
		t.Errorf("Data = %v, want nil for standalone plugin", p.Data)
	}
}
