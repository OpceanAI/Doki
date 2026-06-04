package common

import "testing"

func TestVersionConstants(t *testing.T) {
	if DokiVersion == "" {
		t.Fatal("DokiVersion must not be empty")
	}
	if DokiAPIVersion == "" {
		t.Fatal("DokiAPIVersion must not be empty")
	}
	if DokiMinClient == "" {
		t.Fatal("DokiMinClient must not be empty")
	}
	// Version must be SemVer (e.g. 0.9.2).
	if len(DokiVersion) < 5 {
		t.Errorf("DokiVersion %q looks invalid", DokiVersion)
	}
	// API version must look like 1.NN
	if len(DokiAPIVersion) < 3 || DokiAPIVersion[0] != '1' {
		t.Errorf("DokiAPIVersion %q looks invalid", DokiAPIVersion)
	}
	// min <= current (numerically).
	var cur, min int
	for _, c := range DokiAPIVersion {
		if c >= '0' && c <= '9' {
			cur = cur*10 + int(c-'0')
		}
	}
	for _, c := range DokiMinClient {
		if c >= '0' && c <= '9' {
			min = min*10 + int(c-'0')
		}
	}
	if min > cur {
		t.Errorf("DokiMinClient %s > DokiAPIVersion %s", DokiMinClient, DokiAPIVersion)
	}
}

func TestGetVersion(t *testing.T) {
	v := GetVersion()
	if v == nil {
		t.Fatal("GetVersion returned nil")
	}
	if v.Version == "" {
		t.Error("Version empty")
	}
	if v.APIVersion == "" {
		t.Error("APIVersion empty")
	}
	if v.MinAPIVersion == "" {
		t.Error("MinAPIVersion empty")
	}
	if v.GoVersion == "" {
		t.Error("GoVersion empty")
	}
}

func TestFullVersion(t *testing.T) {
	s := FullVersion()
	if len(s) < 10 {
		t.Errorf("FullVersion too short: %q", s)
	}
	// Must mention Doki, the version, and the API.
	for _, want := range []string{"Doki", DokiVersion, DokiAPIVersion} {
		if !contains(s, want) {
			t.Errorf("FullVersion %q missing %q", s, want)
		}
	}
}

func TestUserAgent(t *testing.T) {
	ua := UserAgent()
	if ua == "" {
		t.Error("UserAgent empty")
	}
	if !contains(ua, "Doki/") {
		t.Errorf("UserAgent %q missing Doki/ prefix", ua)
	}
}

func contains(haystack, needle string) bool {
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
