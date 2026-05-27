package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lagz0ne/zerobased/internal/controlplane"
)

func TestStartCreatesLockSocketAndHealth(t *testing.T) {
	home := t.TempDir()

	result, err := Start(Config{Home: home})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer func() {
		if err := result.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	}()

	assertExists(t, result.LockPath)
	assertExists(t, result.SocketPath)
	assertExists(t, result.HealthPath)
	assertExists(t, filepath.Join(home, "daemon.lock"))
	assertExists(t, filepath.Join(home, "daemon.sock"))
	assertExists(t, filepath.Join(home, "health.json"))
}

func TestConfigFromEnvUsesDefaultRouteRuntime(t *testing.T) {
	t.Setenv("ZEROBASED_ROUTE_RUNTIME_BACKEND", "")
	t.Setenv("ZEROBASED_ROUTE_RUNTIME_ADMIN_URL", "")

	config := ConfigFromEnv(t.TempDir())
	if config.RouteRuntime == nil {
		t.Fatalf("ConfigFromEnv RouteRuntime is nil, want default route runtime")
	}
}

func TestStartRejectsSecondDaemonWhileLockHeld(t *testing.T) {
	home := t.TempDir()

	result, err := Start(Config{Home: home})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer result.Close()

	second, err := Start(Config{Home: home})
	if err == nil {
		second.Close()
		t.Fatalf("second Start returned nil error")
	}
}

func TestStartFailsFastWhenSocketAlreadyExistsWithoutLock(t *testing.T) {
	home := t.TempDir()
	socketPath := filepath.Join(home, "daemon.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen socket: %v", err)
	}
	defer listener.Close()

	result, err := Start(Config{Home: home})
	if err == nil {
		result.Close()
		t.Fatalf("Start returned nil error")
	}
}

func TestPublishRequiresMatchingClaimToken(t *testing.T) {
	publisher := &recordingRoutePublisher{}
	server := &apiServer{
		routeRuntime: publisher,
		claims:       make(map[string]claimRecord),
	}

	publication := controlplane.Publication{
		Name:       "example",
		Host:       "example.localhost",
		ProjectDir: t.TempDir(),
		Routes: []controlplane.Route{{
			Path:    "/",
			Process: "web",
			Port:    3000,
		}},
	}

	response := request(t, server, http.MethodPut, "/routes", publication)
	if response.Code != http.StatusConflict {
		t.Fatalf("unclaimed publish status = %d, want %d", response.Code, http.StatusConflict)
	}
	if publisher.publishes != 0 {
		t.Fatalf("unclaimed publish reached route runtime")
	}

	claimResponse := request(t, server, http.MethodPut, "/claims", controlplane.Claim{
		Name:       publication.Name,
		Host:       publication.Host,
		ProjectDir: publication.ProjectDir,
		Routes:     publication.Routes,
		SessionID:  "session-1",
	})
	if claimResponse.Code != http.StatusOK {
		t.Fatalf("claim status = %d, want %d body=%s", claimResponse.Code, http.StatusOK, claimResponse.Body.String())
	}
	var receipt controlplane.ClaimReceipt
	if err := json.NewDecoder(claimResponse.Body).Decode(&receipt); err != nil {
		t.Fatalf("decode claim receipt: %v", err)
	}

	publication.ClaimToken = "wrong"
	publication.Generation = receipt.Generation
	publication.SessionID = "session-1"
	response = request(t, server, http.MethodPut, "/routes", publication)
	if response.Code != http.StatusConflict {
		t.Fatalf("wrong-token publish status = %d, want %d", response.Code, http.StatusConflict)
	}
	if publisher.publishes != 0 {
		t.Fatalf("wrong-token publish reached route runtime")
	}

	publication.ClaimToken = receipt.Token
	response = request(t, server, http.MethodPut, "/routes", publication)
	if response.Code != http.StatusNoContent {
		t.Fatalf("claimed publish status = %d, want %d body=%s", response.Code, http.StatusNoContent, response.Body.String())
	}
	if publisher.publishes != 1 {
		t.Fatalf("claimed publish calls = %d, want 1", publisher.publishes)
	}
}

func TestReleaseRemovesClaimAndUnpublishes(t *testing.T) {
	publisher := &recordingRoutePublisher{}
	server := &apiServer{
		routeRuntime: publisher,
		claims:       make(map[string]claimRecord),
	}
	projectDir := t.TempDir()
	claim := controlplane.Claim{
		Name:       "example",
		Host:       "example.localhost",
		ProjectDir: projectDir,
		SessionID:  "session-1",
		Routes: []controlplane.Route{{
			Path:    "/",
			Process: "web",
			Port:    3000,
		}},
	}

	claimResponse := request(t, server, http.MethodPut, "/claims", claim)
	var receipt controlplane.ClaimReceipt
	if err := json.NewDecoder(claimResponse.Body).Decode(&receipt); err != nil {
		t.Fatalf("decode claim receipt: %v", err)
	}
	publication := controlplane.Publication{
		Name:       claim.Name,
		Host:       claim.Host,
		ProjectDir: claim.ProjectDir,
		Routes:     claim.Routes,
		ClaimToken: receipt.Token,
		Generation: receipt.Generation,
		SessionID:  claim.SessionID,
	}
	response := request(t, server, http.MethodPut, "/routes", publication)
	if response.Code != http.StatusNoContent {
		t.Fatalf("publish status = %d, want %d body=%s", response.Code, http.StatusNoContent, response.Body.String())
	}

	response = request(t, server, http.MethodDelete, "/claims", controlplane.Release{
		Name:       claim.Name,
		Host:       claim.Host,
		ProjectDir: claim.ProjectDir,
		ClaimToken: receipt.Token,
		Generation: receipt.Generation,
		SessionID:  claim.SessionID,
	})
	if response.Code != http.StatusNoContent {
		t.Fatalf("release status = %d, want %d body=%s", response.Code, http.StatusNoContent, response.Body.String())
	}
	if publisher.unpublishes != 1 {
		t.Fatalf("unpublish calls = %d, want 1", publisher.unpublishes)
	}

	response = request(t, server, http.MethodPut, "/routes", publication)
	if response.Code != http.StatusConflict {
		t.Fatalf("publish after release status = %d, want %d", response.Code, http.StatusConflict)
	}
}

func TestClaimTokenIsNotReusedForSecondSameProjectSession(t *testing.T) {
	server := &apiServer{claims: make(map[string]claimRecord)}
	claim := controlplane.Claim{
		Name:       "example",
		Host:       "example.localhost",
		ProjectDir: t.TempDir(),
		SessionID:  "session-1",
	}

	response := request(t, server, http.MethodPut, "/claims", claim)
	if response.Code != http.StatusOK {
		t.Fatalf("first claim status = %d, want %d", response.Code, http.StatusOK)
	}

	claim.SessionID = "session-2"
	response = request(t, server, http.MethodPut, "/claims", claim)
	if response.Code != http.StatusConflict {
		t.Fatalf("second same-project claim status = %d, want %d", response.Code, http.StatusConflict)
	}
}

func TestClaimHostIsCanonical(t *testing.T) {
	server := &apiServer{claims: make(map[string]claimRecord)}

	response := request(t, server, http.MethodPut, "/claims", controlplane.Claim{
		Name:       "example",
		Host:       "Example.Localhost.",
		ProjectDir: t.TempDir(),
		SessionID:  "session-1",
	})
	if response.Code != http.StatusOK {
		t.Fatalf("first claim status = %d, want %d", response.Code, http.StatusOK)
	}

	response = request(t, server, http.MethodPut, "/claims", controlplane.Claim{
		Name:       "other",
		Host:       "example.localhost",
		ProjectDir: t.TempDir(),
		SessionID:  "session-2",
	})
	if response.Code != http.StatusConflict {
		t.Fatalf("canonical host claim status = %d, want %d", response.Code, http.StatusConflict)
	}
}

func TestReleaseBlocksPublishUntilUnpublishCompletes(t *testing.T) {
	publisher := &recordingRoutePublisher{
		unpublishStarted: make(chan struct{}),
		releaseUnpublish: make(chan struct{}),
	}
	server := &apiServer{
		routeRuntime: publisher,
		claims:       make(map[string]claimRecord),
	}
	claim := controlplane.Claim{
		Name:       "example",
		Host:       "example.localhost",
		ProjectDir: t.TempDir(),
		SessionID:  "session-1",
		Routes: []controlplane.Route{{
			Path:    "/",
			Process: "web",
			Port:    3000,
		}},
	}
	claimResponse := request(t, server, http.MethodPut, "/claims", claim)
	var receipt controlplane.ClaimReceipt
	if err := json.NewDecoder(claimResponse.Body).Decode(&receipt); err != nil {
		t.Fatalf("decode receipt: %v", err)
	}
	publication := controlplane.Publication{
		Name:       claim.Name,
		Host:       claim.Host,
		ProjectDir: claim.ProjectDir,
		SessionID:  claim.SessionID,
		Routes:     claim.Routes,
		ClaimToken: receipt.Token,
		Generation: receipt.Generation,
	}
	if response := request(t, server, http.MethodPut, "/routes", publication); response.Code != http.StatusNoContent {
		t.Fatalf("publish status = %d, want %d", response.Code, http.StatusNoContent)
	}

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- request(t, server, http.MethodDelete, "/claims", controlplane.Release{
			Name:       claim.Name,
			Host:       claim.Host,
			ProjectDir: claim.ProjectDir,
			SessionID:  claim.SessionID,
			ClaimToken: receipt.Token,
			Generation: receipt.Generation,
		})
	}()

	select {
	case <-publisher.unpublishStarted:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for unpublish")
	}

	response := request(t, server, http.MethodPut, "/routes", publication)
	if response.Code != http.StatusConflict {
		t.Fatalf("publish during release status = %d, want %d", response.Code, http.StatusConflict)
	}
	close(publisher.releaseUnpublish)

	select {
	case response := <-done:
		if response.Code != http.StatusNoContent {
			t.Fatalf("release status = %d, want %d", response.Code, http.StatusNoContent)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for release")
	}
}

type recordingRoutePublisher struct {
	publishes        int
	unpublishes      int
	unpublishStarted chan struct{}
	releaseUnpublish chan struct{}
}

func (publisher *recordingRoutePublisher) Publish(ctx context.Context, publication controlplane.Publication) error {
	publisher.publishes++
	return nil
}

func (publisher *recordingRoutePublisher) Unpublish(ctx context.Context, publication controlplane.Publication) error {
	publisher.unpublishes++
	if publisher.unpublishStarted != nil {
		close(publisher.unpublishStarted)
	}
	if publisher.releaseUnpublish != nil {
		select {
		case <-publisher.releaseUnpublish:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func request(t *testing.T, handler http.Handler, method string, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}
