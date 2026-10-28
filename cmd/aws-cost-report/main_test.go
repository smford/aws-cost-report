package main

import "testing"

func TestVersionDefaults(t *testing.T) {
	if version == "" {
		t.Error("expected non-empty version")
	}
}
