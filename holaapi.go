package main

import (
	"bytes"
	"context"
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
	"sync"
	"time"

	"github.com/campoy/unique"
	"github.com/google/uuid"
)

const USER_AGENT = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/89.0.4389.72 Safari/537.36"
const EXT_VER = "1.181.350"
const EXT_BROWSER = "chrome"
const PRODUCT = "cws"
const CCGI_URL = "https://client.hola.org/client_cgi/"
const VPN_COUNTRIES_URL = CCGI_URL + "vpn_countries.json"
const BG_INIT_URL = CCGI_URL + "background_init"
const ZGETTUNNELS_URL = CCGI_URL + "zgettunnels"
const LOGIN_PREFIX = "user-uuid-"
const FALLBACK_CONF_URL = "https://www.dropbox.com/s/jemizcvpmf2qb9v/cloud_failover.conf?dl=1"
const AGENT_SUFFIX = ".hola.org"

var TemporaryBanError = errors.New("temporary ban detected")
var PermanentBanError = errors.New("permanent ban detected")

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

func (c *FallbackConfig) ToProxies() []*url.URL {
	res := make([]*url.URL, 0, len(c.Agents))
	for _, agent := range c.Agents {
		url := &url.URL{
			Scheme: "https",
			Host: net.JoinHostPort(agent.Name+AGENT_SUFFIX,
				fmt.Sprintf("%d", agent.Port)),
		}
		res = append(res, url)
	}
	return res
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
	req.Header.Set("User-Agent", USER_AGENT)
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

func background_init(ctx context.Context, client *http.Client, user_uuid string) (res BgInitResponse, reterr error) {
	post_data := make(url.Values)
	post_data.Add("login", "1")
	post_data.Add("ver", EXT_VER)
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
	params.Add("ext_ver", EXT_VER)
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
	reterr = json.Unmarshal(data, &tunnels)
	res = &tunnels
	return
}

func fetchFallbackConfig(ctx context.Context) (*FallbackConfig, error) {
	confRaw, err := do_req(ctx, &http.Client{}, "", FALLBACK_CONF_URL, nil, nil)
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

func GetFallbackProxies(ctx context.Context) ([]*url.URL, error) {
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
	} else {
		fbc = cachedFBC
	}

	return fbc.ToProxies(), nil
}

func Tunnels(ctx context.Context,
	client *http.Client,
	country string,
	proxy_type string,
	limit uint) (res *ZGetTunnelsResponse, user_uuid string, reterr error) {
	u := uuid.New()
	user_uuid = hex.EncodeToString(u[:])
	initres, err := background_init(ctx, client, user_uuid)
	if err != nil {
		reterr = err
		return
	}
	res, reterr = zgettunnels(ctx, client, user_uuid, initres.Key, country, proxy_type, limit)
	return
}

// Returns default http client with a proxy override
func httpClientWithProxy(proxy *url.URL) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxy),
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

func EnsureTransaction(baseCtx context.Context, txnTimeout time.Duration, txn func(context.Context, *http.Client) bool) (bool, error) {
	client := httpClientWithProxy(nil)
	defer client.CloseIdleConnections()

	ctx, cancel := context.WithTimeout(baseCtx, txnTimeout)
	defer cancel()

	if txn(ctx, client) {
		return true, nil
	}

	// Fallback needed
	proxies, err := GetFallbackProxies(baseCtx)
	if err != nil {
		return false, err
	}

	for _, proxy := range proxies {
		client = httpClientWithProxy(proxy)
		defer client.CloseIdleConnections()

		ctx, cancel = context.WithTimeout(baseCtx, txnTimeout)
		defer cancel()

		if txn(ctx, client) {
			return true, nil
		}
	}

	return false, nil
}
