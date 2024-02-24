package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/campoy/unique"
	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	tls "github.com/refraction-networking/utls"
)

const EXT_BROWSER = "chrome"
const PRODUCT = "cws"
const CCGI_URL = "https://client.hola.org/client_cgi/"
const VPN_COUNTRIES_URL = CCGI_URL + "vpn_countries.json"
const BG_INIT_URL = CCGI_URL + "background_init"
const ZGETTUNNELS_URL = CCGI_URL + "zgettunnels"
const FALLBACK_CONF_URL = "https://www.dropbox.com/s/jemizcvpmf2qb9v/cloud_failover.conf?dl=1"
const AGENT_SUFFIX = ".hola.org"

var LOGIN_TEMPLATE = template.Must(template.New("LOGIN_TEMPLATE").Parse("user-uuid-{{.uuid}}-is_prem-{{.prem}}"))
var TemporaryBanError = errors.New("temporary ban detected")
var PermanentBanError = errors.New("permanent ban detected")
var EmptyResponseError = errors.New("empty response")

var userAgent = "Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/118.0"

func SetUserAgent(ua string) {
	userAgent = ua
}

func GetUserAgent() string {
	return userAgent
}

type CountryList []string

type BgInitResponse struct {
	Ver       string `json:"ver"`
	Key       int64  `json:"key"`
	Country   string `json:"country"`
	Blocked   bool   `json:"blocked,omitempty"`
	Permanent bool   `json:"permanent,omitempty"`
}

type PortMap struct {
	Direct    uint16 `json:"direct"`
	Hola      uint16 `json:"hola"`
	Peer      uint16 `json:"peer"`
	Trial     uint16 `json:"trial"`
	TrialPeer uint16 `json:"trial_peer"`
}

type ZGetTunnelsResponse struct {
	AgentKey   string              `json:"agent_key"`
	AgentTypes map[string]string   `json:"agent_types"`
	IPList     map[string]string   `json:"ip_list"`
	Port       PortMap             `json:"port"`
	Protocol   map[string]string   `json:"protocol"`
	Vendor     map[string]string   `json:"vendor"`
	Ztun       map[string][]string `json:"ztun"`
}

type FallbackAgent struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
	Port uint16 `json:"port"`
}

type fallbackConfResponse struct {
	Agents    []FallbackAgent `json:"agents"`
	UpdatedAt int64           `json:"updated_ts"`
	TTL       int64           `json:"ttl_ms"`
}

type FallbackConfig struct {
	Agents    []FallbackAgent
	UpdatedAt time.Time
	TTL       time.Duration
}

func (c *FallbackConfig) UnmarshalJSON(data []byte) error {
	r := fallbackConfResponse{}
	err := json.Unmarshal(data, &r)
	if err != nil {
		return err
	}
	c.Agents = r.Agents
	c.UpdatedAt = time.Unix(r.UpdatedAt/1000, (r.UpdatedAt%1000)*1000000)
	c.TTL = time.Duration(r.TTL * 1000000)
	return nil
}

func (c *FallbackConfig) Expired() bool {
	return time.Now().After(c.UpdatedAt.Add(c.TTL))
}

func (c *FallbackConfig) ShuffleAgents() {
	rand.New(RandomSource).Shuffle(len(c.Agents), func(i, j int) {
		c.Agents[i], c.Agents[j] = c.Agents[j], c.Agents[i]
	})
}

func (c *FallbackConfig) Clone() *FallbackConfig {
	return &FallbackConfig{
		Agents:    append([]FallbackAgent(nil), c.Agents...),
		UpdatedAt: c.UpdatedAt,
		TTL:       c.TTL,
	}
}

func (a *FallbackAgent) ToProxy() *url.URL {
	return &url.URL{
		Scheme: "https",
		Host: net.JoinHostPort(a.Name+AGENT_SUFFIX,
			fmt.Sprintf("%d", a.Port)),
	}
}

func (a *FallbackAgent) Hostname() string {
	return a.Name + AGENT_SUFFIX
}

func (a *FallbackAgent) NetAddr() string {
	return net.JoinHostPort(a.IP, fmt.Sprintf("%d", a.Port))
}

func do_req(ctx context.Context, client *http.Client, method, url string, query, data url.Values) ([]byte, error) {
	var (
		req *http.Request
		err error
	)
	if method == "" {
		method = "GET"
	}
	if data == nil {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	} else {
		req, err = http.NewRequestWithContext(ctx,
			method,
			url,
			bytes.NewReader([]byte(data.Encode())))
	}
	if err != nil {
		return nil, err
	}
	if data != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if query != nil {
		req.URL.RawQuery = query.Encode()
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent:
	default:
		return nil, errors.New(fmt.Sprintf("Bad HTTP response: %s", resp.Status))
	}
	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	return body, nil
}

func VPNCountries(ctx context.Context, client *http.Client) (res CountryList, err error) {
	params := make(url.Values)
	params.Add("browser", EXT_BROWSER)
	data, err := do_req(ctx, client, "", VPN_COUNTRIES_URL, params, nil)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(data, &res)
	for _, a := range res {
		if a == "uk" {
			res = append(res, "gb")
		}
	}
	less := func(i, j int) bool { return res[i] < res[j] }
	unique.Slice(&res, less)
	return
}

func background_init(ctx context.Context, client *http.Client, extVer, user_uuid string) (res BgInitResponse, reterr error) {
	post_data := make(url.Values)
	post_data.Add("login", "1")
	post_data.Add("ver", extVer)
	qs := make(url.Values)
	qs.Add("uuid", user_uuid)
	resp, err := do_req(ctx, client, "POST", BG_INIT_URL, qs, post_data)
	if err != nil {
		reterr = err
		return
	}

	reterr = json.Unmarshal(resp, &res)
	if reterr == nil && res.Blocked {
		if res.Permanent {
			reterr = PermanentBanError
		} else {
			reterr = TemporaryBanError
		}
	}
	return
}

func zgettunnels(ctx context.Context,
	client *http.Client,
	user_uuid string,
	session_key int64,
	extVer string,
	country string,
	proxy_type string,
	limit uint) (res *ZGetTunnelsResponse, reterr error) {
	var tunnels ZGetTunnelsResponse
	params := make(url.Values)
	if proxy_type == "lum" {
		params.Add("country", country+".pool_lum_"+country+"_shared")
	} else if proxy_type == "virt" { // seems to be for brazil and japan only
		params.Add("country", country+".pool_virt_pool_"+country)
	} else if proxy_type == "peer" {
		//params.Add("country", country + ".peer")
		params.Add("country", country)
	} else if proxy_type == "pool" {
		params.Add("country", country+".pool")
	} else { // direct or skip
		params.Add("country", country)
	}
	params.Add("limit", strconv.FormatInt(int64(limit), 10))
	params.Add("ping_id", strconv.FormatFloat(rand.New(RandomSource).Float64(), 'f', -1, 64))
	params.Add("ext_ver", extVer)
	params.Add("browser", EXT_BROWSER)
	params.Add("product", PRODUCT)
	params.Add("uuid", user_uuid)
	params.Add("session_key", strconv.FormatInt(session_key, 10))
	params.Add("is_premium", "0")
	data, err := do_req(ctx, client, "", ZGETTUNNELS_URL, params, nil)
	if err != nil {
		reterr = err
		return
	}
	err = json.Unmarshal(data, &tunnels)
	if err != nil {
		return nil, fmt.Errorf("unable to unmashal zgettunnels response: %w", err)
	}
	if len(tunnels.IPList) == 0 {
		return nil, EmptyResponseError
	}
	res = &tunnels
	return
}

func fetchFallbackConfig(ctx context.Context) (*FallbackConfig, error) {
	client := httpClientWithProxy(nil)
	confRaw, err := do_req(ctx, client, "", FALLBACK_CONF_URL, nil, nil)
	if err != nil {
		return nil, err
	}

	l := len(confRaw)
	if l < 4 {
		return nil, errors.New("bad response length from fallback conf URL")
	}

	buf := &bytes.Buffer{}
	buf.Grow(l)
	buf.Write(confRaw[l-3:])
	buf.Write(confRaw[:l-3])

	b64dec := base64.NewDecoder(base64.RawStdEncoding, buf)
	jdec := json.NewDecoder(b64dec)
	fbc := &FallbackConfig{}

	err = jdec.Decode(fbc)
	if err != nil {
		return nil, err
	}

	if fbc.Expired() {
		return nil, errors.New("fetched expired fallback config")
	}

	fbc.ShuffleAgents()
	return fbc, nil
}

var (
	fbcMux    sync.Mutex
	cachedFBC *FallbackConfig
)

func GetFallbackProxies(ctx context.Context) (*FallbackConfig, error) {
	fbcMux.Lock()
	defer fbcMux.Unlock()

	var (
		fbc *FallbackConfig
		err error
	)

	if cachedFBC == nil || cachedFBC.Expired() {
		fbc, err = fetchFallbackConfig(ctx)
		if err != nil {
			return nil, err
		}
		cachedFBC = fbc
	} else {
		fbc = cachedFBC
	}

	return fbc.Clone(), nil
}

func Tunnels(ctx context.Context,
	logger *CondLogger,
	client *http.Client,
	extVer string,
	country string,
	proxy_type string,
	limit uint,
	timeout time.Duration,
	backoffInitial time.Duration,
	backoffDeadline time.Duration,
) (res *ZGetTunnelsResponse, user_uuid string, reterr error) {
	u := uuid.New()
	user_uuid = hex.EncodeToString(u[:])
	ctx1, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	initres, err := background_init(ctx1, client, extVer, user_uuid)
	if err != nil {
		reterr = err
		return
	}
	var bo backoff.BackOff = &backoff.ExponentialBackOff{
		InitialInterval:     backoffInitial,
		RandomizationFactor: 0.5,
		Multiplier:          1.5,
		MaxInterval:         10 * time.Minute,
		MaxElapsedTime:      backoffDeadline,
		Stop:                backoff.Stop,
		Clock:               backoff.SystemClock,
	}
	bo = backoff.WithContext(bo, ctx)
	err = backoff.RetryNotify(func() error {
		ctx1, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		res, reterr = zgettunnels(ctx1, client, user_uuid, initres.Key, extVer, country, proxy_type, limit)
		return reterr
	}, bo, func(err error, dur time.Duration) {
		logger.Info("zgettunnels error: %v; will retry after %v", err, dur.Truncate(time.Millisecond))
	})
	if err != nil {
		logger.Error("All attempts failed: %v", err)
		return nil, "", err
	}
	return
}

var baseDialer ContextDialer = &net.Dialer{
	Timeout:   30 * time.Second,
	KeepAlive: 30 * time.Second,
}

var tlsConfig *tls.Config

func UpdateHolaDialer(dialer ContextDialer) {
	baseDialer = dialer
}

func UpdateHolaTLSConfig(config *tls.Config) {
	tlsConfig = config
}

// Returns default http client with a proxy override
func httpClientWithProxy(agent *FallbackAgent) *http.Client {
	t := &http.Transport{
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	var dialer ContextDialer = baseDialer
	var rootCAs *x509.CertPool
	if tlsConfig != nil {
		rootCAs = tlsConfig.RootCAs
	}
	if agent != nil {
		dialer = NewProxyDialer(agent.NetAddr(), agent.Hostname(), rootCAs, nil, true, dialer)
	}
	t.DialContext = dialer.DialContext
	t.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		conn, err := dialer.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		var cfg tls.Config
		if tlsConfig != nil {
			cfg = *tlsConfig
		}
		cfg.ServerName = host
		conn = tls.UClient(conn, &cfg, tls.HelloChrome_Auto)
		if err := conn.(*tls.UConn).HandshakeContext(ctx); err != nil {
			return nil, err
		}
		return conn, nil
	}
	return &http.Client{
		Transport: t,
	}
}

func EnsureTransaction(ctx context.Context, getFBTimeout time.Duration, txn func(context.Context, *http.Client) bool) (bool, error) {
	client := httpClientWithProxy(nil)
	defer client.CloseIdleConnections()

	if txn(ctx, client) {
		return true, nil
	}

	// Fallback needed
	getFBCtx, cancel := context.WithTimeout(ctx, getFBTimeout)
	defer cancel()
	fbc, err := GetFallbackProxies(getFBCtx)
	if err != nil {
		return false, err
	}

	for _, agent := range fbc.Agents {
		client = httpClientWithProxy(&agent)
		defer client.CloseIdleConnections()
		if txn(ctx, client) {
			return true, nil
		}
	}

	return false, nil
}

func TemplateLogin(user_uuid string) string {
	var b strings.Builder
	LOGIN_TEMPLATE.Execute(&b, map[string]string{
		"uuid": user_uuid,
		"prem": "0",
	})
	return b.String()
}
