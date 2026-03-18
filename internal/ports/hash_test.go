package ports

import "testing"

func TestDeterministicPort_Stable(t *testing.T) {
	p1 := DeterministicPort("acountee", "nats", 4222)
	p2 := DeterministicPort("acountee", "nats", 4222)
	if p1 != p2 {
		t.Errorf("DeterministicPort not stable: %d != %d", p1, p2)
	}
}

func TestDeterministicPort_Range(t *testing.T) {
	p := DeterministicPort("acountee", "nats", 4222)
	if p < 10000 || p >= 30000 {
		t.Errorf("DeterministicPort = %d, want [10000, 30000)", p)
	}
}

func TestDeterministicPort_DifferentInputs(t *testing.T) {
	p1 := DeterministicPort("acountee", "nats", 4222)
	p2 := DeterministicPort("acountee", "postgres", 5432)
	p3 := DeterministicPort("other-project", "nats", 4222)

	if p1 == p2 {
		t.Error("Different service/port should (usually) produce different hash")
	}
	if p1 == p3 {
		t.Error("Different project should (usually) produce different hash")
	}
}
