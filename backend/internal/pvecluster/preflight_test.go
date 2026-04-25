package pvecluster

import (
	"testing"
)

func TestValidateClusterName(t *testing.T) {
	ok := []string{"a", "lab1", "home-lab", "Home9"}
	for _, n := range ok {
		if msg := validateClusterName(n); msg != "" {
			t.Errorf("%q should be valid, got error: %s", n, msg)
		}
	}
	bad := []string{
		"",                 // empty
		"-leading",         // leading dash
		"trailing-",        // trailing dash
		"under_score",      // underscore not allowed
		"has space",        // space
		"way-too-long-name-here", // >15 chars
		"home.lab",         // dot
	}
	for _, n := range bad {
		if msg := validateClusterName(n); msg == "" {
			t.Errorf("%q should be invalid but validateClusterName accepted it", n)
		}
	}
}

func TestMajorVersion(t *testing.T) {
	cases := map[string]string{
		"8.2.4":                 "8",
		"7.4-17":                "7",
		"pve-manager/8.0.3/abc": "8",
		"9":                     "9",
		"":                      "",
	}
	for in, want := range cases {
		if got := majorVersion(in); got != want {
			t.Errorf("majorVersion(%q) = %q, want %q", in, got, want)
		}
	}
}
