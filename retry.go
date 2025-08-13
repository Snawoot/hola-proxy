package main

import (
	"context"
	"fmt"
	"net"
)

type RetryDialer struct {
	dialer   ContextDialer
	resolver LookupNetIPer
	logger   *CondLogger
}

func NewRetryDialer(dialer ContextDialer, resolver LookupNetIPer, logger *CondLogger) *RetryDialer {
	return &RetryDialer{
		dialer:   dialer,
		resolver: resolver,
		logger:   logger,
	}
}

func (d *RetryDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := d.dialer.DialContext(ctx, network, address)
	if err == UpstreamBlockedError {
		d.logger.Info("Destination %s blocked by upstream. Rescuing it with resolve&tunnel workaround.", address)
		host, port, err1 := net.SplitHostPort(address)
		if err1 != nil {
			return conn, err
		}

		var resolveNetwork string
		switch network {
		case "udp4", "tcp4", "ip4":
			resolveNetwork = "ip4"
		case "udp6", "tcp6", "ip6":
			resolveNetwork = "ip6"
		case "udp", "tcp", "ip":
			resolveNetwork = "ip"
		default:
			return nil, fmt.Errorf("resolving dial %q: unsupported network %q", address, network)
		}
		resolved, err := d.resolver.LookupNetIP(ctx, resolveNetwork, host)
		if err != nil {
			return nil, fmt.Errorf("dial failed on address lookup: %w", err)
		}

		var conn net.Conn
		for _, ip := range resolved {
			conn, err = d.dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
		}
		return nil, fmt.Errorf("failed to dial %s: %w", address, err)
	}
	return conn, err
}

func (d *RetryDialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}
