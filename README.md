# hola-proxy

Standalone Hola proxy client. Just run it and it'll start plain HTTP proxy server forwarding traffic via Hola proxies of your choice. By default application listens port on 127.0.0.1:8080.

Application is capable to forward traffic via proxies in datacenters (flag `-proxy-type direct`, default) or via peer proxies on residental IPs (consumer ISP) in that country (flag `-proxy-type peer`).

## Features

* Cross-platform (Windows/Mac OS/Linux/Android (via shell)/\*BSD)
* Uses TLS for secure communication with upstream proxies
* Zero-configuration

## Installation

Pre-built binaries available on [releases](https://github.com/Snawoot/hola-proxy/releases/latest) page.

Alternatively, you may install hola-proxy from source. Run within source directory

```
go install
```

Docker image is available as well. Here is an example for running proxy via DE as a background service:

```sh
docker run -d \
    --security-opt no-new-privileges \
    -p 127.0.0.1:8080:8080 \
    --restart unless-stopped \
    --name hola-proxy \
    yarmak/hola-proxy -country de
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
gr - Greece
hk - Hong Kong
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
$ ~/go/bin/hola-proxy -country de -proxy-type peer
```

Also it is possible to export proxy addresses and credentials:

```
$ ~/go/bin/hola-proxy -country de -list-proxies -limit 3
Login: user-uuid-f4c2c3a8657640048e7243a807867d52
Password: e194c4f457e0
Proxy-Authorization: basic dXNlci11dWlkLWY0YzJjM2E4NjU3NjQwMDQ4ZTcyNDNhODA3ODY3ZDUyOmUxOTRjNGY0NTdlMA==

Host,IP address,Direct port,Peer port,Vendor
zagent90.hola.org,185.72.246.203,22222,22223,nqhost
zagent249.hola.org,165.22.80.107,22222,22223,digitalocean
zagent248.hola.org,165.22.65.3,22222,22223,digitalocean
```

## Synopsis

```
$ ~/go/bin/hola-proxy -h
Usage of /home/user/go/bin/hola-proxy:
  -bind-address string
    	HTTP proxy listen address (default "127.0.0.1:8080")
  -country string
    	desired proxy location (default "us")
  -limit uint
    	amount of proxies in retrieved list (default 3)
  -list-countries
    	list available countries and exit
  -list-proxies
    	output proxy list and exit
  -proxy-type string
    	proxy type: direct or peer (default "direct")
  -resolver string
    	DNS/DoH/DoT resolver to workaround Hola blocked hosts. See https://github.com/ameshkov/dnslookup/ for upstream DNS URL format. (default "https://cloudflare-dns.com/dns-query")
  -rotate duration
    	rotate user ID once per given period (default 1h0m0s)
  -timeout duration
    	timeout for network operations (default 10s)
  -verbosity int
    	logging verbosity (10 - debug, 20 - info, 30 - warning, 40 - error, 50 - critical) (default 20)

```
