package composebackend

import (
	"context"
	"os"

	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	composeapi "github.com/docker/compose/v5/pkg/api"
	composepkg "github.com/docker/compose/v5/pkg/compose"
)

type dockerLifecycle struct {
	service composeapi.Compose
}

func newDefaultLifecycle() (Lifecycle, error) {
	dockerCLI, err := command.NewDockerCli()
	if err != nil {
		return nil, err
	}
	if err := dockerCLI.Initialize(&flags.ClientOptions{}); err != nil {
		return nil, err
	}
	service, err := composepkg.NewComposeService(dockerCLI,
		composepkg.WithOutputStream(os.Stdout),
		composepkg.WithErrorStream(os.Stderr),
		composepkg.WithPrompt(composepkg.AlwaysOkPrompt()),
	)
	if err != nil {
		return nil, err
	}
	return dockerLifecycle{service: service}, nil
}

func (lifecycle dockerLifecycle) Up(ctx context.Context, project *Project) error {
	services := project.ServiceNames()
	return lifecycle.service.Up(ctx, project, composeapi.UpOptions{
		Create: composeapi.CreateOptions{
			Services:      services,
			RemoveOrphans: true,
		},
		Start: composeapi.StartOptions{
			Project:  project,
			Services: services,
		},
	})
}

func (lifecycle dockerLifecycle) Down(ctx context.Context, project *Project) error {
	return lifecycle.service.Down(ctx, project.Name, composeapi.DownOptions{
		Project:       project,
		RemoveOrphans: true,
	})
}
