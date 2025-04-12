package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

type chromeVerResponse struct {
	Versions [1]struct {
		Version string `json:"version"`
	} `json:"versions"`
}

const chromeVerURL = "https://versionhistory.googleapis.com/v1/chrome/platforms/win/channels/stable/versions?alt=json&orderBy=version+desc&pageSize=1&prettyPrint=false"

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

	req, err := http.NewRequestWithContext(ctx, "GET", chromeVerURL, nil)
	if err != nil {
		return "", fmt.Errorf("chrome browser version request construction failed: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("chrome browser version request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("chrome browser version request failed: bad status code: %d", resp.StatusCode)
	}

	dec := json.NewDecoder(resp.Body)
	var chromeVerResp chromeVerResponse
	if err := dec.Decode(&chromeVerResp); err != nil {
		return "", fmt.Errorf("unable to decode chrome browser version response: %w", err)
	}

	return chromeVerResp.Versions[0].Version, nil
}
