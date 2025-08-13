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
	parsed, err := url.Parse(u)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(parsed.Scheme) {
	case "", "dns":
		host := parsed.Hostname()
		port := parsed.Port()
		if port == "" {
			port = "53"
		}
		return NewPlainResolver(net.JoinHostPort(host, port)), nil
	case "tcp":
		host := parsed.Hostname()
		port := parsed.Port()
		if port == "" {
			port = "53"
		}
		return NewTCPResolver(net.JoinHostPort(host, port)), nil
	case "http", "https":
		return dns.NewDoHResolver(u)
	case "tls":
		host := parsed.Hostname()
		port := parsed.Port()
		if port == "" {
			port = "853"
		}
		return dns.NewDoTResolver(net.JoinHostPort(host, port))
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

type lookupReply struct {
	addrs []netip.Addr
	err   error
}

func FastResolverFromURLs(urls ...string) (*FastResolver, error) {
	resolvers := make([]LookupNetIPer, 0, len(urls))
	for i, u := range urls {
		res, err := FromURL(u)
		if err != nil {
			return nil, fmt.Errorf("unable to construct resolver #%d (%q): %w", i, u, err)
		}
		resolvers = append(resolvers, res)
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
	drain := make(chan lookupReply, len(r.upstreams))
	for _, res := range r.upstreams {
		go func(res LookupNetIPer) {
			addrs, err := res.LookupNetIP(ctx, network, host)
			drain <- lookupReply{addrs, err}
		}(res)
	}

	i := 0
	var resAddrs []netip.Addr
	var resErr error
	for ; i < len(r.upstreams); i++ {
		pair := <-drain
		if pair.err != nil {
			resErr = multierror.Append(resErr, pair.err)
		} else {
			cl()
			resAddrs = pair.addrs
			resErr = nil
			break
		}
	}
	go func() {
		for i = i + 1; i < len(r.upstreams); i++ {
			<-drain
		}
	}()
	return resAddrs, resErr
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
