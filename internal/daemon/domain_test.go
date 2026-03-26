package daemon

import (
	"os"
	"testing"
	"time"
)

func TestDomainEntryIsExpired(t *testing.T) {
	// Persistent: never expires
	e := DomainEntry{Domain: "a.com", Persistent: true}
	if e.IsExpired() {
		t.Error("persistent domain should not expire")
	}

	// Future expiry: not expired
	e = DomainEntry{Domain: "b.com", Expires: time.Now().Add(time.Hour)}
	if e.IsExpired() {
		t.Error("future expiry should not be expired")
	}

	// Past expiry: expired
	e = DomainEntry{Domain: "c.com", Expires: time.Now().Add(-time.Hour)}
	if !e.IsExpired() {
		t.Error("past expiry should be expired")
	}

	// Zero expiry without persistent: treated as persistent (no TTL set)
	e = DomainEntry{Domain: "d.com"}
	if e.IsExpired() {
		t.Error("zero expiry without persistent flag should not expire")
	}
}

func TestDomainFilePersistence(t *testing.T) {
	dir := t.TempDir()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", orig)

	// Initially empty
	entries := LoadDomains()
	if len(entries) != 0 {
		t.Fatalf("expected empty, got %v", entries)
	}

	// Add domains
	idx1, err := AddDomain("preview.dev.co", 4*time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	if idx1 != 1 {
		t.Errorf("expected @1, got @%d", idx1)
	}

	idx2, err := AddDomain("box.ts.net", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if idx2 != 2 {
		t.Errorf("expected @2, got @%d", idx2)
	}

	// Reload
	entries = LoadDomains()
	if len(entries) != 2 {
		t.Fatalf("expected 2, got %d", len(entries))
	}
	if entries[0].Domain != "preview.dev.co" || entries[0].Persistent {
		t.Errorf("entry[0] = %+v", entries[0])
	}
	if entries[1].Domain != "box.ts.net" || !entries[1].Persistent {
		t.Errorf("entry[1] = %+v", entries[1])
	}

	// Duplicate updates TTL
	idx, err := AddDomain("preview.dev.co", 30*time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	if idx != 1 {
		t.Errorf("duplicate should return same index, got @%d", idx)
	}

	// Remove by index
	name, err := RemoveDomain(1)
	if err != nil {
		t.Fatal(err)
	}
	if name != "preview.dev.co" {
		t.Errorf("removed = %q", name)
	}

	entries = LoadDomains()
	if len(entries) != 1 || entries[0].Domain != "box.ts.net" {
		t.Errorf("after remove: %v", entries)
	}
}

func TestSweepExpired(t *testing.T) {
	dir := t.TempDir()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", orig)

	entries := []DomainEntry{
		{Domain: "expired.com", Expires: time.Now().Add(-time.Hour)},
		{Domain: "alive.com", Expires: time.Now().Add(time.Hour)},
		{Domain: "persistent.com", Persistent: true},
	}
	SaveDomains(entries)

	removed := SweepExpired()
	if len(removed) != 1 || removed[0] != "expired.com" {
		t.Errorf("removed = %v", removed)
	}

	remaining := LoadDomains()
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining, got %d", len(remaining))
	}
}

func TestDomainRouteID(t *testing.T) {
	got := domainRouteID("zb-myapp-api-3000", "preview.dev.co")
	want := "zb-myapp-api-3000@preview.dev.co"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestValidateDomain(t *testing.T) {
	valid := []string{
		"preview.dev.co",
		"box.tailnet.ts.net",
		"10.0.1.50.nip.io",
		"my-preview.example.com",
	}
	for _, d := range valid {
		if err := ValidateDomain(d); err != nil {
			t.Errorf("ValidateDomain(%q) = %v, want nil", d, err)
		}
	}

	invalid := []string{
		"",                        // empty
		"localhost",               // conflicts with local routing
		"foo.localhost",           // conflicts with local routing
		"../../admin",             // path traversal
		"evil.com/../../config",   // path separator
		"*.example.com",           // wildcard
		"foo bar.com",             // space
		"host:2019",               // port in domain
		"a",                       // single label, no TLD
	}
	for _, d := range invalid {
		if err := ValidateDomain(d); err == nil {
			t.Errorf("ValidateDomain(%q) = nil, want error", d)
		}
	}
}

func TestParseDomainRef(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"@1", 1},
		{"@2", 2},
		{"@10", 10},
		{"@0", 0},
		{"1", 0},
		{"@abc", 0},
		{"", 0},
	}
	for _, tt := range tests {
		got := ParseDomainRef(tt.in)
		if got != tt.want {
			t.Errorf("ParseDomainRef(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestDomainNames(t *testing.T) {
	entries := []DomainEntry{
		{Domain: "a.com"},
		{Domain: "b.com"},
	}
	names := DomainNames(entries)
	if len(names) != 2 || names[0] != "a.com" || names[1] != "b.com" {
		t.Errorf("got %v", names)
	}
}
