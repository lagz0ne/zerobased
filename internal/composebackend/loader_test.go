package composebackend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lagz0ne/zerobased/internal/zberr"
)

func TestLoadRejectsMissingExplicitComposeFiles(t *testing.T) {
	project := t.TempDir()
	writeFile(t, filepath.Join(project, "compose.yaml"), `services:
  postgres:
    image: postgres:18
`)

	_, err := Load(context.Background(), Request{
		ProjectDir: project,
		StackName:  "example",
		Profile:    "dev",
		Ownership:  OwnershipOwned,
	})
	if err == nil {
		t.Fatalf("Load returned nil error")
	}
	if !zberr.Is(err, zberr.LayerComposeBackend, zberr.CodeModelInvalid) {
		t.Fatalf("error = %v, want compose backend ModelInvalid", err)
	}
}

func TestLoadOwnsComposeProjectIdentityAndLabels(t *testing.T) {
	project := t.TempDir()
	t.Setenv("COMPOSE_PROJECT_NAME", "from-shell")
	writeFile(t, filepath.Join(project, ".env"), "COMPOSE_PROJECT_NAME=from-dotenv\n")
	writeFile(t, filepath.Join(project, "compose.yaml"), `name: from-compose
services:
  postgres:
    image: postgres:18
`)

	loaded, err := Load(context.Background(), Request{
		ProjectDir: project,
		StackName:  "example",
		Profile:    "backend",
		SessionID:  "session-1",
		Files:      []string{"compose.yaml"},
		Services:   []string{"postgres"},
		Ownership:  OwnershipOwned,
	})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	wantName := ProjectName("example", "backend", project)
	if loaded.Project.Name != wantName {
		t.Fatalf("project name = %q, want %q", loaded.Project.Name, wantName)
	}
	service, err := loaded.Project.GetService("postgres")
	if err != nil {
		t.Fatalf("get postgres service: %v", err)
	}
	wantLabels := map[string]string{
		"dev.zerobased.project":         wantName,
		"dev.zerobased.stack":           "example",
		"dev.zerobased.profile":         "backend",
		"dev.zerobased.session":         "session-1",
		"dev.zerobased.ownership":       string(OwnershipOwned),
		"dev.zerobased.project_root":    project,
		"dev.zerobased.compose_backend": "true",
	}
	for key, want := range wantLabels {
		if got := service.Labels[key]; got != want {
			t.Fatalf("label %s = %q, want %q", key, got, want)
		}
	}
	for key := range service.Labels {
		if strings.HasPrefix(key, "com.docker.compose.") {
			t.Fatalf("zerobased wrote reserved compose label %q", key)
		}
	}
}

func TestLoadUsesDotEnvForNormalInterpolationWithoutProjectNameOwnership(t *testing.T) {
	project := t.TempDir()
	writeFile(t, filepath.Join(project, ".env"), "POSTGRES_IMAGE=postgres:18\nCOMPOSE_PROJECT_NAME=from-dotenv\n")
	writeFile(t, filepath.Join(project, "compose.yaml"), `services:
  postgres:
    image: ${POSTGRES_IMAGE}
`)

	loaded, err := Load(context.Background(), Request{
		ProjectDir: project,
		StackName:  "example",
		Profile:    "backend",
		Files:      []string{"compose.yaml"},
		Services:   []string{"postgres"},
		Ownership:  OwnershipOwned,
	})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	service, err := loaded.Project.GetService("postgres")
	if err != nil {
		t.Fatalf("get postgres service: %v", err)
	}
	if service.Image != "postgres:18" {
		t.Fatalf("image = %q, want postgres:18", service.Image)
	}
	if loaded.Project.Name != ProjectName("example", "backend", project) {
		t.Fatalf("project name = %q, want generated", loaded.Project.Name)
	}
}

func TestLoadRejectsContainerName(t *testing.T) {
	project := t.TempDir()
	writeFile(t, filepath.Join(project, "compose.yaml"), `services:
  postgres:
    image: postgres:18
    container_name: global-postgres
`)

	_, err := Load(context.Background(), ownedRequest(project))
	if err == nil {
		t.Fatalf("Load returned nil error")
	}
	if !zberr.Is(err, zberr.LayerComposeBackend, zberr.CodeIsolationRejected) {
		t.Fatalf("error = %v, want compose backend IsolationRejected", err)
	}
}

func TestLoadRejectsHostPorts(t *testing.T) {
	project := t.TempDir()
	writeFile(t, filepath.Join(project, "compose.yaml"), `services:
  postgres:
    image: postgres:18
    ports:
      - "5432:5432"
`)

	_, err := Load(context.Background(), ownedRequest(project))
	if err == nil {
		t.Fatalf("Load returned nil error")
	}
	if !zberr.Is(err, zberr.LayerComposeBackend, zberr.CodeIsolationRejected) {
		t.Fatalf("error = %v, want compose backend IsolationRejected", err)
	}
}

func TestLoadRejectsHostNetworkingAndCustomResourceNames(t *testing.T) {
	tests := []struct {
		name    string
		compose string
	}{
		{
			name: "host network mode",
			compose: `services:
  app:
    image: busybox
    network_mode: host
`,
		},
		{
			name: "custom network name",
			compose: `services:
  app:
    image: busybox
    networks: [dbnet]
networks:
  dbnet:
    name: global-dbnet
`,
		},
		{
			name: "external volume",
			compose: `services:
  app:
    image: busybox
    volumes:
      - data:/data
volumes:
  data:
    external: true
`,
		},
		{
			name: "external volume object",
			compose: `services:
  app:
    image: busybox
    volumes:
      - data:/data
volumes:
  data:
    external:
      name: global-data
`,
		},
		{
			name: "custom secret name",
			compose: `services:
  app:
    image: busybox
    secrets: [api-key]
secrets:
  api-key:
    name: global-api-key
    file: ./api-key.txt
`,
		},
		{
			name: "external config",
			compose: `services:
  app:
    image: busybox
    configs: [app-config]
configs:
  app-config:
    external: true
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := t.TempDir()
			writeFile(t, filepath.Join(project, "compose.yaml"), tt.compose)

			_, err := Load(context.Background(), ownedRequest(project))
			if err == nil {
				t.Fatalf("Load returned nil error")
			}
			if !zberr.Is(err, zberr.LayerComposeBackend, zberr.CodeIsolationRejected) {
				t.Fatalf("error = %v, want compose backend IsolationRejected", err)
			}
		})
	}
}

func TestLoadSelectsServicesAndPrunesInactiveProfileResources(t *testing.T) {
	project := t.TempDir()
	writeFile(t, filepath.Join(project, "compose.yaml"), `services:
  postgres:
    image: postgres:18
  debug:
    image: busybox
    profiles: ["debug"]
    networks: [debugnet]
networks:
  debugnet:
    name: global-debugnet
`)

	loaded, err := Load(context.Background(), Request{
		ProjectDir: project,
		StackName:  "example",
		Profile:    "backend",
		Files:      []string{"compose.yaml"},
		Services:   []string{"postgres"},
		Ownership:  OwnershipOwned,
	})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got, want := loaded.Project.ServiceNames(), []string{"postgres"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("service names = %v, want %v", got, want)
	}
	for _, network := range loaded.Project.NetworkNames() {
		if network == "debugnet" {
			t.Fatalf("inactive profile network was not pruned")
		}
	}
}

func TestBackendStartRunsComposeLifecycleAndStopRunsDown(t *testing.T) {
	project := t.TempDir()
	writeFile(t, filepath.Join(project, "compose.yaml"), `services:
  postgres:
    image: postgres:18
`)
	lifecycle := &recordingLifecycle{}
	backend := Backend{lifecycle: lifecycle}

	stack, err := backend.Start(context.Background(), ownedRequest(project))
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if lifecycle.ups != 1 {
		t.Fatalf("up calls = %d, want 1", lifecycle.ups)
	}
	if lifecycle.upProject == nil || lifecycle.upProject.Name != ProjectName("example", "backend", project) {
		t.Fatalf("up project name = %#v", lifecycle.upProject)
	}

	if err := stack.Stop(context.Background()); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	if lifecycle.downs != 1 {
		t.Fatalf("down calls = %d, want 1", lifecycle.downs)
	}
	if lifecycle.downProject == nil || lifecycle.downProject.Name != lifecycle.upProject.Name {
		t.Fatalf("down project = %#v, want project %q", lifecycle.downProject, lifecycle.upProject.Name)
	}
}

func TestBackendMapsComposeLifecycleFailure(t *testing.T) {
	project := t.TempDir()
	writeFile(t, filepath.Join(project, "compose.yaml"), `services:
  postgres:
    image: postgres:18
`)
	backend := Backend{lifecycle: &recordingLifecycle{upErr: fmt.Errorf("docker unavailable")}}

	_, err := backend.Start(context.Background(), ownedRequest(project))
	if err == nil {
		t.Fatalf("Start returned nil error")
	}
	if !zberr.Is(err, zberr.LayerComposeBackend, zberr.CodeLifecycleFailed) {
		t.Fatalf("error = %v, want compose backend LifecycleFailed", err)
	}
}

func TestBackendRollsBackWhenComposeUpFails(t *testing.T) {
	project := t.TempDir()
	writeFile(t, filepath.Join(project, "compose.yaml"), `services:
  postgres:
    image: postgres:18
`)
	lifecycle := &recordingLifecycle{upErr: fmt.Errorf("partial create failed")}
	backend := Backend{lifecycle: lifecycle}

	_, err := backend.Start(context.Background(), ownedRequest(project))
	if err == nil {
		t.Fatalf("Start returned nil error")
	}
	if lifecycle.downs != 1 {
		t.Fatalf("down calls = %d, want 1 rollback", lifecycle.downs)
	}
	if lifecycle.downProject == nil || lifecycle.downProject.Name != ProjectName("example", "backend", project) {
		t.Fatalf("down project = %#v, want generated project rollback", lifecycle.downProject)
	}
}

func ownedRequest(project string) Request {
	return Request{
		ProjectDir: project,
		StackName:  "example",
		Profile:    "backend",
		Files:      []string{"compose.yaml"},
		Ownership:  OwnershipOwned,
	}
}

type recordingLifecycle struct {
	ups         int
	downs       int
	upProject   *Project
	downProject *Project
	upErr       error
	downErr     error
}

func (lifecycle *recordingLifecycle) Up(ctx context.Context, project *Project) error {
	lifecycle.ups++
	lifecycle.upProject = project
	return lifecycle.upErr
}

func (lifecycle *recordingLifecycle) Down(ctx context.Context, project *Project) error {
	lifecycle.downs++
	lifecycle.downProject = project
	return lifecycle.downErr
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
