package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	xproxy "golang.org/x/net/proxy"
)

var (
	PROTOCOL_WHITELIST map[string]bool
	version            = "undefined"
)

func init() {
	PROTOCOL_WHITELIST = map[string]bool{
		"HTTP": true,
		"http": true,
	}
}

func perror(msg string) {
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, msg)
}

func arg_fail(msg string) {
	perror(msg)
	perror("Usage:")
	flag.PrintDefaults()
	os.Exit(2)
}

type CLIArgs struct {
	country                                 string
	list_countries, list_proxies, use_trial bool
	limit                                   uint
	bind_address                            string
	verbosity                               int
	timeout, rotate                         time.Duration
	proxy_type                              string
	resolver                                string
	force_port_field                        string
	showVersion                             bool
	proxy                                   string
	caFile                                  string
}

func parse_args() CLIArgs {
	var args CLIArgs
	flag.StringVar(&args.force_port_field, "force-port-field", "", "force specific port field/num (example 24232 or lum)") // would be nice to not show in help page
	flag.StringVar(&args.country, "country", "us", "desired proxy location")
	flag.BoolVar(&args.list_countries, "list-countries", false, "list available countries and exit")
	flag.BoolVar(&args.list_proxies, "list-proxies", false, "output proxy list and exit")
	flag.UintVar(&args.limit, "limit", 3, "amount of proxies in retrieved list")
	flag.StringVar(&args.bind_address, "bind-address", "127.0.0.1:8080", "HTTP proxy listen address")
	flag.IntVar(&args.verbosity, "verbosity", 20, "logging verbosity "+
		"(10 - debug, 20 - info, 30 - warning, 40 - error, 50 - critical)")
	flag.DurationVar(&args.timeout, "timeout", 10*time.Second, "timeout for network operations")
	flag.DurationVar(&args.rotate, "rotate", 1*time.Hour, "rotate user ID once per given period")
	flag.StringVar(&args.proxy_type, "proxy-type", "direct", "proxy type: direct or lum") // or skip but not mentioned
	// skip would be used something like this: `./bin/hola-proxy -proxy-type skip -force-port-field 24232 -country ua.peer` for debugging
	flag.StringVar(&args.resolver, "resolver", "https://cloudflare-dns.com/dns-query",
		"DNS/DoH/DoT resolver to workaround Hola blocked hosts. "+
			"See https://github.com/ameshkov/dnslookup/ for upstream DNS URL format.")
	flag.BoolVar(&args.use_trial, "dont-use-trial", false, "use regular ports instead of trial ports") // would be nice to not show in help page
	flag.BoolVar(&args.showVersion, "version", false, "show program version and exit")
	flag.StringVar(&args.proxy, "proxy", "", "sets base proxy to use for all dial-outs. "+
		"Format: <http|https|socks5|socks5h>://[login:password@]host[:port] "+
		"Examples: http://user:password@192.168.1.1:3128, socks5://10.0.0.1:1080")
	flag.StringVar(&args.caFile, "cafile", "", "use custom CA certificate bundle file")
	flag.Parse()
	if args.country == "" {
		arg_fail("Country can't be empty string.")
	}
	if args.proxy_type == "" {
		arg_fail("Proxy type can't be an empty string.")
	}
	if args.list_countries && args.list_proxies {
		arg_fail("list-countries and list-proxies flags are mutually exclusive")
	}
	return args
}

func run() int {
	args := parse_args()
	if args.showVersion {
		fmt.Println(version)
		return 0
	}

	logWriter := NewLogWriter(os.Stderr)
	defer logWriter.Close()

	mainLogger := NewCondLogger(log.New(logWriter, "MAIN    : ",
		log.LstdFlags|log.Lshortfile),
		args.verbosity)
	credLogger := NewCondLogger(log.New(logWriter, "CRED    : ",
		log.LstdFlags|log.Lshortfile),
		args.verbosity)
	proxyLogger := NewCondLogger(log.New(logWriter, "PROXY   : ",
		log.LstdFlags|log.Lshortfile),
		args.verbosity)

	var dialer ContextDialer = &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	var caPool *x509.CertPool
	if args.caFile != "" {
		caPool = x509.NewCertPool()
		certs, err := ioutil.ReadFile(args.caFile)
		if err != nil {
			mainLogger.Error("Can't load CA file: %v", err)
			return 15
		}
		if ok := caPool.AppendCertsFromPEM(certs); !ok {
			mainLogger.Error("Can't load certificates from CA file")
			return 15
		}
		UpdateHolaTLSConfig(&tls.Config{
			RootCAs: caPool,
		})
	}

	proxyFromURLWrapper := func(u *url.URL, next xproxy.Dialer) (xproxy.Dialer, error) {
		cdialer, ok := next.(ContextDialer)
		if !ok {
			return nil, errors.New("only context dialers are accepted")
		}

		return ProxyDialerFromURL(u, caPool, cdialer)
	}

	if args.proxy != "" {
		xproxy.RegisterDialerType("http", proxyFromURLWrapper)
		xproxy.RegisterDialerType("https", proxyFromURLWrapper)
		proxyURL, err := url.Parse(args.proxy)
		if err != nil {
			mainLogger.Critical("Unable to parse base proxy URL: %v", err)
			return 6
		}
		pxDialer, err := xproxy.FromURL(proxyURL, dialer)
		if err != nil {
			mainLogger.Critical("Unable to instantiate base proxy dialer: %v", err)
			return 7
		}
		dialer = pxDialer.(ContextDialer)
		UpdateHolaDialer(dialer)
	}

	if args.list_countries {
		return print_countries(args.timeout)
	}
	if args.list_proxies {
		return print_proxies(args.country, args.proxy_type, args.limit, args.timeout)
	}

	mainLogger.Info("hola-proxy client version %s is starting...", version)
	mainLogger.Info("Constructing fallback DNS upstream...")
	resolver, err := NewResolver(args.resolver, args.timeout)
	if err != nil {
		mainLogger.Critical("Unable to instantiate DNS resolver: %v", err)
		return 6
	}

	mainLogger.Info("Initializing configuration provider...")
	auth, tunnels, err := CredService(args.rotate, args.timeout, args.country, args.proxy_type, credLogger)
	if err != nil {
		mainLogger.Critical("Unable to instantiate credential service: %v", err)
		return 4
	}
	endpoint, err := get_endpoint(tunnels, args.proxy_type, args.use_trial, args.force_port_field)
	if err != nil {
		mainLogger.Critical("Unable to determine proxy endpoint: %v", err)
		return 5
	}
	handlerDialer := NewProxyDialer(endpoint.NetAddr(), endpoint.TLSName, caPool, auth, dialer)
	requestDialer := NewPlaintextDialer(endpoint.NetAddr(), endpoint.TLSName, caPool, dialer)
	mainLogger.Info("Endpoint: %s", endpoint.URL().String())
	mainLogger.Info("Starting proxy server...")
	handler := NewProxyHandler(handlerDialer, requestDialer, auth, resolver, proxyLogger)
	mainLogger.Info("Init complete.")
	err = http.ListenAndServe(args.bind_address, handler)
	mainLogger.Critical("Server terminated with a reason: %v", err)
	mainLogger.Info("Shutting down...")
	return 0
}

func main() {
	os.Exit(run())
}
