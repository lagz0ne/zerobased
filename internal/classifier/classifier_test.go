package classifier

import "testing"

func TestClassify_WellKnownSockets(t *testing.T) {
	tests := []struct {
		port uint16
	}{
		{5432},  // Postgres
		{3306},  // MySQL
		{6379},  // Redis
		{27017}, // MongoDB
		{4317},  // OTEL gRPC
	}
	for _, tt := range tests {
		if got := Classify(tt.port, ""); got != Socket {
			t.Errorf("Classify(%d) = %s, want socket", tt.port, got)
		}
	}
}

func TestClassify_WellKnownHTTP(t *testing.T) {
	tests := []uint16{80, 443, 3000, 4318, 5173, 8000, 8025, 8080, 8222, 8443, 8888, 9090}
	for _, port := range tests {
		if got := Classify(port, ""); got != HTTP {
			t.Errorf("Classify(%d) = %s, want http", port, got)
		}
	}
}

func TestClassify_InternalPorts(t *testing.T) {
	tests := []uint16{6222, 9222, 7946, 2377, 2380}
	for _, port := range tests {
		if got := Classify(port, ""); got != Internal {
			t.Errorf("Classify(%d) = %s, want internal", port, got)
		}
	}
}

func TestClassify_UnknownPort(t *testing.T) {
	if got := Classify(4222, ""); got != Port {
		t.Errorf("Classify(4222) = %s, want port", got)
	}
}

func TestClassify_LabelOverride(t *testing.T) {
	if got := Classify(5432, "http"); got != HTTP {
		t.Errorf("Classify(5432, 'http') = %s, want http (label override)", got)
	}
	if got := Classify(80, "socket"); got != Socket {
		t.Errorf("Classify(80, 'socket') = %s, want socket (label override)", got)
	}
}

func TestSocketFilename_Postgres(t *testing.T) {
	if got := SocketFilename("pg", 5432); got != ".s.PGSQL.5432" {
		t.Errorf("SocketFilename('pg', 5432) = %s, want .s.PGSQL.5432", got)
	}
}

func TestSocketFilename_Generic(t *testing.T) {
	if got := SocketFilename("redis", 6379); got != "redis-6379.sock" {
		t.Errorf("SocketFilename('redis', 6379) = %s, want redis-6379.sock", got)
	}
}
