package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

var (
	defaultProdVersion = "113.0"
)

type StoreExtUpdateResponse struct {
	XMLName xml.Name `xml:"gupdate"`
	App     *struct {
		AppID       string `xml:"appid,attr"`
		Status      string `xml:"status,attr"`
		UpdateCheck *struct {
			Version string `xml:"version,attr"`
			Status  string `xml:"status,attr"`
		} `xml:"updatecheck"`
	} `xml:"app"`
}

func GetExtVer(ctx context.Context,
	prodVersion *string,
	id string,
	dialer ContextDialer,
) (string, error) {
	if prodVersion == nil {
		prodVersion = &defaultProdVersion
	}
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

	reqURL := (&url.URL{
		Scheme: "https",
		Host:   "clients2.google.com",
		Path:   "/service/update2/crx",
		RawQuery: url.Values{
			"prodversion":  {*prodVersion},
			"acceptformat": {"crx2,crx3"},
			"x": {url.Values{
				"id": {id},
				"uc": {""},
			}.Encode()},
		}.Encode(),
	}).String()

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("chrome web store request construction failed: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("chrome web store request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	reader := io.LimitReader(resp.Body, 64*1024)
	var respData StoreExtUpdateResponse

	dec := xml.NewDecoder(reader)
	err = dec.Decode(&respData)
	if err != nil {
		return "", fmt.Errorf("unmarshaling of chrome web store response failed: %w", err)
	}

	return respData.App.UpdateCheck.Version, nil
}
