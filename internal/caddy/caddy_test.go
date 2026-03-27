package caddy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBuildRoute_HTTPS(t *testing.T) {
	route := buildRoute("zb-test", "myapp.localhost", "/api", "api.staging.com:443", "zb-gw-myapp", true)

	if route["@id"] != "zb-test" {
		t.Errorf("@id = %v, want zb-test", route["@id"])
	}
	if route["group"] != "zb-gw-myapp" {
		t.Errorf("group = %v, want zb-gw-myapp", route["group"])
	}

	match := route["match"].([]map[string]any)
	host := match[0]["host"].([]string)
	if host[0] != "myapp.localhost" {
		t.Errorf("host = %v, want myapp.localhost", host)
	}
	path := match[0]["path"].([]string)
	if path[0] != "/api/*" {
		t.Errorf("path = %v, want /api/*", path)
	}

	handle := route["handle"].([]map[string]any)
	upstreams := handle[0]["upstreams"].([]map[string]string)
	if upstreams[0]["dial"] != "api.staging.com:443" {
		t.Errorf("dial = %v, want api.staging.com:443", upstreams[0]["dial"])
	}

	transport := handle[0]["transport"].(map[string]any)
	if transport["protocol"] != "http" {
		t.Errorf("transport protocol = %v, want http", transport["protocol"])
	}
	if _, ok := transport["tls"].(map[string]any); !ok {
		t.Error("transport.tls missing")
	}
}

func TestBuildRoute_NoTLS(t *testing.T) {
	route := buildRoute("zb-test", "myapp.localhost", "/api", "172.17.0.3:3000", "zb-gw-myapp", false)

	handle := route["handle"].([]map[string]any)
	if _, exists := handle[0]["transport"]; exists {
		t.Error("transport should not be present when tls=false")
	}
}

func TestBuildRoute_CatchAll(t *testing.T) {
	route := buildRoute("zb-test", "myapp.localhost", "/", "frontend.staging.com:443", "zb-gw-myapp", true)

	match := route["match"].([]map[string]any)
	if _, exists := match[0]["path"]; exists {
		t.Error("catch-all should not have path matcher")
	}
}

func TestBuildRoute_PathMatch(t *testing.T) {
	route := buildRoute("zb-test", "myapp.localhost", "/ws", "ws.staging.com:443", "zb-gw-myapp", true)

	match := route["match"].([]map[string]any)
	path := match[0]["path"].([]string)
	if path[0] != "/ws/*" {
		t.Errorf("path = %v, want /ws/*", path)
	}
}

func TestPostRoute_BootstrapsOnInvalidTraversal(t *testing.T) {
	var postAttempts atomic.Int32
	var bootstrapped atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/config/apps/http/servers/zerobased/routes" && r.Method == "POST":
			n := postAttempts.Add(1)
			if n == 1 {
				// First attempt: Caddy has no config tree
				w.WriteHeader(500)
				w.Write([]byte(`{"error":"invalid traversal path at: config/apps"}`))
				return
			}
			// After bootstrap: success
			w.WriteHeader(200)

		case r.URL.Path == "/config/apps/http/servers/zerobased" && r.Method == "PUT":
			w.WriteHeader(500)
			w.Write([]byte(`{"error":"invalid traversal"}`))

		case r.URL.Path == "/load" && r.Method == "POST":
			// Fallback bootstrap via /load
			bootstrapped.Store(true)
			w.WriteHeader(200)

		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	m := &Manager{
		http:    srv.Client(),
		baseURL: srv.URL,
	}

	err := m.AddHTTPRoute("test-id", "test.localhost", "localhost:3000")
	if err != nil {
		t.Fatalf("AddHTTPRoute should succeed after bootstrap, got: %v", err)
	}
	if !bootstrapped.Load() {
		t.Error("expected /load bootstrap to be called")
	}
	if postAttempts.Load() != 2 {
		t.Errorf("expected 2 POST attempts, got %d", postAttempts.Load())
	}
}

func TestPostRoute_NoBootstrapWhenServerExists(t *testing.T) {
	var bootstrapped atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/config/apps/http/servers/zerobased/routes" && r.Method == "POST":
			w.WriteHeader(200)

		case r.URL.Path == "/load" && r.Method == "POST":
			bootstrapped.Store(true)
			w.WriteHeader(200)

		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	m := &Manager{
		http:    srv.Client(),
		baseURL: srv.URL,
	}

	err := m.AddHTTPRoute("test-id", "test.localhost", "localhost:3000")
	if err != nil {
		t.Fatalf("AddHTTPRoute failed: %v", err)
	}
	if bootstrapped.Load() {
		t.Error("should not bootstrap when route POST succeeds directly")
	}
}

func TestEnsureServerConfig_CreateViaPUT(t *testing.T) {
	var putCalled atomic.Bool
	var loadCalled atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/config/apps/http/servers/zerobased" && r.Method == "PUT":
			putCalled.Store(true)
			w.WriteHeader(200)

		case r.URL.Path == "/load" && r.Method == "POST":
			loadCalled.Store(true)
			w.WriteHeader(200)

		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	m := &Manager{
		http:    srv.Client(),
		baseURL: srv.URL,
	}

	if err := m.ensureServerConfig(); err != nil {
		t.Fatalf("ensureServerConfig failed: %v", err)
	}
	if !putCalled.Load() {
		t.Error("expected PUT to be tried first")
	}
	if loadCalled.Load() {
		t.Error("should not fall back to /load when PUT succeeds")
	}
}

func TestPostRoute_BootstrapFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/config/apps/http/servers/zerobased/routes" && r.Method == "POST":
			w.WriteHeader(500)
			w.Write([]byte(`{"error":"invalid traversal path at: config/apps"}`))

		case r.URL.Path == "/config/apps/http/servers/zerobased" && r.Method == "PUT":
			w.WriteHeader(500)

		case r.URL.Path == "/load" && r.Method == "POST":
			w.WriteHeader(500)
			w.Write([]byte(`{"error":"some caddy error"}`))

		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	m := &Manager{
		http:    srv.Client(),
		baseURL: srv.URL,
	}

	err := m.AddHTTPRoute("test-id", "test.localhost", "localhost:3000")
	if err == nil {
		t.Fatal("expected error when bootstrap fails")
	}
	if !strings.Contains(err.Error(), "bootstrap server config") {
		t.Errorf("error should mention bootstrap, got: %v", err)
	}
}
