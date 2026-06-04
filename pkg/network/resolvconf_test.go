package network

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseResolvConf(t *testing.T) {
	dir := t.TempDir()
	content := `# Comment line
; Another comment
nameserver 8.8.8.8
nameserver 1.1.1.1
search example.com local
options ndots:2 timeout:3
sortlist 130.155.160.0/255.255.240.0
`
	path := filepath.Join(dir, "resolv.conf")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	rc, err := ParseResolvConf(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rc.Nameservers) != 2 {
		t.Errorf("nameservers = %d, want 2", len(rc.Nameservers))
	}
	if rc.Nameservers[0] != "8.8.8.8" {
		t.Errorf("ns[0] = %s, want 8.8.8.8", rc.Nameservers[0])
	}
	if rc.Nameservers[1] != "1.1.1.1" {
		t.Errorf("ns[1] = %s, want 1.1.1.1", rc.Nameservers[1])
	}
	if len(rc.Search) != 2 {
		t.Errorf("search = %d, want 2", len(rc.Search))
	}
	if rc.Search[0] != "example.com" || rc.Search[1] != "local" {
		t.Errorf("search = %v, want [example.com local]", rc.Search)
	}
	if len(rc.Options) != 2 {
		t.Errorf("options = %d, want 2", len(rc.Options))
	}
	if len(rc.SortList) != 1 {
		t.Errorf("sortlist = %d, want 1", len(rc.SortList))
	}
}

func TestParseResolvConfEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.conf")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	rc, err := ParseResolvConf(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rc.Nameservers) != 0 {
		t.Errorf("nameservers = %d, want 0", len(rc.Nameservers))
	}
}

func TestParseResolvConfCommentsOnly(t *testing.T) {
	dir := t.TempDir()
	content := "# Just a comment\n; Another comment\n"
	path := filepath.Join(dir, "comments.conf")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	rc, err := ParseResolvConf(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rc.Nameservers) != 0 {
		t.Errorf("nameservers = %d, want 0", len(rc.Nameservers))
	}
}

func TestParseResolvConfWithPort(t *testing.T) {
	dir := t.TempDir()
	content := "nameserver 10.0.0.1:5353\n"
	path := filepath.Join(dir, "port.conf")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	rc, err := ParseResolvConf(path)
	if err != nil {
		t.Fatal(err)
	}
	if rc.Nameservers[0] != "10.0.0.1" {
		t.Errorf("ns[0] = %s, want 10.0.0.1 (port stripped)", rc.Nameservers[0])
	}
}

func TestParseResolvConfNotFound(t *testing.T) {
	_, err := ParseResolvConf("/nonexistent/path/resolv.conf")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestHostResolvConf(t *testing.T) {
	rc := HostResolvConf()
	if len(rc.Nameservers) == 0 {
		t.Error("HostResolvConf returned empty nameservers")
	}
	// Nameservers are stored without port after C3 fix.
	// Use NameserverList() to get host:port format.
	for _, ns := range rc.Nameservers {
		if strings.Contains(ns, ":") {
			t.Errorf("nameserver %s should not have port (stripped by ParseResolvConf)", ns)
		}
	}
	// Verify port is added by NameserverList.
	list := rc.NameserverList()
	for _, ns := range list {
		if !strings.Contains(ns, ":") {
			t.Errorf("NameserverList() %s missing port", ns)
		}
	}
}

func TestNameserverList(t *testing.T) {
	t.Run("with nameservers", func(t *testing.T) {
		rc := &ResolvConf{Nameservers: []string{"1.1.1.1", "8.8.8.8"}}
		list := rc.NameserverList()
		if len(list) != 2 {
			t.Errorf("len = %d, want 2", len(list))
		}
		if list[0] != "1.1.1.1:53" {
			t.Errorf("list[0] = %s, want 1.1.1.1:53", list[0])
		}
	})
	t.Run("empty", func(t *testing.T) {
		rc := &ResolvConf{}
		list := rc.NameserverList()
		if len(list) != 2 {
			t.Errorf("len = %d, want 2 (default)", len(list))
		}
		if list[0] != "8.8.8.8:53" {
			t.Errorf("default[0] = %s, want 8.8.8.8:53", list[0])
		}
	})
}
