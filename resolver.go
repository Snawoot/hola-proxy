package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/ncruces/go-dns"
)

func FromURL(u string) (*net.Resolver, error) {
begin:
	parsed, err := url.Parse(u)
	if err != nil {
		return nil, err
	}
	host := parsed.Hostname()
	port := parsed.Port()
	switch scheme := strings.ToLower(parsed.Scheme); scheme {
	case "":
		switch {
		case strings.HasPrefix(u, "//"):
			u = "dns:" + u
		default:
			u = "dns://" + u
		}
		goto begin
	case "udp", "dns":
		if port == "" {
			port = "53"
		}
		return NewPlainResolver(net.JoinHostPort(host, port)), nil
	case "tcp":
		if port == "" {
			port = "53"
		}
		return NewTCPResolver(net.JoinHostPort(host, port)), nil
	case "http", "https", "doh":
		if port == "" {
			if scheme == "http" {
				port = "80"
			} else {
				port = "443"
			}
		}
		if scheme == "doh" {
			parsed.Scheme = "https"
			u = parsed.String()
		}
		return dns.NewDoHResolver(u, dns.DoHAddresses(net.JoinHostPort(host, port)))
	case "tls", "dot":
		if port == "" {
			port = "853"
		}
		hp := net.JoinHostPort(host, port)
		return dns.NewDoTResolver(hp, dns.DoTAddresses(hp))
	default:
		return nil, errors.New("not implemented")
	}
}

type LookupNetIPer interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type FastResolver struct {
	upstreams []LookupNetIPer
}

func FastResolverFromURLs(urls ...string) (LookupNetIPer, error) {
	resolvers := make([]LookupNetIPer, 0, len(urls))
	for i, u := range urls {
		res, err := FromURL(u)
		if err != nil {
			return nil, fmt.Errorf("unable to construct resolver #%d (%q): %w", i, u, err)
		}
		resolvers = append(resolvers, res)
	}
	if len(resolvers) == 1 {
		return resolvers[0], nil
	}
	return NewFastResolver(resolvers...), nil
}

func NewFastResolver(resolvers ...LookupNetIPer) *FastResolver {
	return &FastResolver{
		upstreams: resolvers,
	}
}

func (r FastResolver) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	ctx, cl := context.WithCancel(ctx)
	defer cl()
	errors := make(chan error)
	success := make(chan []netip.Addr)
	for _, res := range r.upstreams {
		go func(res LookupNetIPer) {
			addrs, err := res.LookupNetIP(ctx, network, host)
			if err == nil {
				select {
				case success <- addrs:
				case <-ctx.Done():
				}
			} else {
				select {
				case errors <- err:
				case <-ctx.Done():
				}
			}
		}(res)
	}

	var resErr error
	for _ = range r.upstreams {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case resAddrs := <-success:
			return resAddrs, nil
		case err := <-errors:
			resErr = multierror.Append(resErr, err)
		}
	}
	return nil, resErr
}

func NewPlainResolver(addr string) *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{
				Resolver: &net.Resolver{},
			}).DialContext(ctx, network, addr)
		},
	}
}

func NewTCPResolver(addr string) *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			dnet := "tcp"
			switch network {
			case "udp4":
				dnet = "tcp4"
			case "udp6":
				dnet = "tcp6"
			}
			return (&net.Dialer{
				Resolver: &net.Resolver{},
			}).DialContext(ctx, dnet, addr)
		},
	}
}
