package env

import "testing"

func TestHostname(t *testing.T) {
	got := Hostname("myapp", "api", 3000)
	want := "api-3000.myapp.localhost"
	if got != want {
		t.Errorf("Hostname() = %q, want %q", got, want)
	}
}

func TestHostnameForDomain(t *testing.T) {
	tests := []struct {
		project, service string
		port             uint16
		domain           string
		want             string
	}{
		{"myapp", "api", 3000, "localhost", "api-3000.myapp.localhost"},
		{"myapp", "api", 3000, "preview.dev.co", "api-3000.myapp.preview.dev.co"},
		{"myapp", "frontend", 5173, "box.tailnet.ts.net", "frontend-5173.myapp.box.tailnet.ts.net"},
		{"myapp", "api", 8080, "10.0.1.50.nip.io", "api-8080.myapp.10.0.1.50.nip.io"},
	}
	for _, tt := range tests {
		got := HostnameForDomain(tt.project, tt.service, tt.port, tt.domain)
		if got != tt.want {
			t.Errorf("HostnameForDomain(%q, %q, %d, %q) = %q, want %q",
				tt.project, tt.service, tt.port, tt.domain, got, tt.want)
		}
	}
}

func TestGatewayForDomain(t *testing.T) {
	tests := []struct {
		project, domain, want string
	}{
		{"myapp", "localhost", "myapp.localhost"},
		{"myapp", "preview.dev.co", "myapp.preview.dev.co"},
		{"project", "box.ts.net", "project.box.ts.net"},
	}
	for _, tt := range tests {
		got := GatewayForDomain(tt.project, tt.domain)
		if got != tt.want {
			t.Errorf("GatewayForDomain(%q, %q) = %q, want %q",
				tt.project, tt.domain, got, tt.want)
		}
	}
}
