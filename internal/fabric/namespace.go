package fabric

import (
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const (
	dnsLabelMaxLen = 63
	labelHashLen   = 8
)

// NamespaceInput identifies one service instance in a repository worktree.
type NamespaceInput struct {
	Repo     string
	Branch   string
	Worktree string
	Service  string
}

// ServiceNamespace is the canonical, URL-safe identity for one service instance.
type ServiceNamespace struct {
	Repo      Label
	Scope     Label
	Worktree  Label
	Service   Label
	HostLabel string
}

// Label keeps both the readable slug and the collision-resistant label.
type Label struct {
	Raw   string
	Slug  string
	Hash  string
	Value string
}

// NamespaceConflict reports raw values that collapse to the same readable slug.
type NamespaceConflict struct {
	Field  string
	Slug   string
	Values []string
}

// CanonicalizeNamespace creates a stable URL-safe identity without consulting IO.
func CanonicalizeNamespace(in NamespaceInput) (ServiceNamespace, error) {
	repo, err := canonicalLabel("repo", in.Repo)
	if err != nil {
		return ServiceNamespace{}, err
	}
	scopeRaw := in.Branch
	if scopeRaw == "" {
		scopeRaw = in.Worktree
	}
	scope, err := canonicalLabel("scope", scopeRaw)
	if err != nil {
		return ServiceNamespace{}, err
	}
	service, err := canonicalLabel("service", in.Service)
	if err != nil {
		return ServiceNamespace{}, err
	}

	return ServiceNamespace{
		Repo:      repo,
		Scope:     scope,
		Worktree:  scope,
		Service:   service,
		HostLabel: strings.Join([]string{service.Value, scope.Value, repo.Value}, "."),
	}, nil
}

// FindNamespaceConflicts reports where different raw values share a slug.
func FindNamespaceConflicts(inputs []NamespaceInput) []NamespaceConflict {
	fields := map[string]map[string]map[string]struct{}{
		"repo":    {},
		"scope":   {},
		"service": {},
	}

	for _, in := range inputs {
		addSlug(fields["repo"], in.Repo)
		scope := in.Branch
		if scope == "" {
			scope = in.Worktree
		}
		addSlug(fields["scope"], scope)
		addSlug(fields["service"], in.Service)
	}

	var conflicts []NamespaceConflict
	for _, field := range []string{"repo", "scope", "service"} {
		for slug, raws := range fields[field] {
			if len(raws) < 2 {
				continue
			}
			values := make([]string, 0, len(raws))
			for raw := range raws {
				values = append(values, raw)
			}
			sort.Strings(values)
			conflicts = append(conflicts, NamespaceConflict{
				Field:  field,
				Slug:   slug,
				Values: values,
			})
		}
	}

	sort.Slice(conflicts, func(i, j int) bool {
		if conflicts[i].Field != conflicts[j].Field {
			return conflicts[i].Field < conflicts[j].Field
		}
		return conflicts[i].Slug < conflicts[j].Slug
	})
	return conflicts
}

func addSlug(slugs map[string]map[string]struct{}, raw string) {
	slug := slugify(raw)
	if slug == "" {
		return
	}
	if slugs[slug] == nil {
		slugs[slug] = map[string]struct{}{}
	}
	slugs[slug][raw] = struct{}{}
}

func canonicalLabel(field, raw string) (Label, error) {
	slug := slugify(raw)
	if slug == "" {
		return Label{}, fmt.Errorf("%s label cannot be empty", field)
	}
	hash := shortHash(raw)
	slug = truncateSlug(slug, dnsLabelMaxLen-len(hash)-1)
	return Label{
		Raw:   raw,
		Slug:  slug,
		Hash:  hash,
		Value: slug + "-" + hash,
	}, nil
}

func slugify(raw string) string {
	var b strings.Builder
	lastSep := false
	for _, r := range strings.TrimSpace(raw) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastSep = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(unicode.ToLower(r))
			lastSep = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastSep = false
		default:
			if b.Len() > 0 && !lastSep {
				b.WriteByte('-')
				lastSep = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func truncateSlug(slug string, maxLen int) string {
	if len(slug) <= maxLen {
		return slug
	}
	return strings.Trim(slug[:maxLen], "-")
}

func shortHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:])
	return strings.ToLower(encoded[:labelHashLen])
}

func hashUint32(raw string) uint32 {
	sum := sha256.Sum256([]byte(raw))
	return uint32(sum[0])<<24 | uint32(sum[1])<<16 | uint32(sum[2])<<8 | uint32(sum[3])
}
