#!/usr/bin/env bash

# arguments <country> <proxytype(default direct, needs explicit country)> <port(default autogen using country+proxytype, needs explicit proxytype)>
country=${1-us}
proxytype=${2-direct}

if [ -z "$3" ]
then
	port=17160
	for x in {a..z}{a..z} # loop over all possible country codes (676 possibilities)
	do
		port=$((port+1))
		if [ "$x" == "$country" ]
		then
			true
			break
		else
			false
		fi
	done || { echo "country code $country is invalid" >&2; exit 1;}

	case $proxytype in # port range = 17160+1 -> 17160+676*5
		direct) port=$((676*0+port))                     ;;
		peer)   port=$((676*1+port))                     ;;
		lum)    port=$((676*2+port))                     ;;
		virt)   port=$((676*3+port))                     ;;
		pool)   port=$((676*4+port))                     ;;
		*)      echo "proxy-type $proxytype invalid" >&2
                        exit 1                                   ;;
	esac
else
	port=$3
fi

try_binary() {
	for x in "${@}"
	do
		type -a "$x" >/dev/null 2>&1 && { echo "$x"; return 0; } || false
	done || return 1
}

binary=$(try_binary "hola-proxy" "$HOME/go/bin/hola-proxy")
if [ -n "$binary" ]
then
	echo "country    $country"
	echo "proxytype  $proxytype"
	echo "proxy      127.0.0.1:$port"
	echo
	exec "$binary" -bind-address "127.0.0.1:$port" -country "$country" -proxy-type "$proxytype" -verbosity 50
else
	echo "hola-proxy binary cannot be found" >&2
	exit 1
fi
