package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"
)

type AuthProvider func() string

type ProxyHandler struct {
	auth          AuthProvider
	upstreamAddr  string
	tlsName       string
	logger        *CondLogger
	dialer        *net.Dialer
	httptransport http.RoundTripper
	resolver      *Resolver
}

func NewProxyHandler(upstream *Endpoint, auth AuthProvider, resolver *Resolver, logger *CondLogger) *ProxyHandler {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	netaddr := net.JoinHostPort(upstream.Host, fmt.Sprintf("%d", upstream.Port))
	httptransport := &http.Transport{
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		Proxy:                 http.ProxyURL(upstream.URL()),
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp", netaddr)
		},
	}
	return &ProxyHandler{
		auth:          auth,
		upstreamAddr:  netaddr,
		tlsName:       upstream.TLSName,
		logger:        logger,
		dialer:        dialer,
		httptransport: httptransport,
		resolver:      resolver,
	}
}

func (s *ProxyHandler) ServeHTTP(wr http.ResponseWriter, req *http.Request) {
	s.logger.Info("Request: %v %v %v", req.RemoteAddr, req.Method, req.URL)
	if strings.ToUpper(req.Method) == "CONNECT" {
		req.Header.Set("Proxy-Authorization", s.auth())
		rawreq, err := httputil.DumpRequest(req, false)
		if err != nil {
			s.logger.Error("Can't dump request: %v", err)
			http.Error(wr, "Can't dump request", http.StatusInternalServerError)
			return
		}

		conn, err := s.dialer.DialContext(req.Context(), "tcp", s.upstreamAddr)
		if err != nil {
			s.logger.Error("Can't dial upstream: %v", err)
			http.Error(wr, "Can't dial upstream", http.StatusBadGateway)
			return
		}
		defer conn.Close()

		if s.tlsName != "" {
			conn = tls.Client(conn, &tls.Config{
				ServerName: s.tlsName,
			})
			defer conn.Close()
		}

		_, err = conn.Write(rawreq)
		if err != nil {
			s.logger.Error("Can't write upstream: %v", err)
			http.Error(wr, "Can't write upstream", http.StatusBadGateway)
			return
		}
		bufrd := bufio.NewReader(conn)
		proxyResp, err := http.ReadResponse(bufrd, req)
		responseBytes := make([]byte, 0)
		if err != nil {
			s.logger.Error("Can't read response from upstream: %v", err)
			http.Error(wr, "Can't read response from upstream", http.StatusBadGateway)
			return
		}

		if proxyResp.StatusCode == http.StatusForbidden &&
			proxyResp.Header.Get("X-Hola-Error") == "Forbidden Host" {
			s.logger.Info("Request %s denied by upstream. Rescuing it with resolve&rewrite workaround.",
				req.URL.String())

			conn, err = s.dialer.DialContext(req.Context(), "tcp", s.upstreamAddr)
			if err != nil {
				s.logger.Error("Can't dial upstream: %v", err)
				http.Error(wr, "Can't dial upstream", http.StatusBadGateway)
				return
			}
			defer conn.Close()

			if s.tlsName != "" {
				conn = tls.Client(conn, &tls.Config{
					ServerName: s.tlsName,
				})
				defer conn.Close()
			}

			err = rewriteConnectReq(req, s.resolver)
			if err != nil {
				s.logger.Error("Can't rewrite request: %v", err)
				http.Error(wr, "Can't rewrite request", http.StatusInternalServerError)
				return
			}
			rawreq, err = httputil.DumpRequest(req, false)
			if err != nil {
				s.logger.Error("Can't dump request: %v", err)
				http.Error(wr, "Can't dump request", http.StatusInternalServerError)
				return
			}
			_, err = conn.Write(rawreq)
			if err != nil {
				s.logger.Error("Can't write tls upstream: %v", err)
				http.Error(wr, "Can't write tls upstream", http.StatusBadGateway)
				return
			}
		} else {
			responseBytes, err = httputil.DumpResponse(proxyResp, false)
			if err != nil {
				s.logger.Error("Can't dump response: %v", err)
				http.Error(wr, "Can't dump response", http.StatusInternalServerError)
				return
			}
			buffered := bufrd.Buffered()
			if buffered > 0 {
				trailer := make([]byte, buffered)
				bufrd.Read(trailer)
				responseBytes = append(responseBytes, trailer...)
			}
		}
		bufrd = nil

		// Upgrade client connection
		localconn, _, err := hijack(wr)
		if err != nil {
			s.logger.Error("Can't hijack client connection: %v", err)
			http.Error(wr, "Can't hijack client connection", http.StatusInternalServerError)
			return
		}
		defer localconn.Close()

		if len(responseBytes) > 0 {
			_, err = localconn.Write(responseBytes)
			if err != nil {
				return
			}
		}
		proxy(req.Context(), localconn, conn)
	} else {
		delHopHeaders(req.Header)
		orig_req := req.Clone(req.Context())
		req.RequestURI = ""
		req.Header.Set("Proxy-Authorization", s.auth())
		resp, err := s.httptransport.RoundTrip(req)
		if err != nil {
			s.logger.Error("HTTP fetch error: %v", err)
			http.Error(wr, "Server Error", http.StatusInternalServerError)
			return
		}
		if resp.StatusCode == http.StatusForbidden &&
			resp.Header.Get("X-Hola-Error") == "Forbidden Host" {
			s.logger.Info("Request %s denied by upstream. Rescuing it with resolve&tunnel workaround.",
				req.URL.String())
			resp.Body.Close()

			// Prepare tunnel request
			proxyReq, err := makeConnReq(orig_req.RequestURI, s.resolver)
			if err != nil {
				s.logger.Error("Can't rewrite request: %v", err)
				http.Error(wr, "Can't rewrite request", http.StatusBadGateway)
				return
			}
			proxyReq.Header.Set("Proxy-Authorization", s.auth())
			rawreq, _ := httputil.DumpRequest(proxyReq, false)

			// Prepare upstream conn
			conn, err := s.dialer.DialContext(req.Context(), "tcp", s.upstreamAddr)
			if err != nil {
				s.logger.Error("Can't dial tls upstream: %v", err)
				http.Error(wr, "Can't dial tls upstream", http.StatusBadGateway)
				return
			}
			defer conn.Close()

			if s.tlsName != "" {
				conn = tls.Client(conn, &tls.Config{
					ServerName: s.tlsName,
				})
				defer conn.Close()
			}

			// Send proxy request
			_, err = conn.Write(rawreq)
			if err != nil {
				s.logger.Error("Can't write tls upstream: %v", err)
				http.Error(wr, "Can't write tls upstream", http.StatusBadGateway)
				return
			}

			// Read proxy response
			bufrd := bufio.NewReader(conn)
			proxyResp, err := http.ReadResponse(bufrd, proxyReq)
			if err != nil {
				s.logger.Error("Can't read response from upstream: %v", err)
				http.Error(wr, "Can't read response from upstream", http.StatusBadGateway)
				return
			}
			if proxyResp.StatusCode != http.StatusOK {
				delHopHeaders(proxyResp.Header)
				copyHeader(wr.Header(), proxyResp.Header)
				wr.WriteHeader(proxyResp.StatusCode)
			}

			// Send tunneled request
			orig_req.RequestURI = ""
			orig_req.Header.Set("Connection", "close")
			rawreq, _ = httputil.DumpRequest(orig_req, false)
			_, err = conn.Write(rawreq)
			if err != nil {
				s.logger.Error("Can't write tls upstream: %v", err)
				http.Error(wr, "Can't write tls upstream", http.StatusBadGateway)
				return
			}

			// Read tunneled response
			resp, err = http.ReadResponse(bufrd, orig_req)
			if err != nil {
				s.logger.Error("Can't read response from upstream: %v", err)
				http.Error(wr, "Can't read response from upstream", http.StatusBadGateway)
				return
			}
		}
		defer resp.Body.Close()
		s.logger.Info("%v %v %v %v", req.RemoteAddr, req.Method, req.URL, resp.Status)
		delHopHeaders(resp.Header)
		copyHeader(wr.Header(), resp.Header)
		wr.WriteHeader(resp.StatusCode)
		io.Copy(wr, resp.Body)
	}
}
