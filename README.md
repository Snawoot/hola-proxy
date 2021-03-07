# hola-proxy

[![hola-proxy](https://snapcraft.io//hola-proxy/badge.svg)](https://snapcraft.io/hola-proxy)

Standalone Hola proxy client. Just run it and it'll start plain HTTP proxy server forwarding traffic via Hola proxies of your choice. By default application listens port on 127.0.0.1:8080.

Application is capable to forward traffic via proxies in datacenters (flag `-proxy-type direct`, default) or via peer proxies on residental IPs (consumer ISP) in that country (flag `-proxy-type pool` or `-proxy-type lum`).

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
* Zero-configuration

## Installation

#### Binary download

Pre-built binaries available on [releases](https://github.com/Snawoot/hola-proxy/releases/latest) page.

#### From source

Alternatively, you may install hola-proxy from source. Run within source directory

```
go install
```

#### Docker

Docker image is available as well. Here is an example for running proxy via DE as a background service:

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
$ ~/go/bin/hola-proxy -list-countries
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
$ ~/go/bin/hola-proxy -country de
```

Or run proxy on residental IP:

```
$ ~/go/bin/hola-proxy -country de -proxy-type lum
```

Also it is possible to export proxy addresses and credentials:

```
$ ~/go/bin/hola-proxy -country de -list-proxies -limit 3
Login: user-uuid-0a67c797b3214cbdb432b089c4b801cd
Password: cd123c465901
Proxy-Authorization: basic dXNlci11dWlkLTBhNjdjNzk3YjMyMTRjYmRiNDMyYjA4OWM0YjgwMWNkOmNkMTIzYzQ2NTkwMQ==

host,ip_address,direct,peer,hola,trial,trial_peer,vendor
zagent783.hola.org,165.22.22.6,22222,22223,22224,22225,22226,digitalocean
zagent830.hola.org,104.248.24.64,22222,22223,22224,22225,22226,digitalocean
zagent248.hola.org,165.22.65.3,22222,22223,22224,22225,22226,digitalocean
```

## Synopsis

```
$ ~/go/bin/hola-proxy -h
Usage of /home/user/go/bin/hola-proxy:
  -bind-address string
    	HTTP proxy listen address (default "127.0.0.1:8080")
  -country string
    	desired proxy location (default "us")
  -force-port-field string
    	force specific port field/num (example 24232 or lum)
  -limit uint
    	amount of proxies in retrieved list (default 3)
  -list-countries
    	list available countries and exit
  -list-proxies
    	output proxy list and exit
  -proxy-type string
    	proxy type: direct or peer or lum or virt or pool (default "direct")
  -resolver string
    	DNS/DoH/DoT resolver to workaround Hola blocked hosts. See https://github.com/ameshkov/dnslookup/ for upstream DNS URL format. (default "https://cloudflare-dns.com/dns-query")
  -rotate duration
    	rotate user ID once per given period (default 1h0m0s)
  -timeout duration
    	timeout for network operations (default 10s)
  -use-trial
    	use regular ports instead of trial ports (default true)
  -verbosity int
    	logging verbosity (10 - debug, 20 - info, 30 - warning, 40 - error, 50 - critical) (default 20)
```
