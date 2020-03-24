package main

import (
    "time"
    "net"
    "crypto/tls"
    "crypto/x509"
    "io/ioutil"
    "errors"
    "context"
    "strconv"
)

const MAX_READ_CH_QLEN = 128
const COPY_BUFFER_SIZE = 32 * 1024
var ZEROTIME time.Time
var EPOCH time.Time

func init() {
    EPOCH = time.Unix(0, 0)
}

type WrappedConn interface {
    Read(p []byte) (n int, err error)
    Write(p []byte) (n int, err error)
    Close()
    LocalAddr() net.Addr
    RemoteAddr() net.Addr
    SetDeadline(t time.Time) error
    SetReadDeadline(t time.Time) error
    SetWriteDeadline(t time.Time) error
    ReadContext(ctx context.Context, p []byte) (n int, err error)
    WriteContext(ctx context.Context, p []byte) (n int, err error)
}

type ConnFactory struct {
    addr string
    tlsConfig *tls.Config
    dialer *net.Dialer
    logger *CondLogger
}

func NewConnFactory(host string, port uint16, timeout time.Duration,
                    certfile, keyfile string, cafile string, hostname_check bool,
                    tls_servername string, dialers uint, logger *CondLogger) (*ConnFactory, error) {
    if !hostname_check && cafile == "" {
        return nil, errors.New("Hostname check should not be disabled in absence of custom CA file")
    }
    if certfile != "" && keyfile == "" || certfile == "" && keyfile != "" {
        return nil, errors.New("Certificate file and key file must be specified only together")
    }
    var certs []tls.Certificate
    if certfile != "" && keyfile != "" {
        cert, err := tls.LoadX509KeyPair(certfile, keyfile)
        if err != nil {
            return nil, err
        }
        certs = append(certs, cert)
    }
    var roots *x509.CertPool
    if cafile == "" {
        sysroots, err := x509.SystemCertPool()
        if err != nil {
            return nil, err
        }
        roots = sysroots
    } else {
        roots = x509.NewCertPool()
        certs, err := ioutil.ReadFile(cafile)
        if err != nil {
            return nil, err
        }
        if ok := roots.AppendCertsFromPEM(certs); !ok {
            return nil, errors.New("Failed to load CA certificates")
        }
    }
    servername := host
    if tls_servername != "" {
        servername = tls_servername
    }
    tlsConfig := tls.Config{
        RootCAs: roots,
        ServerName: servername,
        Certificates: certs,
    }
    if !hostname_check {
        tlsConfig.InsecureSkipVerify = true
        tlsConfig.VerifyPeerCertificate = func(certificates [][]byte, _ [][]*x509.Certificate) error {
            certs := make([]*x509.Certificate, len(certificates))
            for i, asn1Data := range certificates {
                cert, err := x509.ParseCertificate(asn1Data)
                if err != nil {
                    return errors.New("tls: failed to parse certificate from server: " + err.Error())
                }
                certs[i] = cert
            }

            opts := x509.VerifyOptions{
                Roots:         roots, // On the server side, use config.ClientCAs.
                DNSName:       "", // No hostname check
                Intermediates: x509.NewCertPool(),
            }
            for _, cert := range certs[1:] {
                opts.Intermediates.AddCert(cert)
            }
            _, err := certs[0].Verify(opts)
            return err
        }
    }
    return &ConnFactory{
        addr: net.JoinHostPort(host, strconv.Itoa(int(port))),
        tlsConfig: &tlsConfig,
        dialer: &net.Dialer{Timeout: timeout},
        logger: logger,
    }, nil
}

type WrappedTLSConn struct {
    conn *tls.Conn
    logger *CondLogger
}

func (cf *ConnFactory) DialContext(ctx context.Context) (*WrappedTLSConn, error) {
    var newconn *tls.Conn
    var err error
    ch := make(chan struct{}, 1)
    go func () {
        newconn, err = tls.DialWithDialer(cf.dialer, "tcp", cf.addr, cf.tlsConfig)
        ch <- struct{}{}
    }()
    select {
    case <- ch:
        if err != nil {
            cf.logger.Error("Got error during connection: %s", err)
            return nil, err
        }
        wrappedconn := &WrappedTLSConn{
            conn: newconn,
            logger: cf.logger,
        }
        return wrappedconn, nil
    case <- ctx.Done():
        return nil, errors.New("Context was cancelled")
    }
}

func (c *WrappedTLSConn) Read(p []byte) (n int, err error) {
    return c.conn.Read(p)
}

func (c *WrappedTLSConn) Write(p []byte) (n int, err error) {
    return c.conn.Write(p)
}

func (c *WrappedTLSConn) Close() error {
    return c.conn.Close()
}

func (c *WrappedTLSConn) LocalAddr() net.Addr {
    return c.conn.LocalAddr()
}

func (c *WrappedTLSConn) RemoteAddr() net.Addr {
    return c.conn.RemoteAddr()
}

func (c *WrappedTLSConn) SetDeadline(t time.Time) error {
    return c.conn.SetDeadline(t)
}

func (c *WrappedTLSConn) SetReadDeadline(t time.Time) error {
    return c.conn.SetReadDeadline(t)
}

func (c *WrappedTLSConn) SetWriteDeadline(t time.Time) error {
    return c.conn.SetWriteDeadline(t)
}

func (c *WrappedTLSConn) ReadContext(ctx context.Context, p []byte) (n int, err error) {
    ch := make(chan struct{}, 1)
    go func () {
        n, err = c.conn.Read(p)
        ch <-struct{}{}
    }()
    select {
    case <-ctx.Done():
        c.conn.SetReadDeadline(EPOCH)
        <-ch
        c.conn.SetReadDeadline(ZEROTIME)
    case <-ch:
    }
    return
}
