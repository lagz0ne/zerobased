package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lagz0ne/zerobased/internal/controlplane"
	route_runtime "github.com/lagz0ne/zerobased/internal/route_runtime"
	"github.com/lagz0ne/zerobased/internal/zberr"
)

type RouteRuntime interface {
	Publish(context.Context, controlplane.Publication) error
	Unpublish(context.Context, controlplane.Publication) error
}

type bootstrappingRouteRuntime interface {
	Bootstrap(context.Context) error
}

type closingRouteRuntime interface {
	Close(context.Context) error
}

type Config struct {
	Home         string
	RouteRuntime RouteRuntime
}

type StartResult struct {
	LockPath   string
	SocketPath string
	HealthPath string
	listener   net.Listener
	server     *http.Server
	lockFile   *os.File
	runtime    closingRouteRuntime
}

func ConfigFromEnv(home string) Config {
	config := Config{Home: home}
	if os.Getenv("ZEROBASED_ROUTE_RUNTIME_BACKEND") == "admin-url" {
		config.RouteRuntime = route_runtime.AdminURL{URL: os.Getenv("ZEROBASED_ROUTE_RUNTIME_ADMIN_URL")}
		return config
	}
	config.RouteRuntime = route_runtime.NewDockerCaddy(route_runtime.DockerCaddyOptions{Home: home})
	return config
}

func Start(config Config) (StartResult, error) {
	home := config.Home
	if home == "" {
		return StartResult{}, zberr.New(zberr.LayerDaemonIPC, zberr.CodeSocketBindFailed, zberr.WithCause(fmt.Errorf("home is required")))
	}

	if err := os.MkdirAll(home, 0o755); err != nil {
		return StartResult{}, zberr.New(zberr.LayerDaemonIPC, zberr.CodeSocketBindFailed, zberr.WithCause(err))
	}

	result := StartResult{
		LockPath:   filepath.Join(home, "daemon.lock"),
		SocketPath: filepath.Join(home, "daemon.sock"),
		HealthPath: filepath.Join(home, "health.json"),
	}

	lockFile, err := acquireDaemonLock(result.LockPath)
	if err != nil {
		return StartResult{}, err
	}
	result.lockFile = lockFile

	if socketIsLive(result.SocketPath) {
		_ = result.Close()
		return StartResult{}, zberr.New(zberr.LayerDaemonIPC, zberr.CodeSocketBindFailed, zberr.WithCause(fmt.Errorf("daemon socket already exists at %s", result.SocketPath)))
	}
	if err := os.Remove(result.SocketPath); err != nil && !os.IsNotExist(err) {
		_ = result.Close()
		return StartResult{}, zberr.New(zberr.LayerDaemonIPC, zberr.CodeSocketBindFailed, zberr.WithCause(err))
	}

	if runtime, ok := config.RouteRuntime.(bootstrappingRouteRuntime); ok {
		bootstrapCtx, cancelBootstrap := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancelBootstrap()
		if err := runtime.Bootstrap(bootstrapCtx); err != nil {
			_ = result.Close()
			return StartResult{}, zberr.New(zberr.LayerControlPlane, zberr.CodeBootFailed, zberr.WithCause(err))
		}
	}
	if runtime, ok := config.RouteRuntime.(closingRouteRuntime); ok {
		result.runtime = runtime
	}

	listener, err := net.Listen("unix", result.SocketPath)
	if err != nil {
		_ = result.Close()
		return StartResult{}, zberr.New(zberr.LayerDaemonIPC, zberr.CodeSocketBindFailed, zberr.WithCause(err))
	}
	result.listener = listener

	health := map[string]string{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}
	payload, err := json.MarshalIndent(health, "", "  ")
	if err != nil {
		return StartResult{}, zberr.Critical(zberr.LayerDaemonIPC, err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(result.HealthPath, payload, 0o644); err != nil {
		_ = result.Close()
		return StartResult{}, zberr.New(zberr.LayerDaemonIPC, zberr.CodeHealthCheckFailed, zberr.WithCause(err))
	}

	server := &http.Server{Handler: &apiServer{
		routeRuntime: config.RouteRuntime,
		claims:       make(map[string]claimRecord),
	}}
	result.server = server
	go func() {
		_ = server.Serve(listener)
	}()

	return result, nil
}

func (result StartResult) Close() error {
	var closeErr error
	if result.server != nil {
		if err := result.server.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			closeErr = err
		}
	}
	if result.listener != nil {
		if err := result.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) && closeErr == nil {
			closeErr = err
		}
	}
	if result.runtime != nil {
		closeCtx, cancelClose := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelClose()
		if err := result.runtime.Close(closeCtx); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	if err := closeLock(result.lockFile); err != nil && closeErr == nil {
		closeErr = err
	}
	return closeErr
}

type apiServer struct {
	routeRuntime RouteRuntime
	mu           sync.Mutex
	claims       map[string]claimRecord
	generation   int64
}

type claimRecord struct {
	Claim       controlplane.Claim
	Token       string
	Generation  int64
	Published   bool
	Releasing   bool
	Publication controlplane.Publication
}

func (server *apiServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/health":
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPut && r.URL.Path == "/claims":
		server.handleClaim(w, r)
	case r.Method == http.MethodPut && r.URL.Path == "/routes":
		server.handleRoutes(w, r)
	case r.Method == http.MethodDelete && r.URL.Path == "/claims":
		server.handleRelease(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (server *apiServer) handleClaim(w http.ResponseWriter, r *http.Request) {
	var claim controlplane.Claim
	if err := json.NewDecoder(r.Body).Decode(&claim); err != nil {
		writeZBError(w, http.StatusBadRequest, zberr.LayerDaemonIPC, zberr.CodeProtocolDecodeFailed, "", "decode claim")
		return
	}
	if claim.Host == "" {
		writeZBError(w, http.StatusBadRequest, zberr.LayerControlPlane, zberr.CodeHostClaimConflict, "", "claim host required")
		return
	}
	if claim.SessionID == "" {
		writeZBError(w, http.StatusBadRequest, zberr.LayerControlPlane, zberr.CodeLeaseRejected, "", "claim session required")
		return
	}
	claim.Host = canonicalHost(claim.Host)

	server.mu.Lock()
	defer server.mu.Unlock()
	if existing, ok := server.claims[claim.Host]; ok {
		if existing.Claim.ProjectDir == claim.ProjectDir && existing.Claim.SessionID == claim.SessionID && !existing.Releasing {
			writeJSON(w, http.StatusOK, controlplane.ClaimReceipt{Token: existing.Token, Generation: existing.Generation})
			return
		}
		writeZBError(w, http.StatusConflict, zberr.LayerControlPlane, zberr.CodeHostClaimConflict, existing.Claim.ProjectDir, "host already claimed")
		return
	}

	server.generation++
	record := claimRecord{
		Claim:      claim,
		Token:      newClaimToken(),
		Generation: server.generation,
	}
	server.claims[claim.Host] = record
	writeJSON(w, http.StatusOK, controlplane.ClaimReceipt{Token: record.Token, Generation: record.Generation})
}

func (server *apiServer) handleRoutes(w http.ResponseWriter, r *http.Request) {
	if server.routeRuntime == nil {
		writeZBError(w, http.StatusServiceUnavailable, zberr.LayerControlPlane, zberr.CodeRouteRuntimeUnavailable, "", "route runtime not configured")
		return
	}

	var publication controlplane.Publication
	if err := json.NewDecoder(r.Body).Decode(&publication); err != nil {
		writeZBError(w, http.StatusBadRequest, zberr.LayerDaemonIPC, zberr.CodeProtocolDecodeFailed, "", "decode publication")
		return
	}
	publication.Host = canonicalHost(publication.Host)

	server.mu.Lock()
	record, ok := server.claims[publication.Host]
	if !ok || record.Releasing || !recordMatchesPublication(record, publication) {
		server.mu.Unlock()
		writeZBError(w, http.StatusConflict, zberr.LayerControlPlane, zberr.CodeStaleTokenRejected, "", "publication does not match an active claim")
		return
	}
	if err := server.routeRuntime.Publish(r.Context(), publication); err != nil {
		server.mu.Unlock()
		writeZBError(w, http.StatusBadGateway, zberr.LayerControlPlane, zberr.CodePublicationRejected, "", err.Error())
		return
	}
	record.Published = true
	record.Publication = publication
	server.claims[publication.Host] = record
	server.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (server *apiServer) handleRelease(w http.ResponseWriter, r *http.Request) {
	var release controlplane.Release
	if err := json.NewDecoder(r.Body).Decode(&release); err != nil {
		writeZBError(w, http.StatusBadRequest, zberr.LayerDaemonIPC, zberr.CodeProtocolDecodeFailed, "", "decode release")
		return
	}
	release.Host = canonicalHost(release.Host)

	server.mu.Lock()
	record, ok := server.claims[release.Host]
	if !ok || record.Releasing || !recordMatchesRelease(record, release) {
		server.mu.Unlock()
		writeZBError(w, http.StatusConflict, zberr.LayerControlPlane, zberr.CodeStaleTokenRejected, "", "release does not match an active claim")
		return
	}
	record.Releasing = true
	server.claims[release.Host] = record
	server.mu.Unlock()

	if record.Published && server.routeRuntime != nil {
		if err := server.routeRuntime.Unpublish(r.Context(), record.Publication); err != nil {
			server.mu.Lock()
			if current, ok := server.claims[release.Host]; ok && recordMatchesRelease(current, release) {
				current.Releasing = false
				server.claims[release.Host] = current
			}
			server.mu.Unlock()
			writeZBError(w, http.StatusBadGateway, zberr.LayerControlPlane, zberr.CodePublicationRejected, "", err.Error())
			return
		}
	}

	server.mu.Lock()
	if current, ok := server.claims[release.Host]; ok && recordMatchesRelease(current, release) {
		delete(server.claims, release.Host)
	}
	server.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func recordMatchesPublication(record claimRecord, publication controlplane.Publication) bool {
	if record.Token != publication.ClaimToken || record.Generation != publication.Generation {
		return false
	}
	if record.Claim.ProjectDir != publication.ProjectDir || record.Claim.Name != publication.Name || record.Claim.Host != publication.Host || record.Claim.SessionID != publication.SessionID {
		return false
	}
	return routesEqual(record.Claim.Routes, publication.Routes)
}

func recordMatchesRelease(record claimRecord, release controlplane.Release) bool {
	return record.Token == release.ClaimToken &&
		record.Generation == release.Generation &&
		record.Claim.ProjectDir == release.ProjectDir &&
		record.Claim.SessionID == release.SessionID &&
		record.Claim.Name == release.Name &&
		record.Claim.Host == release.Host
}

func routesEqual(left []controlplane.Route, right []controlplane.Route) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeZBError(w http.ResponseWriter, status int, layer zberr.Layer, code zberr.Code, owner string, message string) {
	writeJSON(w, status, controlplane.ErrorResponse{
		Layer:   string(layer),
		Code:    string(code),
		Owner:   owner,
		Message: message,
	})
}

func newClaimToken() string {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(token[:])
}

func acquireDaemonLock(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, zberr.New(zberr.LayerDaemonIPC, zberr.CodeSocketBindFailed, zberr.WithCause(err))
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, zberr.New(zberr.LayerDaemonIPC, zberr.CodeSocketBindFailed, zberr.WithCause(fmt.Errorf("daemon lock held at %s: %w", path, err)))
	}
	if err := file.Truncate(0); err != nil {
		_ = closeLock(file)
		return nil, zberr.New(zberr.LayerDaemonIPC, zberr.CodeSocketBindFailed, zberr.WithCause(err))
	}
	if _, err := file.Seek(0, 0); err != nil {
		_ = closeLock(file)
		return nil, zberr.New(zberr.LayerDaemonIPC, zberr.CodeSocketBindFailed, zberr.WithCause(err))
	}
	if _, err := fmt.Fprintf(file, "pid=%d\n", os.Getpid()); err != nil {
		_ = closeLock(file)
		return nil, zberr.New(zberr.LayerDaemonIPC, zberr.CodeSocketBindFailed, zberr.WithCause(err))
	}
	return file, nil
}

func closeLock(file *os.File) error {
	if file == nil {
		return nil
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func socketIsLive(path string) bool {
	conn, err := net.DialTimeout("unix", path, 100*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func canonicalHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}
