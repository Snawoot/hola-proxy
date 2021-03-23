package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
)

const (
	PROXY_CONNECT_METHOD       = "CONNECT"
	PROXY_HOST_HEADER          = "Host"
	PROXY_AUTHORIZATION_HEADER = "Proxy-Authorization"
)

type ContextDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

type ProxyDialer struct {
	address       string
	tlsServerName string
	auth          AuthProvider
	next          ContextDialer
}

func NewProxyDialer(address, tlsServerName string, auth AuthProvider, nextDialer ContextDialer) *ProxyDialer {
	return &ProxyDialer{
		address:       address,
		tlsServerName: tlsServerName,
		auth:          auth,
		next:          nextDialer,
	}
}

func (d *ProxyDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil, errors.New("bad network specified for DialContext: only tcp is supported")
	}

	conn, err := d.next.DialContext(ctx, "tcp", d.address)
	if err != nil {
		return nil, err
	}

	if d.tlsServerName != "" {
		// Custom cert verification logic:
		// DO NOT send SNI extension of TLS ClientHello
		// DO peer certificate verification against specified servername
		conn = tls.Client(conn, &tls.Config{
			ServerName:         "",
			InsecureSkipVerify: true,
			VerifyConnection: func(cs tls.ConnectionState) error {
				opts := x509.VerifyOptions{
					DNSName:       d.tlsServerName,
					Intermediates: x509.NewCertPool(),
				}
				for _, cert := range cs.PeerCertificates[1:] {
					opts.Intermediates.AddCert(cert)
				}
				_, err := cs.PeerCertificates[0].Verify(opts)
				return err
			},
		})
	}

	req := &http.Request{
		Method:     PROXY_CONNECT_METHOD,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		RequestURI: address,
		Host:       address,
		Header: http.Header{
			PROXY_HOST_HEADER: []string{address},
		},
	}

	if d.auth != nil {
		req.Header.Set(PROXY_AUTHORIZATION_HEADER, d.auth())
	}

	rawreq, err := httputil.DumpRequest(req, false)
	if err != nil {
		return nil, err
	}

	_, err = conn.Write(rawreq)
	if err != nil {
		return nil, err
	}

	proxyResp, err := readResponse(conn, req)
	if err != nil {
		return nil, err
	}

	if proxyResp.StatusCode != http.StatusOK {
		return nil, errors.New("Bad response from upstream proxy server")
	}

	return conn, nil
}

func readResponse(r io.Reader, req *http.Request) (*http.Response, error) {
	endOfResponse := []byte("\r\n\r\n")
	buf := &bytes.Buffer{}
	b := make([]byte, 1)
	for {
		n, err := r.Read(b)
		if n < 1 && err == nil {
			continue
		}

		buf.Write(b)
		sl := buf.Bytes()
		if len(sl) < len(endOfResponse) {
			continue
		}

		if bytes.Equal(sl[len(sl)-4:], endOfResponse) {
			break
		}

		if err != nil {
			return nil, err
		}
	}
	return http.ReadResponse(bufio.NewReader(buf), req)
}
