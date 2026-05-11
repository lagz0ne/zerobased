package fabric

import (
	"strings"
	"testing"
)

func TestCanonicalizeNamespaceStableAndURLSafe(t *testing.T) {
	in := NamespaceInput{
		Repo:     "Acme Billing",
		Worktree: "/tmp/worktrees/billing-feature",
		Service:  "API Server",
	}

	first, err := CanonicalizeNamespace(in)
	if err != nil {
		t.Fatalf("CanonicalizeNamespace() error = %v", err)
	}
	second, err := CanonicalizeNamespace(in)
	if err != nil {
		t.Fatalf("CanonicalizeNamespace() second error = %v", err)
	}
	if first.HostLabel != second.HostLabel {
		t.Fatalf("namespace not stable: %q != %q", first.HostLabel, second.HostLabel)
	}
	if strings.ContainsAny(first.HostLabel, "/:@#?& \t\n\\") {
		t.Fatalf("namespace %q contains URL-unsafe characters", first.HostLabel)
	}
	if !strings.HasPrefix(first.HostLabel, first.Service.Value+".") {
		t.Fatalf("namespace %q should start with service label %q", first.HostLabel, first.Service.Value)
	}
	if !strings.HasSuffix(first.HostLabel, "."+first.Repo.Value) {
		t.Fatalf("namespace %q should end with repo label %q", first.HostLabel, first.Repo.Value)
	}
}

func TestCanonicalizeNamespaceWorktreesDoNotCollide(t *testing.T) {
	tests := []struct {
		name string
		a    NamespaceInput
		b    NamespaceInput
	}{
		{
			name: "same basename different parents",
			a:    NamespaceInput{Repo: "acme", Worktree: "/tmp/a/main", Service: "api"},
			b:    NamespaceInput{Repo: "acme", Worktree: "/tmp/b/main", Service: "api"},
		},
		{
			name: "case variant remains distinct",
			a:    NamespaceInput{Repo: "acme", Worktree: "Main", Service: "api"},
			b:    NamespaceInput{Repo: "acme", Worktree: "main", Service: "api"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := CanonicalizeNamespace(tt.a)
			if err != nil {
				t.Fatalf("CanonicalizeNamespace(a) error = %v", err)
			}
			b, err := CanonicalizeNamespace(tt.b)
			if err != nil {
				t.Fatalf("CanonicalizeNamespace(b) error = %v", err)
			}
			if a.HostLabel == b.HostLabel {
				t.Fatalf("names collided: %q", a.HostLabel)
			}
		})
	}
}

func TestCanonicalizeNamespaceExactShape(t *testing.T) {
	got, err := CanonicalizeNamespace(NamespaceInput{
		Repo:     "repo",
		Worktree: "main",
		Service:  "api",
	})
	if err != nil {
		t.Fatalf("CanonicalizeNamespace() error = %v", err)
	}
	want := got.Service.Value + "." + got.Worktree.Value + "." + got.Repo.Value
	if got.HostLabel != want {
		t.Fatalf("HostLabel = %q, want %q", got.HostLabel, want)
	}
}

func TestCanonicalizeNamespaceUsesBranchBeforeWorktree(t *testing.T) {
	got, err := CanonicalizeNamespace(NamespaceInput{
		Repo:     "repo",
		Branch:   "feature/auth",
		Worktree: "/tmp/random-worktree-name",
		Service:  "api",
	})
	if err != nil {
		t.Fatalf("CanonicalizeNamespace() error = %v", err)
	}
	if got.Scope.Raw != "feature/auth" {
		t.Fatalf("scope raw = %q, want branch", got.Scope.Raw)
	}
	if !strings.Contains(got.HostLabel, got.Scope.Value) {
		t.Fatalf("HostLabel = %q, want scope label %q", got.HostLabel, got.Scope.Value)
	}
}

func TestCanonicalizeNamespaceLabelsFitDNS(t *testing.T) {
	got, err := CanonicalizeNamespace(NamespaceInput{
		Repo:     strings.Repeat("repo-", 30),
		Worktree: strings.Repeat("feature-", 30),
		Service:  strings.Repeat("api-", 30),
	})
	if err != nil {
		t.Fatalf("CanonicalizeNamespace() error = %v", err)
	}

	for _, label := range []Label{got.Repo, got.Worktree, got.Service} {
		if len(label.Value) > dnsLabelMaxLen {
			t.Fatalf("label %q length = %d, want <= %d", label.Value, len(label.Value), dnsLabelMaxLen)
		}
		if strings.HasPrefix(label.Value, "-") || strings.HasSuffix(label.Value, "-") {
			t.Fatalf("label %q should not start or end with hyphen", label.Value)
		}
	}
}

func TestCanonicalizeNamespaceInvalidCases(t *testing.T) {
	tests := []struct {
		name string
		in   NamespaceInput
	}{
		{
			name: "empty repo",
			in:   NamespaceInput{Repo: "", Worktree: "main", Service: "api"},
		},
		{
			name: "empty worktree after cleanup",
			in:   NamespaceInput{Repo: "repo", Worktree: "///", Service: "api"},
		},
		{
			name: "empty service",
			in:   NamespaceInput{Repo: "repo", Worktree: "main", Service: "   "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := CanonicalizeNamespace(tt.in); err == nil {
				t.Fatal("CanonicalizeNamespace() error = nil, want error")
			}
		})
	}
}

func TestFindNamespaceConflictsVisible(t *testing.T) {
	inputs := []NamespaceInput{
		{Repo: "repo", Worktree: "main", Service: "api"},
		{Repo: "repo", Worktree: "MAIN", Service: "api"},
		{Repo: "repo", Worktree: "feature", Service: "api@web"},
		{Repo: "repo", Worktree: "other", Service: "api web"},
	}

	conflicts := FindNamespaceConflicts(inputs)
	want := map[string]bool{
		"scope:main":      false,
		"service:api-web": false,
	}
	for _, conflict := range conflicts {
		key := conflict.Field + ":" + conflict.Slug
		if _, ok := want[key]; ok {
			want[key] = true
		}
		if len(conflict.Values) < 2 {
			t.Fatalf("conflict %s has too few values: %#v", key, conflict.Values)
		}
	}
	for key, found := range want {
		if !found {
			t.Fatalf("missing visible conflict %s in %#v", key, conflicts)
		}
	}
}
