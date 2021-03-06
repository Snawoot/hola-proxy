package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Endpoint struct {
	Host    string
	Port    uint16
	TLSName string
}

func (e *Endpoint) URL() *url.URL {
	if e.TLSName == "" {
		return &url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort(e.Host, fmt.Sprintf("%d", e.Port)),
		}
	} else {
		return &url.URL{
			Scheme: "https",
			Host:   net.JoinHostPort(e.TLSName, fmt.Sprintf("%d", e.Port)),
		}
	}
}

func basic_auth_header(login, password string) string {
	return "basic " + base64.StdEncoding.EncodeToString(
		[]byte(login+":"+password))
}

func proxy(ctx context.Context, left, right net.Conn) {
	wg := sync.WaitGroup{}
	cpy := func(dst, src net.Conn) {
		defer wg.Done()
		io.Copy(dst, src)
		dst.Close()
	}
	wg.Add(2)
	go cpy(left, right)
	go cpy(right, left)
	groupdone := make(chan struct{})
	go func() {
		wg.Wait()
		groupdone <- struct{}{}
	}()
	select {
	case <-ctx.Done():
		left.Close()
		right.Close()
	case <-groupdone:
		return
	}
	<-groupdone
	return
}

func print_countries(timeout time.Duration) int {
	var (
		countries CountryList
		err       error
	)
	tx_res, tx_err := EnsureTransaction(context.Background(), timeout, func(ctx context.Context, client *http.Client) bool {
		countries, err = VPNCountries(ctx, client)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Transaction error: %v. Retrying with the fallback mechanism...\n", err)
			return false
		}
		return true
	})
	if tx_err != nil {
		fmt.Fprintf(os.Stderr, "Transaction recovery mechanism failure: %v.\n", tx_err)
		return 4
	}
	if !tx_res {
		fmt.Fprintf(os.Stderr, "All attempts failed.")
		return 3
	}
	for _, code := range countries {
		fmt.Printf("%v - %v\n", code, ISO3166[strings.ToUpper(code)])
	}
	return 0
}

func print_proxies(country string, proxy_type string, limit uint, timeout time.Duration) int {
	var (
		tunnels   *ZGetTunnelsResponse
		user_uuid string
		err       error
	)
	tx_res, tx_err := EnsureTransaction(context.Background(), timeout, func(ctx context.Context, client *http.Client) bool {
		tunnels, user_uuid, err = Tunnels(ctx, client, country, proxy_type, limit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Transaction error: %v. Retrying with the fallback mechanism...\n", err)
			return false
		}
		return true
	})
	if tx_err != nil {
		fmt.Fprintf(os.Stderr, "Transaction recovery mechanism failure: %v.\n", tx_err)
		return 4
	}
	if !tx_res {
		fmt.Fprintf(os.Stderr, "All attempts failed.")
		return 3
	}
	wr := csv.NewWriter(os.Stdout)
	login := LOGIN_PREFIX + user_uuid
	password := tunnels.AgentKey
	fmt.Println("Login:", login)
	fmt.Println("Password:", password)
	fmt.Println("Proxy-Authorization:",
		basic_auth_header(login, password))
	fmt.Println("")
	wr.Write([]string{"host", "ip_address", "direct", "peer", "hola", "trial", "trial_peer", "vendor"})
	for host, ip := range tunnels.IPList {
		if PROTOCOL_WHITELIST[tunnels.Protocol[host]] {
			wr.Write([]string{host,
				ip,
				strconv.FormatUint(uint64(tunnels.Port.Direct), 10),
				strconv.FormatUint(uint64(tunnels.Port.Peer), 10),
				strconv.FormatUint(uint64(tunnels.Port.Hola), 10),
				strconv.FormatUint(uint64(tunnels.Port.Trial), 10),
				strconv.FormatUint(uint64(tunnels.Port.TrialPeer), 10),
				tunnels.Vendor[host]})
		}
	}
	wr.Flush()
	return 0
}

func get_endpoint(tunnels *ZGetTunnelsResponse, typ string, trial bool, force_port_field string) (*Endpoint, error) {
	var hostname, ip string
	for k, v := range tunnels.IPList {
		hostname = k
		ip = v
		break
	}
	if hostname == "" || ip == "" {
		return nil, errors.New("No tunnels found in API response")
	}

	var port uint16
	if force_port_field != "" {
		port2, err := strconv.ParseUint(force_port_field, 0, 16)
		if err == nil {
			port = (uint16)(port2)
			typ = "skip"
		} else {
			typ = force_port_field
		}
	}
	if typ != "skip" {
		if typ == "direct" || typ == "lum" || typ == "pool" || typ == "virt" {
			if !trial {
				port = tunnels.Port.Trial
			} else {
				port = tunnels.Port.Direct
			}
		} else if typ == "peer" {
			if !trial {
				port = tunnels.Port.TrialPeer
			} else {
				port = tunnels.Port.Peer
			}
		} else {
			return nil, errors.New("Unsupported port type")
		}
	}
	return &Endpoint{
		Host:    ip,
		Port:    port,
		TLSName: hostname,
	}, nil
}

// Hop-by-hop headers. These are removed when sent to the backend.
// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
var hopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Connection",
	"Te", // canonicalized version of "TE"
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func delHopHeaders(header http.Header) {
	for _, h := range hopHeaders {
		header.Del(h)
	}
}

func hijack(hijackable interface{}) (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := hijackable.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("Connection doesn't support hijacking")
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return nil, nil, err
	}
	var emptytime time.Time
	err = conn.SetDeadline(emptytime)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	return conn, rw, nil
}

func rewriteConnectReq(req *http.Request, resolver *Resolver) error {
	origHost := req.Host
	origAddr, origPort, err := net.SplitHostPort(origHost)
	if err == nil {
		origHost = origAddr
	}
	addrs := resolver.Resolve(origHost)
	if len(addrs) == 0 {
		return errors.New("Can't resolve host")
	}
	if origPort == "" {
		req.URL.Host = addrs[0]
		req.Host = addrs[0]
		req.RequestURI = addrs[0]
	} else {
		req.URL.Host = net.JoinHostPort(addrs[0], origPort)
		req.Host = net.JoinHostPort(addrs[0], origPort)
		req.RequestURI = net.JoinHostPort(addrs[0], origPort)
	}
	return nil
}

func rewriteReq(req *http.Request, resolver *Resolver) error {
	origHost := req.URL.Host
	origAddr, origPort, err := net.SplitHostPort(origHost)
	if err == nil {
		origHost = origAddr
	}
	addrs := resolver.Resolve(origHost)
	if len(addrs) == 0 {
		return errors.New("Can't resolve host")
	}
	if origPort == "" {
		req.URL.Host = addrs[0]
		req.Host = addrs[0]
	} else {
		req.URL.Host = net.JoinHostPort(addrs[0], origPort)
		req.Host = net.JoinHostPort(addrs[0], origPort)
	}
	req.Header.Set("Host", origHost)
	return nil
}

func makeConnReq(uri string, resolver *Resolver) (*http.Request, error) {
	parsed_url, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	origAddr, origPort, err := net.SplitHostPort(parsed_url.Host)
	if err != nil {
		origAddr = parsed_url.Host
		switch strings.ToLower(parsed_url.Scheme) {
		case "https":
			origPort = "443"
		case "http":
			origPort = "80"
		default:
			return nil, errors.New("Unknown scheme")
		}
	}
	addrs := resolver.Resolve(origAddr)
	if len(addrs) == 0 {
		return nil, errors.New("Can't resolve host")
	}
	new_uri := net.JoinHostPort(addrs[0], origPort)
	req, err := http.NewRequest("CONNECT", "http://"+new_uri, nil)
	if err != nil {
		return nil, err
	}
	req.RequestURI = new_uri
	req.Host = new_uri
	return req, nil
}
