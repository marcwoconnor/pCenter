package pve

import "testing"

func TestSSHHome_UsesEnvWhenSet(t *testing.T) {
	t.Setenv("HOME", "/opt/pcenter/data")
	if got := sshHome(); got != "/opt/pcenter/data" {
		t.Fatalf("sshHome() = %q, want %q", got, "/opt/pcenter/data")
	}
}

func TestSSHHome_FallsBackToRootWhenUnset(t *testing.T) {
	// Can't truly unset HOME in a test without clobbering the harness; simulate
	// with an empty string which is what Getenv returns for unset vars too.
	t.Setenv("HOME", "")
	if got := sshHome(); got != "/root" {
		t.Fatalf("sshHome() with empty HOME = %q, want /root", got)
	}
}
