# hola-proxy

[![hola-proxy](https://snapcraft.io//hola-proxy/badge.svg)](https://snapcraft.io/hola-proxy)

Standalone Hola proxy client. Just run it and it'll start a plain HTTP proxy server forwarding traffic through Hola proxies of your choice.
By default the application listens on 127.0.0.1:8080.

Application is capable to forward traffic via proxies in datacenters (flag `-proxy-type direct`, default) or via peer proxies on residental IPs (consumer ISP) in that country (flag `-proxy-type lum`).

---

:heart: :heart: :heart:

You can say thanks to the author by donations to these wallets:

- ETH: `0xB71250010e8beC90C5f9ddF408251eBA9dD7320e`
- BTC:
  - Legacy: `1N89PRvG1CSsUk9sxKwBwudN6TjTPQ1N8a`
  - Segwit: `bc1qc0hcyxc000qf0ketv4r44ld7dlgmmu73rtlntw`

---

## Mirrors

IPFS git mirror:

```
git clone https://ipfs.io/ipns/k51qzi5uqu5dkrgx0hozpy1tlggw5o0whtquyrjlc6pprhvbmczr6qtj4ocrv0 hola-proxy
```

## Features

* Cross-platform (Windows/Mac OS/Linux/Android (via shell)/\*BSD)
* Uses TLS for secure communication with upstream proxies
* Zero configuration
* Simple and straight forward

## Installation

#### Binaries

Pre-built binaries are available [here](https://github.com/Snawoot/hola-proxy/releases/latest).

Don't forget to make file executable on Unix-like systems (Linux, MacOS, \*BSD, Android). For your convenience rename downloaded file to `hola-proxy` and run within directory where you placed it:

```sh
chmod +x hola-proxy
```

#### Build from source

Alternatively, you may install hola-proxy from source. Run the following within the source directory:

```
make install
```

#### Docker

A docker image is available as well. Here is an example of running hola-proxy via DE as a background service:

```sh
docker run -d \
    --security-opt no-new-privileges \
    -p 127.0.0.1:8080:8080 \
    --restart unless-stopped \
    --name hola-proxy \
    yarmak/hola-proxy -country de
```

#### Snap Store

[![Get it from the Snap Store](https://snapcraft.io/static/images/badges/en/snap-store-black.svg)](https://snapcraft.io/hola-proxy)

```bash
sudo snap install hola-proxy
```

## Usage

List available countries:

```
$ ./hola-proxy -list-countries
ar - Argentina
at - Austria
au - Australia
be - Belgium
bg - Bulgaria
br - Brazil
ca - Canada
ch - Switzerland
cl - Chile
co - Colombia
cz - Czech Republic
de - Germany
dk - Denmark
es - Spain
fi - Finland
fr - France
gb - United Kingdom (Great Britain)
gr - Greece
hk - Hong Kong
hr - Croatia
hu - Hungary
id - Indonesia
ie - Ireland
il - Israel
in - India
is - Iceland
it - Italy
jp - Japan
kr - Korea, Republic of
mx - Mexico
nl - Netherlands
no - Norway
nz - New Zealand
pl - Poland
ro - Romania
ru - Russian Federation
se - Sweden
sg - Singapore
sk - Slovakia
tr - Turkey
uk - United Kingdom
us - United States of America
```

Run proxy via country of your choice:

```
$ ./hola-proxy -country de
```

Or run proxy on residential IP:

```
$ ./hola-proxy -proxy-type lum
```

Also it is possible to export proxy addresses and credentials:

```
$ ./hola-proxy -country de -list-proxies -limit 3
Login: user-uuid-0a67c797b3214cbdb432b089c4b801cd
Password: cd123c465901
Proxy-Authorization: basic dXNlci11dWlkLTBhNjdjNzk3YjMyMTRjYmRiNDMyYjA4OWM0YjgwMWNkOmNkMTIzYzQ2NTkwMQ==

host,ip_address,direct,peer,hola,trial,trial_peer,vendor
zagent783.hola.org,165.22.22.6,22222,22223,22224,22225,22226,digitalocean
zagent830.hola.org,104.248.24.64,22222,22223,22224,22225,22226,digitalocean
zagent248.hola.org,165.22.65.3,22222,22223,22224,22225,22226,digitalocean
```

## List of arguments

| Argument | Type | Description |
| -------- | ---- | ----------- |
| backoff-deadline | Duration | total duration of zgettunnels method attempts (default 5m0s) |
| backoff-initial | Duration | initial average backoff delay for zgettunnels (randomized by +/-50%) (default 3s) |
| bind-address | String | HTTP proxy address to listen to (default "127.0.0.1:8080") |
| cafile | String | use custom CA certificate bundle file |
| country | String | desired proxy location (default "us") |
| dont-use-trial | - | use regular ports instead of trial ports |
| ext-ver | String | extension version to mimic in requests. Can be obtained from https://chrome.google.com/webstore/detail/hola-vpn-the-website-unbl/gkojfkhlekighikafcpjkiklfbnlmeio (default "999.999.999") |
| force-port-field | Number | force specific port field/num (example 24232 or lum) |
| limit | Unsigned Integer (Number) | amount of proxies in retrieved list (default 3) |
| list-countries | String | list available countries and exit |
| list-proxies | - | output proxy list and exit |
| proxy | String | sets base proxy to use for all dial-outs. Format: `<http\|https\|socks5\|socks5h>://[login:password@]host[:port]` Examples: `http://user:password@192.168.1.1:3128`, `socks5://10.0.0.1:1080` |
| proxy-type | String | proxy type (Datacenter: direct) (Residential: lum) (default "direct") |
| resolver | String | DNS/DoH/DoT resolver to workaround Hola blocked hosts. See https://github.com/ameshkov/dnslookup/ for upstream DNS URL format. (default "https://cloudflare-dns.com/dns-query") |
| rotate | Duration | rotate user ID once per given period (default 1h0m0s) |
| timeout | Duration | timeout for network operations (default 35s) |
| verbosity | Number | logging verbosity (10 - debug, 20 - info, 30 - warning, 40 - error, 50 - critical) (default 20) |

## See also

* [Project wiki](https://github.com/Snawoot/hola-proxy/wiki)
