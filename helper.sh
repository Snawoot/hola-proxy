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

echo "country    $country"
echo "proxytype  $proxytype"
echo "proxy      127.0.0.1:$port"
echo
exec hola-proxy -bind-address "127.0.0.1:$port" -country "$country" -proxy-type "$proxytype" -verbosity 50
