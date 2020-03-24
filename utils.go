package main

import (
    "fmt"
    "context"
    "net"
    "sync"
    "io"
    "os"
    "time"
    "encoding/base64"
    "encoding/csv"
    "errors"
    "strings"
    "strconv"
    "net/http"
)

func basic_auth_header(login, password string) string {
    return "basic " + base64.StdEncoding.EncodeToString(
        []byte(login + ":" + password))
}

func proxy(ctx context.Context, left, right net.Conn) {
    wg := sync.WaitGroup{}
    cpy := func (dst, src net.Conn) {
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
        groupdone <-struct{}{}
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
    ctx, _ := context.WithTimeout(context.Background(), timeout)
    countries, err := VPNCountries(ctx)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        return 3
    }
    for _, code := range countries {
        fmt.Printf("%v - %v\n", code, ISO3166[strings.ToUpper(code)])
    }
    return 0
}

func print_proxies(country string, limit uint, timeout time.Duration) int {
    ctx, _ := context.WithTimeout(context.Background(), timeout)
    tunnels, user_uuid, err := Tunnels(ctx, country, limit)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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
    wr.Write([]string{"Host", "IP address", "Direct port", "Peer port", "Vendor"})
    for host, ip := range tunnels.IPList {
        if (PROTOCOL_WHITELIST[tunnels.Protocol[host]]) {
            wr.Write([]string{host,
                              ip,
                              strconv.FormatUint(uint64(tunnels.Port.Direct), 10),
                              strconv.FormatUint(uint64(tunnels.Port.Peer), 10),
                              tunnels.Vendor[host]})
        }
    }
    wr.Flush()
    return 0
}

func get_endpoint(tunnels *ZGetTunnelsResponse, typ string) (string, error) {
    var hostname string
    for k, _ := range tunnels.IPList {
        hostname = k
        break
    }
    if hostname == "" {
        return "", errors.New("No tunnels found in API response")
    }
    var port uint16
    if typ == "direct" {
        port = tunnels.Port.Direct
    } else if typ == "peer" {
        port = tunnels.Port.Peer
    } else {
        return "", errors.New("Unsupported port type")
    }
    return net.JoinHostPort(hostname, strconv.FormatUint(uint64(port), 10)), nil
}

// Hop-by-hop headers. These are removed when sent to the backend.
// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
var hopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
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

