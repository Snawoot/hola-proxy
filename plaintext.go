package main

import (
	"context"
	"crypto/x509"
	"errors"
	"net"

	tls "github.com/refraction-networking/utls"
)

type PlaintextDialer struct {
	fixedAddress  string
	tlsServerName string
	next          ContextDialer
	caPool        *x509.CertPool
	hideSNI       bool
}

func NewPlaintextDialer(address, tlsServerName string, caPool *x509.CertPool, hideSNI bool, next ContextDialer) *PlaintextDialer {
	return &PlaintextDialer{
		fixedAddress:  address,
		tlsServerName: tlsServerName,
		next:          next,
		caPool:        caPool,
		hideSNI:       hideSNI,
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
		sni := d.tlsServerName
		if d.hideSNI {
			sni = ""
		}
		conn = tls.UClient(conn, &tls.Config{
			ServerName:         sni,
			InsecureSkipVerify: true,
			VerifyConnection: func(cs tls.ConnectionState) error {
				opts := x509.VerifyOptions{
					DNSName:       d.tlsServerName,
					Intermediates: x509.NewCertPool(),
					Roots:         d.caPool,
				}
				for _, cert := range cs.PeerCertificates[1:] {
					opts.Intermediates.AddCert(cert)
				}
				_, err := cs.PeerCertificates[0].Verify(opts)
				return err
			},
		}, tls.HelloChrome_Auto)
	}
	return conn, nil
}

func (d *PlaintextDialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}
