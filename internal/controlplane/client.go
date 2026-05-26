package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/lagz0ne/zerobased/internal/zberr"
)

type Client struct {
	SocketPath string
}

func (client Client) Claim(ctx context.Context, claim Claim) (ClaimReceipt, error) {
	var receipt ClaimReceipt
	if err := client.request(ctx, http.MethodPut, "/claims", claim, &receipt); err != nil {
		return ClaimReceipt{}, err
	}
	return receipt, nil
}

func (client Client) Publish(ctx context.Context, publication Publication) error {
	return client.request(ctx, http.MethodPut, "/routes", publication, nil)
}

func (client Client) Release(ctx context.Context, release Release) error {
	return client.request(ctx, http.MethodDelete, "/claims", release, nil)
}

func (client Client) request(ctx context.Context, method string, path string, payload any, responsePayload any) error {
	if client.SocketPath == "" {
		return zberr.New(zberr.LayerControlPlaneClient, zberr.CodeConnectionFailed, zberr.WithCause(fmt.Errorf("daemon socket path is required")))
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return zberr.Critical(zberr.LayerControlPlaneClient, err)
	}

	httpClient := http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", client.SocketPath)
		},
	}}
	defer httpClient.CloseIdleConnections()

	request, err := http.NewRequestWithContext(ctx, method, "http://zerobased"+path, bytes.NewReader(body))
	if err != nil {
		return zberr.Critical(zberr.LayerControlPlaneClient, err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := httpClient.Do(request)
	if err != nil {
		return zberr.New(zberr.LayerControlPlaneClient, zberr.CodeConnectionFailed, zberr.WithCause(err))
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return decodeErrorResponse(response)
	}
	if responsePayload != nil {
		if err := json.NewDecoder(response.Body).Decode(responsePayload); err != nil && err != io.EOF {
			return zberr.Critical(zberr.LayerControlPlaneClient, err)
		}
	}
	return nil
}

func decodeErrorResponse(response *http.Response) error {
	var payload ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil || payload.Layer == "" || payload.Code == "" {
		return zberr.Critical(zberr.LayerControlPlaneClient, fmt.Errorf("daemon returned %s", response.Status))
	}
	return zberr.New(
		zberr.Layer(payload.Layer),
		zberr.Code(payload.Code),
		zberr.WithOwner(payload.Owner),
		zberr.WithCause(fmt.Errorf("%s", payload.Message)),
	)
}

func IsConnectionFailed(err error) bool {
	return zberr.Is(err, zberr.LayerControlPlaneClient, zberr.CodeConnectionFailed)
}
