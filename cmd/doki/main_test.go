package main

import "testing"

func TestDoctorCommandRegistered(t *testing.T) {
	cmd, ok := commands["doctor"]
	if !ok {
		t.Fatal("doctor command is not registered")
	}
	if cmd.Handler == nil {
		t.Fatal("doctor command has nil handler")
	}
	if len(cmd.Aliases) != 1 || cmd.Aliases[0] != "diagnose" {
		t.Fatalf("doctor aliases = %v, want [diagnose]", cmd.Aliases)
	}
}
