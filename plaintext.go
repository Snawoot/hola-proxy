package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
)

type PlaintextDialer struct {
	fixedAddress  string
	tlsServerName string
	next          ContextDialer
}

func NewPlaintextDialer(address, tlsServerName string, next ContextDialer) *PlaintextDialer {
	return &PlaintextDialer{
		fixedAddress:  address,
		tlsServerName: tlsServerName,
		next:          next,
	}
}

func (d *PlaintextDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil, errors.New("bad network specified for DialContext: only tcp is supported")
	}

	conn, err := d.next.DialContext(ctx, "tcp", d.fixedAddress)
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
	return conn, nil
}

func (d *PlaintextDialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}
