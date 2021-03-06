package main

import (
    "context"
    "net/http"
    "net/url"
    "io/ioutil"
    "encoding/json"
    "encoding/hex"
    "github.com/google/uuid"
    "bytes"
    "strconv"
    "math/rand"
    "github.com/campoy/unique"
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

type CountryList []string

type BgInitResponse struct {
    Ver string `json:"ver"`
    Key int64 `json:"key"`
    Country string `json:"country"`
}

type PortMap struct {
    Direct uint16 `json:"direct"`
    Hola uint16 `json:"hola"`
    Peer uint16 `json:"peer"`
    Trial uint16 `json:"trial"`
    TrialPeer uint16 `json:"trial_peer"`
}

type ZGetTunnelsResponse struct {
    AgentKey string `json:"agent_key"`
    AgentTypes map[string]string `json:"agent_types"`
    IPList map[string]string `json:"ip_list"`
    Port PortMap `json:"port"`
    Protocol map[string]string `json:"protocol"`
    Vendor map[string]string `json:"vendor"`
    Ztun map[string][]string `json:"ztun"`
}

func do_req(ctx context.Context, method, url string, query, data url.Values) ([]byte, error) {
    var (
        client http.Client
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
    body, err := ioutil.ReadAll(resp.Body)
    resp.Body.Close()
    if err != nil {
        return nil, err
    }
    return body, nil
}

func VPNCountries(ctx context.Context) (res CountryList, err error) {
    params := make(url.Values)
    params.Add("browser", EXT_BROWSER)
    data, err := do_req(ctx, "", VPN_COUNTRIES_URL, params, nil)
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

func background_init(ctx context.Context, user_uuid string) (res BgInitResponse, reterr error) {
    post_data := make(url.Values)
    post_data.Add("login", "1")
    post_data.Add("ver", EXT_VER)
    qs := make(url.Values)
    qs.Add("uuid", user_uuid)
    resp, err := do_req(ctx, "POST", BG_INIT_URL, qs, post_data)
    if err != nil {
        reterr = err
        return
    }
    reterr = json.Unmarshal(resp, &res)
    return
}

func zgettunnels(ctx context.Context,
                 user_uuid string,
                 session_key int64,
                 country string,
                 proxy_type string,
                 limit uint) (res *ZGetTunnelsResponse, reterr error) {
    var tunnels ZGetTunnelsResponse
    params := make(url.Values)
    if proxy_type == "lum" {
        params.Add("country", country + ".pool_lum_" + country + "_shared")
    } else if proxy_type == "peer" {
    	params.Add("country", country + ".peer")
    } else {
    	params.Add("country", country)
    }
    params.Add("limit", strconv.FormatInt(int64(limit), 10))
    params.Add("ping_id", strconv.FormatFloat(rand.Float64(), 'f', -1, 64))
    params.Add("ext_ver", EXT_VER)
    params.Add("browser", EXT_BROWSER)
    params.Add("product", PRODUCT)
    params.Add("uuid", user_uuid)
    params.Add("session_key", strconv.FormatInt(session_key, 10))
    params.Add("is_premium", "0")
    data, err := do_req(ctx, "", ZGETTUNNELS_URL, params, nil)
    if err != nil {
        reterr = err
        return
    }
    reterr = json.Unmarshal(data, &tunnels)
    res = &tunnels
    return
}

func Tunnels(ctx context.Context,
             country string,
             proxy_type string,
             limit uint) (res *ZGetTunnelsResponse, user_uuid string, reterr error) {
    u := uuid.New()
    user_uuid = hex.EncodeToString(u[:])
    initres, err := background_init(ctx, user_uuid)
    if err != nil {
        reterr = err
        return
    }
    res, reterr = zgettunnels(ctx, user_uuid, initres.Key, country, proxy_type, limit)
    return
}
