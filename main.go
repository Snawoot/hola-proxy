package main

import (
    "log"
    "os"
    "fmt"
    "flag"
//    "os/signal"
//    "syscall"
    "time"
    "net/http"
)

var (
    PROTOCOL_WHITELIST map[string]bool
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
    country string
    list_countries, list_proxies bool
    limit uint
    bind_address string
    verbosity int
    timeout, rotate time.Duration
    proxy_type string
    resolver string
}


func parse_args() CLIArgs {
    var args CLIArgs
    flag.StringVar(&args.country, "country", "us", "desired proxy location")
    flag.BoolVar(&args.list_countries, "list-countries", false, "list available countries and exit")
    flag.BoolVar(&args.list_proxies, "list-proxies", false, "output proxy list and exit")
    flag.UintVar(&args.limit, "limit", 3, "amount of proxies in retrieved list")
    flag.StringVar(&args.bind_address, "bind-address", "127.0.0.1:8080", "HTTP proxy listen address")
    flag.IntVar(&args.verbosity, "verbosity", 20, "logging verbosity " +
            "(10 - debug, 20 - info, 30 - warning, 40 - error, 50 - critical)")
    flag.DurationVar(&args.timeout, "timeout", 10 * time.Second, "timeout for network operations")
    flag.DurationVar(&args.rotate, "rotate", 1 * time.Hour, "rotate user ID once per given period")
    flag.StringVar(&args.proxy_type, "proxy-type", "direct", "proxy type: direct or peer")
    flag.StringVar(&args.resolver, "resolver", "https://cloudflare-dns.com/dns-query",
                   "DNS/DoH/DoT resolver to workaround Hola blocked hosts. " +
                   "See https://github.com/ameshkov/dnslookup/ for upstream DNS URL format.")
    flag.Parse()
    if args.country == "" {
        arg_fail("Country can't be empty string.")
    }
    if args.list_countries && args.list_proxies {
        arg_fail("list-countries and list-proxies flags are mutually exclusive")
    }
    return args
}

func run() int {
    args := parse_args()
    if args.list_countries {
        return print_countries(args.timeout)
    }
    if args.list_proxies {
        return print_proxies(args.country, args.limit, args.timeout)
    }

    logWriter := NewLogWriter(os.Stderr)
    defer logWriter.Close()

    mainLogger := NewCondLogger(log.New(logWriter, "MAIN    : ",
                                log.LstdFlags | log.Lshortfile),
                                args.verbosity)
    credLogger := NewCondLogger(log.New(logWriter, "CRED    : ",
                                log.LstdFlags | log.Lshortfile),
                                args.verbosity)
    proxyLogger := NewCondLogger(log.New(logWriter, "PROXY   : ",
                                log.LstdFlags | log.Lshortfile),
                                args.verbosity)
    mainLogger.Info("Constructing fallback DNS upstream...")
    resolver, err := NewResolver(args.resolver, args.timeout)
    if err != nil {
        mainLogger.Critical("Unable to instantiate DNS resolver: %v", err)
        return 6
    }
    mainLogger.Info("Initializing configuration provider...")
    auth, tunnels, err := CredService(args.rotate, args.timeout, args.country, credLogger)
    if err != nil {
        mainLogger.Critical("Unable to instantiate credential service: %v", err)
        logWriter.Close()
        return 4
    }
    endpoint, err := get_endpoint(tunnels, args.proxy_type)
    if err != nil {
        mainLogger.Critical("Unable to determine proxy endpoint: %v", err)
        logWriter.Close()
        return 5
    }
    mainLogger.Info("Endpoint: %s", endpoint)
    mainLogger.Info("Starting proxy server...")
    handler := NewProxyHandler(endpoint, auth, resolver, proxyLogger)
    err = http.ListenAndServe(args.bind_address, handler)
    mainLogger.Critical("Server terminated with a reason: %v", err)
    mainLogger.Info("Shutting down...")
    return 0
}

func main() {
    os.Exit(run())
}
