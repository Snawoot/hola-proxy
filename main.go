package main

import (
	"context"
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
	"strings"
	"time"

	tls "github.com/refraction-networking/utls"
	xproxy "golang.org/x/net/proxy"
)

const (
	HolaExtStoreID = "gkojfkhlekighikafcpjkiklfbnlmeio"
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
	extVer                                  string
	country                                 string
	list_countries, list_proxies, use_trial bool
	limit                                   uint
	bind_address                            string
	socksMode                               bool
	verbosity                               int
	timeout, rotate                         time.Duration
	proxy_type                              string
	resolver                                string
	force_port_field                        string
	showVersion                             bool
	proxy                                   string
	caFile                                  string
	minPause                                time.Duration
	maxPause                                time.Duration
	backoffInitial                          time.Duration
	backoffDeadline                         time.Duration
	initRetries                             int
	initRetryInterval                       time.Duration
	hideSNI                                 bool
	userAgent                               *string
}

func parse_args() CLIArgs {
	var args CLIArgs
	flag.StringVar(&args.extVer, "ext-ver", "", "extension version to mimic in requests. "+
		"Can be obtained from https://chrome.google.com/webstore/detail/hola-vpn-the-website-unbl/gkojfkhlekighikafcpjkiklfbnlmeio")
	flag.StringVar(&args.force_port_field, "force-port-field", "", "force specific port field/num (example 24232 or lum)") // would be nice to not show in help page
	flag.StringVar(&args.country, "country", "us", "desired proxy location")
	flag.BoolVar(&args.list_countries, "list-countries", false, "list available countries and exit")
	flag.BoolVar(&args.list_proxies, "list-proxies", false, "output proxy list and exit")
	flag.UintVar(&args.limit, "limit", 3, "amount of proxies in retrieved list")
	flag.StringVar(&args.bind_address, "bind-address", "127.0.0.1:8080", "proxy listen address")
	flag.BoolVar(&args.socksMode, "socks-mode", false, "listen for SOCKS requests instead of HTTP")
	flag.IntVar(&args.verbosity, "verbosity", 20, "logging verbosity "+
		"(10 - debug, 20 - info, 30 - warning, 40 - error, 50 - critical)")
	flag.DurationVar(&args.timeout, "timeout", 35*time.Second, "timeout for network operations")
	flag.DurationVar(&args.rotate, "rotate", 48*time.Hour, "rotate user ID once per given period")
	flag.DurationVar(&args.backoffInitial, "backoff-initial", 3*time.Second, "initial average backoff delay for zgettunnels (randomized by +/-50%)")
	flag.DurationVar(&args.backoffDeadline, "backoff-deadline", 5*time.Minute, "total duration of zgettunnels method attempts")
	flag.IntVar(&args.initRetries, "init-retries", 0, "number of attempts for initialization steps, zero for unlimited retry")
	flag.DurationVar(&args.initRetryInterval, "init-retry-interval", 5*time.Second, "delay between initialization retries")
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
	flag.Func("user-agent",
		"value of User-Agent header in requests. Default: User-Agent of latest stable Chrome for Windows",
		func(s string) error {
			args.userAgent = &s
			return nil
		})
	flag.BoolVar(&args.hideSNI, "hide-SNI", true, "hide SNI in TLS sessions with proxy server")
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
	socksLogger := log.New(logWriter, "SOCKS   : ",
		log.LstdFlags|log.Lshortfile)

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

	try := retryPolicy(args.initRetries, args.initRetryInterval, mainLogger)

	if args.list_countries {
		return print_countries(try, args.timeout)
	}

	mainLogger.Info("hola-proxy client version %s is starting...", version)

	var userAgent string
	if args.userAgent == nil {
		err := try("get latest version of Chrome browser", func() error {
			ctx, cl := context.WithTimeout(context.Background(), args.timeout)
			defer cl()
			ver, err := GetChromeVer(ctx, dialer)
			if err != nil {
				return err
			}
			mainLogger.Info("latest Chrome version is %q", ver)
			majorVer, _, _ := strings.Cut(ver, ".")
			userAgent = fmt.Sprintf(
				"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s.0.0.0 Safari/537.36",
				majorVer)
			mainLogger.Info("discovered latest Chrome User-Agent: %q", userAgent)
			return err
		})
		if err != nil {
			mainLogger.Critical("Can't detect latest Chrome version. "+
				"Try to specify proper user agent with -user-agent parameter. Error: %v",
				err)
			return 8
		}
	} else {
		userAgent = *args.userAgent
	}
	SetUserAgent(userAgent)

	if args.extVer == "" {
		err := try("get latest version of browser extension", func() error {
			ctx, cl := context.WithTimeout(context.Background(), args.timeout)
			defer cl()
			extVer, err := GetExtVer(ctx, nil, HolaExtStoreID, dialer)
			if err == nil {
				mainLogger.Info("discovered latest browser extension version: %s", extVer)
				args.extVer = extVer
			}
			return err
		})
		if err != nil {
			mainLogger.Critical("Can't detect latest browser extension version. Try to specify -ext-ver parameter. Error: %v", err)
			return 8
		}
		mainLogger.Warning("Detected latest extension version: %q. Pass -ext-ver parameter to skip resolve and speedup startup", args.extVer)
	}
	if args.list_proxies {
		return print_proxies(try, mainLogger, args.extVer, args.country, args.proxy_type, args.limit, args.timeout,
			args.backoffInitial, args.backoffDeadline)
	}

	mainLogger.Info("Constructing fallback DNS upstream...")
	resolver, err := NewResolver(args.resolver, args.timeout)
	if err != nil {
		mainLogger.Critical("Unable to instantiate DNS resolver: %v", err)
		return 6
	}

	var (
		auth    AuthProvider
		tunnels *ZGetTunnelsResponse
	)
	err = try("run credentials service", func() error {
		auth, tunnels, err = CredService(args.rotate, args.timeout, args.extVer, args.country,
			args.proxy_type, credLogger, args.backoffInitial, args.backoffDeadline)
		return err
	})
	if err != nil {
		return 4
	}
	endpoint, err := get_endpoint(tunnels, args.proxy_type, args.use_trial, args.force_port_field)
	if err != nil {
		mainLogger.Critical("Unable to determine proxy endpoint: %v", err)
		return 5
	}
	handlerDialer := NewProxyDialer(endpoint.NetAddr(), endpoint.TLSName, caPool, auth, args.hideSNI, dialer)
	requestDialer := NewPlaintextDialer(endpoint.NetAddr(), endpoint.TLSName, caPool, args.hideSNI, dialer)
	mainLogger.Info("Endpoint: %s", endpoint.URL().String())
	mainLogger.Info("Starting proxy server...")
	if args.socksMode {
		socks, initError := NewSocksServer(handlerDialer, socksLogger)
		if initError != nil {
			mainLogger.Critical("Failed to start: %v", err)
			return 6
		}
		mainLogger.Info("Init complete.")
		err = socks.ListenAndServe("tcp", args.bind_address)
	} else {
		handler := NewProxyHandler(handlerDialer, requestDialer, auth, resolver, proxyLogger)
		mainLogger.Info("Init complete.")
		err = http.ListenAndServe(args.bind_address, handler)
	}
	mainLogger.Critical("Server terminated with a reason: %v", err)
	mainLogger.Info("Shutting down...")
	return 0
}

func main() {
	os.Exit(run())
}

func retryPolicy(retries int, retryInterval time.Duration, logger *CondLogger) func(string, func() error) error {
	return func(name string, f func() error) error {
		var err error
		for i := 1; retries <= 0 || i <= retries; i++ {
			if i > 1 {
				logger.Warning("Retrying action %q in %v...", name, retryInterval)
				time.Sleep(retryInterval)
			}
			logger.Info("Attempting action %q, attempt #%d...", name, i)
			err = f()
			if err == nil {
				logger.Info("Action %q succeeded on attempt #%d", name, i)
				return nil
			}
			logger.Warning("Action %q failed: %v", name, err)
		}
		logger.Critical("All attempts for action %q have failed. Last error: %v", name, err)
		return err
	}
}
