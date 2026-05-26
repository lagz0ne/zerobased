package route_runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/lagz0ne/zerobased/internal/controlplane"
	"github.com/lagz0ne/zerobased/internal/zberr"
)

type AdminURL struct {
	URL    string
	Client *http.Client
}

func (adapter AdminURL) Publish(ctx context.Context, publication controlplane.Publication) error {
	return adapter.send(ctx, http.MethodPut, publication)
}

func (adapter AdminURL) Unpublish(ctx context.Context, publication controlplane.Publication) error {
	return adapter.send(ctx, http.MethodDelete, publication)
}

func (adapter AdminURL) send(ctx context.Context, method string, publication controlplane.Publication) error {
	if adapter.URL == "" {
		return zberr.New(zberr.LayerRouteRuntime, zberr.CodeBootstrapFailed, zberr.WithCause(fmt.Errorf("admin URL is required")))
	}

	body, err := json.Marshal(publication)
	if err != nil {
		return zberr.Critical(zberr.LayerRouteRuntime, err)
	}

	httpClient := adapter.Client
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(adapter.URL, "/")+"/routes", bytes.NewReader(body))
	if err != nil {
		return zberr.Critical(zberr.LayerRouteRuntime, err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := httpClient.Do(request)
	if err != nil {
		return zberr.New(zberr.LayerRouteRuntime, zberr.CodeReloadFailed, zberr.WithCause(err))
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return zberr.New(zberr.LayerRouteRuntime, zberr.CodeApplyRejected, zberr.WithCause(fmt.Errorf("route runtime returned %s", response.Status)))
	}
	return nil
}
