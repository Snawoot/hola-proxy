package main

import (
	"context"
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

		ips, err := d.resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, err
		}

		return d.dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
	return conn, err
}

func (d *RetryDialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}
