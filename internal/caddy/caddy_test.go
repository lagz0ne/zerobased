package caddy

import "testing"

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
