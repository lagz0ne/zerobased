package controlplane

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/lagz0ne/zerobased/internal/zberr"
)

func TestClientPublishesRoutesOverDaemonSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "daemon.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}
	defer listener.Close()

	received := make(chan Publication, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/routes" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var publication Publication
		if err := json.NewDecoder(r.Body).Decode(&publication); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		received <- publication
		w.WriteHeader(http.StatusNoContent)
	})}
	defer server.Close()
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Errorf("serve daemon socket: %v", err)
		}
	}()

	client := Client{SocketPath: socketPath}
	err = client.Publish(context.Background(), Publication{
		Name: "example",
		Host: "example.localhost",
		Routes: []Route{{
			Path:    "/",
			Process: "web",
			Port:    3000,
		}},
	})
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	publication := <-received
	if publication.Name != "example" {
		t.Fatalf("name = %q, want example", publication.Name)
	}
}

func TestClientReturnsConnectionFailedWhenSocketMissing(t *testing.T) {
	client := Client{SocketPath: filepath.Join(t.TempDir(), "missing.sock")}

	err := client.Publish(context.Background(), Publication{Name: "example"})
	if err == nil {
		t.Fatalf("Publish returned nil error")
	}
	if !os.IsNotExist(err) && !IsConnectionFailed(err) {
		t.Fatalf("error = %v, want connection failure", err)
	}
}

func TestClientDecodesDaemonErrorEnvelope(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "daemon.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}
	defer listener.Close()

	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(ErrorResponse{
			Layer:   string(zberr.LayerControlPlane),
			Code:    string(zberr.CodeHostClaimConflict),
			Owner:   "/tmp/owner",
			Message: "host already claimed",
		})
	})}
	defer server.Close()
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Errorf("serve daemon socket: %v", err)
		}
	}()

	client := Client{SocketPath: socketPath}
	_, err = client.Claim(context.Background(), Claim{Name: "example", Host: "example.localhost"})
	if err == nil {
		t.Fatalf("Claim returned nil error")
	}
	if !zberr.Is(err, zberr.LayerControlPlane, zberr.CodeHostClaimConflict) {
		t.Fatalf("error = %v, want control-plane host conflict", err)
	}
}
