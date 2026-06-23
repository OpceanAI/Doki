package deps

import "testing"

func TestSummarizeSystemDeps(t *testing.T) {
	results := []SystemDepResult{
		{Name: "proot", Required: true, Installed: true},
		{Name: "iptables", Optional: true, Installed: false},
		{Name: "fuse-overlayfs", Required: true, Installed: false},
	}
	summary := SummarizeSystemDeps(results)
	if summary.Total != 3 || summary.Installed != 1 {
		t.Fatalf("summary counts = total %d installed %d, want 3/1", summary.Total, summary.Installed)
	}
	if summary.Healthy() {
		t.Fatal("summary should be unhealthy with a missing required dependency")
	}
	if len(summary.MissingRequired) != 1 || summary.MissingRequired[0].Name != "fuse-overlayfs" {
		t.Fatalf("missing required = %+v", summary.MissingRequired)
	}
	if len(summary.MissingOptional) != 1 || summary.MissingOptional[0].Name != "iptables" {
		t.Fatalf("missing optional = %+v", summary.MissingOptional)
	}
}

func TestSummarizeSystemDepsHealthy(t *testing.T) {
	summary := SummarizeSystemDeps([]SystemDepResult{{Name: "proot", Required: true, Installed: true}})
	if !summary.Healthy() {
		t.Fatal("summary should be healthy when all required deps are installed")
	}
}
