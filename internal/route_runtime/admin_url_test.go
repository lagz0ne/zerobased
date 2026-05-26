package route_runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lagz0ne/zerobased/internal/controlplane"
)

func TestAdminURLPublishesRoutesWithPut(t *testing.T) {
	var method string
	var path string
	var publication controlplane.Publication

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&publication); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	adapter := AdminURL{URL: server.URL}
	err := adapter.Publish(context.Background(), controlplane.Publication{
		Name: "example",
		Host: "example.localhost",
		Routes: []controlplane.Route{{
			Path:    "/",
			Process: "web",
			Port:    3000,
		}},
	})
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	if method != http.MethodPut {
		t.Fatalf("method = %q, want %q", method, http.MethodPut)
	}
	if path != "/routes" {
		t.Fatalf("path = %q, want /routes", path)
	}
	if publication.Host != "example.localhost" {
		t.Fatalf("host = %q, want example.localhost", publication.Host)
	}
}
