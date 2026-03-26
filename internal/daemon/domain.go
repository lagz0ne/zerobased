package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// domainRe validates domain names: RFC 1123 labels separated by dots.
// Rejects wildcards, path separators, control characters, and localhost.
var domainRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?\.[a-zA-Z]{2,}$`)

// ValidateDomain checks that a domain name is safe for use in hostnames and route IDs.
func ValidateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}
	if strings.EqualFold(domain, "localhost") || strings.HasSuffix(strings.ToLower(domain), ".localhost") {
		return fmt.Errorf("domain %q conflicts with localhost routing", domain)
	}
	if strings.ContainsAny(domain, "/:@#*?& \t\n\\") {
		return fmt.Errorf("domain %q contains invalid characters", domain)
	}
	if strings.Contains(domain, "..") {
		return fmt.Errorf("domain %q contains path traversal sequence", domain)
	}
	if !domainRe.MatchString(domain) {
		return fmt.Errorf("domain %q is not a valid hostname (RFC 1123)", domain)
	}
	return nil
}

// DomainEntry represents a configured external domain with optional TTL.
type DomainEntry struct {
	Domain     string    `json:"domain"`
	Added      time.Time `json:"added"`
	Expires    time.Time `json:"expires,omitempty"`    // zero = persistent
	Persistent bool      `json:"persistent,omitempty"` // explicit persistent flag
}

// IsExpired returns true if the domain has a TTL and it has passed.
func (d DomainEntry) IsExpired() bool {
	if d.Persistent || d.Expires.IsZero() {
		return false
	}
	return time.Now().After(d.Expires)
}

// TTLRemaining returns the remaining time before expiry, or 0 if persistent/expired.
func (d DomainEntry) TTLRemaining() time.Duration {
	if d.Persistent || d.Expires.IsZero() {
		return 0
	}
	rem := time.Until(d.Expires)
	if rem < 0 {
		return 0
	}
	return rem
}

// DefaultTTL is the default time-to-live for domains without --ttl or --persistent.
const DefaultTTL = 4 * time.Hour

// domainsFile returns the path to the domains persistence file.
func domainsFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".zerobased", "domains.json")
}

// LoadDomains reads the domains file with a shared lock. Returns nil if file doesn't exist.
func LoadDomains() []DomainEntry {
	f, err := os.Open(domainsFile())
	if err != nil {
		return nil
	}
	defer f.Close()

	syscall.Flock(int(f.Fd()), syscall.LOCK_SH)
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	var entries []DomainEntry
	if err := json.NewDecoder(f).Decode(&entries); err != nil {
		return nil
	}
	return entries
}

// SaveDomains writes the domains file atomically.
func SaveDomains(entries []DomainEntry) error {
	path := domainsFile()
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0700)

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write with file lock to prevent TOCTOU races
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock domains file: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	_, err = f.Write(append(data, '\n'))
	return err
}

// AddDomain adds a domain to the persistent file. Returns the @N index (1-based).
func AddDomain(domain string, ttl time.Duration, persistent bool) (int, error) {
	if err := ValidateDomain(domain); err != nil {
		return 0, err
	}
	entries := LoadDomains()

	// Check for duplicate
	for i, e := range entries {
		if e.Domain == domain {
			// Update TTL on existing
			entries[i].Added = time.Now()
			if persistent {
				entries[i].Persistent = true
				entries[i].Expires = time.Time{}
			} else {
				entries[i].Persistent = false
				entries[i].Expires = time.Now().Add(ttl)
			}
			if err := SaveDomains(entries); err != nil {
				return 0, err
			}
			return i + 1, nil
		}
	}

	entry := DomainEntry{
		Domain:     domain,
		Added:      time.Now(),
		Persistent: persistent,
	}
	if !persistent {
		entry.Expires = time.Now().Add(ttl)
	}

	entries = append(entries, entry)
	if err := SaveDomains(entries); err != nil {
		return 0, err
	}
	return len(entries), nil
}

// RemoveDomain removes a domain by @N index (1-based). Returns the removed domain name.
func RemoveDomain(index int) (string, error) {
	entries := LoadDomains()
	if index < 1 || index > len(entries) {
		return "", fmt.Errorf("invalid index @%d (have %d domains)", index, len(entries))
	}
	domain := entries[index-1].Domain
	entries = append(entries[:index-1], entries[index:]...)
	if err := SaveDomains(entries); err != nil {
		return "", err
	}
	return domain, nil
}

// RemoveAllDomains removes all domains. Returns the count removed.
func RemoveAllDomains() int {
	entries := LoadDomains()
	n := len(entries)
	if n > 0 {
		SaveDomains(nil)
	}
	return n
}

// SweepExpired removes expired domains. Returns removed domain names.
func SweepExpired() []string {
	entries := LoadDomains()
	var kept []DomainEntry
	var removed []string
	for _, e := range entries {
		if e.IsExpired() {
			removed = append(removed, e.Domain)
		} else {
			kept = append(kept, e)
		}
	}
	if len(removed) > 0 {
		SaveDomains(kept)
	}
	return removed
}

// DomainNames returns just the domain strings from entries.
func DomainNames(entries []DomainEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Domain
	}
	return names
}

// domainRouteID returns the Caddy route ID for a domain variant.
// Uses @ separator which cannot appear in valid domain names (RFC 1123).
func domainRouteID(baseID, domain string) string {
	return baseID + "@" + domain
}

// FormatTTL formats a duration for human display.
func FormatTTL(d time.Duration) string {
	if d <= 0 {
		return "expired"
	}
	d = d.Round(time.Minute)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm remaining", h, m)
	}
	return fmt.Sprintf("%dm remaining", m)
}

// ParseDomainRef parses "@1" into index 1. Returns 0 if not a ref.
func ParseDomainRef(s string) int {
	if !strings.HasPrefix(s, "@") {
		return 0
	}
	var idx int
	if _, err := fmt.Sscanf(s[1:], "%d", &idx); err != nil {
		return 0
	}
	return idx
}
