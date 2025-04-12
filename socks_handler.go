package main

import (
	"log"

	"github.com/armon/go-socks5"
)

func NewSocksServer(dialer ContextDialer, logger *log.Logger) (*socks5.Server, error) {
	return socks5.New(&socks5.Config{
		Rules: &socks5.PermitCommand{
			EnableConnect: true,
		},
		Logger: logger,
		Dial:   dialer.DialContext,
	})
}
