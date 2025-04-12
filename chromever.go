package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/versionhistory/v1"
)

func GetChromeVer(ctx context.Context, dialer ContextDialer) (string, error) {
	if dialer == nil {
		dialer = &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
	}

	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	defer transport.CloseIdleConnections()
	httpClient := &http.Client{
		Transport: transport,
	}

	versionHistoryService, err := versionhistory.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return "", fmt.Errorf("unable to create version history service: %w", err)
	}

	call := versionHistoryService.Platforms.Channels.Versions.List("chrome/platforms/win/channels/stable")
	call = call.OrderBy("version desc").PageSize(1)
	call.Context(ctx)
	resp, err := call.Do()
	if err != nil {
		return "", fmt.Errorf("call to version history service failed: %w", err)
	}

	if len(resp.Versions) == 0 {
		return "", ErrNoVerData
	}
	return resp.Versions[0].Version, nil
}
