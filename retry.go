package main

import (
	"context"
	"net"
)

type RetryDialer struct {
	dialer   ContextDialer
	resolver *Resolver
	logger   *CondLogger
}

func NewRetryDialer(dialer ContextDialer, resolver *Resolver, logger *CondLogger) *RetryDialer {
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

		ips := d.resolver.Resolve(host)
		if len(ips) == 0 {
			return conn, err
		}

		return d.dialer.DialContext(ctx, network, net.JoinHostPort(ips[0], port))
	}
	return conn, err
}

func (d *RetryDialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}
