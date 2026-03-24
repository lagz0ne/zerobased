package run

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"time"

	"github.com/lagz0ne/zerobased/internal/caddy"
	"github.com/lagz0ne/zerobased/internal/daemon"
	"github.com/lagz0ne/zerobased/internal/docker"
	"github.com/lagz0ne/zerobased/internal/env"
	"github.com/lagz0ne/zerobased/internal/routes"
)

var portPattern = regexp.MustCompile(`https?://(?:localhost|127\.0\.0\.1|0\.0\.0\.0):(\d+)`)

type Options struct {
	Name       string
	Args       []string
	Port       int
	DockerHost string
	EnvPrefix  string
	Profiles   []string
}

func Run(opts Options) error {
	name := opts.Name
	if name == "" {
		dir, _ := os.Getwd()
		name = filepath.Base(dir)
	}

	if len(opts.Args) == 0 {
		return fmt.Errorf("no command specified")
	}

	routeID := fmt.Sprintf("zb-run-%s", name)
	cm := caddy.NewHTTPOnly()

	cwd, _ := os.Getwd()
	rf, err := routes.LoadWithProfile(cwd, opts.Profiles)
	if err != nil {
		log.Printf("warning: routefile: %v", err)
	}

	registerFn := buildRegisterFn(cm, routeID, name, cwd, rf)

	zbEnv := resolveProjectEnv(name, opts.DockerHost, opts.EnvPrefix)

	cmd := exec.Command(opts.Args[0], opts.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if len(zbEnv) > 0 {
		cmd.Env = append(os.Environ(), zbEnv...)
		log.Printf("injected %d env vars for project %q", len(zbEnv), name)
	}

	if opts.Port > 0 {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start %s: %w", opts.Args[0], err)
		}

		registerFn(fmt.Sprintf("localhost:%d", opts.Port))
	} else {
		pr, pw := io.Pipe()
		cmd.Stdout = io.MultiWriter(os.Stdout, pw)
		cmd.Stderr = io.MultiWriter(os.Stderr, pw)

		if err := cmd.Start(); err != nil {
			pw.Close()
			return fmt.Errorf("start %s: %w", opts.Args[0], err)
		}

		portCh := make(chan int, 1)
		go func() {
			defer pw.Close()
			scanner := bufio.NewScanner(pr)
			for scanner.Scan() {
				if m := portPattern.FindStringSubmatch(scanner.Text()); m != nil {
					if p, err := strconv.Atoi(m[1]); err == nil {
						portCh <- p
						io.Copy(io.Discard, pr)
						return
					}
				}
			}
			portCh <- 0
		}()

		select {
		case port := <-portCh:
			if port > 0 {
				registerFn(fmt.Sprintf("localhost:%d", port))
			} else {
				log.Printf("warning: could not detect port — use -p to specify")
			}
		case <-time.After(30 * time.Second):
			log.Printf("warning: port detection timed out after 30s — use -p to specify")
		}
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case sig := <-sigs:
		syscall.Kill(-cmd.Process.Pid, sig.(syscall.Signal))
		<-done
	case <-done:
	}

	signal.Stop(sigs)
	cm.RemoveRoute(routeID)
	return nil
}

func buildRegisterFn(cm *caddy.Manager, routeID, name, cwd string, rf *routes.File) func(upstream string) {
	if rf != nil {
		if entry := rf.FindService(name); entry != nil {
			project := filepath.Base(cwd)
			gateway := fmt.Sprintf("%s.localhost", project)
			group := fmt.Sprintf("zb-gw-%s", project)
			return func(upstream string) {
				if err := cm.AddPathRoute(routeID, gateway, entry.Path, upstream, group); err != nil {
					log.Printf("warning: failed to register path route: %v", err)
				} else {
					log.Printf("→ http://%s%s", gateway, entry.Path)
				}
			}
		}
	}
	hostname := fmt.Sprintf("%s.localhost", name)
	return func(upstream string) {
		if err := cm.AddHTTPRoute(routeID, hostname, upstream); err != nil {
			log.Printf("warning: failed to register route: %v", err)
		} else {
			log.Printf("→ http://%s", hostname)
		}
	}
}

func resolveProjectEnv(project, dockerHost, prefix string) []string {
	dc, err := docker.NewWithHost(dockerHost)
	if err != nil {
		log.Printf("warning: docker unavailable for env injection: %v", err)
		return nil
	}
	defer dc.Close()

	containers, err := dc.ListRunning(context.Background())
	if err != nil {
		log.Printf("warning: failed to list containers for env injection: %v", err)
		return nil
	}

	return env.AsEnvVars(prefix, env.EndpointsFromContainers(daemon.DefaultBaseDir(), containers, project))
}
