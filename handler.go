package main

import (
    "io"
    "net/http"
    "net/http/httputil"
    "crypto/tls"
    "strings"
    "context"
    "time"
    "net/url"
)

type AuthProvider func() string

type ProxyHandler struct {
    auth AuthProvider
    upstream string
    logger *CondLogger
    httpclient *http.Client
}

func NewProxyHandler(upstream string, auth AuthProvider, logger *CondLogger) *ProxyHandler {
    proxyurl, err := url.Parse("https://" + upstream)
    if err != nil {
        panic(err)
    }
	httpclient := &http.Client{
        Transport: &http.Transport{
            Proxy: http.ProxyURL(proxyurl)}}
    return &ProxyHandler{
        auth: auth,
        upstream: upstream,
        logger: logger,
        httpclient: httpclient,
    }
}

func (s *ProxyHandler) ServeHTTP(wr http.ResponseWriter, req *http.Request) {
	s.logger.Info("Request: %v %v %v", req.RemoteAddr, req.Method, req.URL)
    req.Header.Set("Proxy-Authorization", s.auth())
    if strings.ToUpper(req.Method) == "CONNECT" {
        rawreq, err := httputil.DumpRequest(req, false)
        if err != nil {
            s.logger.Error("Can't dump request: %v", err)
            http.Error(wr, "Can't dump request", http.StatusInternalServerError)
            return
        }
        conn, err := tls.Dial("tcp", s.upstream, nil)
        if err != nil {
            s.logger.Error("Can't dial tls upstream: %v", err)
            http.Error(wr, "Can't dial tls upstream", http.StatusInternalServerError)
            return
        defer conn.Close()
        }
        hj, ok := wr.(http.Hijacker)
		if !ok {
            s.logger.Critical("Webserver doesn't support hijacking")
			http.Error(wr, "Webserver doesn't support hijacking", http.StatusInternalServerError)
			return
		}
        localconn, _, err := hj.Hijack()
        if err != nil {
            s.logger.Error("Can't hijack client connection: %v", err)
            http.Error(wr, "Can't hijack client connection", http.StatusInternalServerError)
            return
        }
        var emptytime time.Time
        err = localconn.SetDeadline(emptytime)
        if err != nil {
            s.logger.Error("Can't clear deadlines on local connection: %v", err)
            http.Error(wr, "Can't clear deadlines on local connection", http.StatusInternalServerError)
            return
        }
        conn.Write(rawreq)
        proxy(context.TODO(), localconn, conn)
    } else {
        req.RequestURI = ""
        delHopHeaders(req.Header)
        resp, err := s.httpclient.Do(req)
        if err != nil {
            s.logger.Error("HTTP fetch error: %v", err)
            http.Error(wr, "Server Error", http.StatusInternalServerError)
            return
        }
        defer resp.Body.Close()
        s.logger.Info("Response: %v %v", req.RemoteAddr, resp.Status)
        delHopHeaders(resp.Header)
        copyHeader(wr.Header(), resp.Header)
        wr.WriteHeader(resp.StatusCode)
        io.Copy(wr, resp.Body)
    }
}
