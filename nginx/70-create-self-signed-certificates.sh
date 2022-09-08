#!/bin/bash

set -e

DEBUG=${DEBUG:-"0"}
[ "${DEBUG}" = "1" ] && set -x


CRT=$(grep "ssl_certificate[[:space:]]" /etc/nginx/nginx.conf  |awk '{print $2}' |sed -e 's|;||g')
KEY=$(grep "ssl_certificate_key[[:space:]]" /etc/nginx/nginx.conf  |awk '{print $2}' |sed -e 's|;||g')

if [ ! -f "$CRT" ] && [ ! -f "$KEY" ]; then

    # Some defaults
    SSL_CA_CSR_COUNTRY=${SSL_CA_CSR_COUNTRY:-"DE"}
    SSL_CA_CSR_STATE=${SSL_CA_CSR_STATE:-"Bavaria"}
    SSL_CA_CSR_ORGANIZATION_UNIT=${SSL_CA_CSR_ORGANIZATION_UNIT:-"Dummy CA"}
    SSL_CA_CSR_CN=${SSL_CA_CSR_CN:-"$(hostname -f)"}
    SSL_ORGANIZATION_UNIT=${SSL_ORGANIZATION_UNIT:-"Server Certificate"}

    mkdir -p "$(dirname "$CRT")"
    mkdir -p "$(dirname "$KEY")"

    openssl req -x509 -nodes -days 365 -newkey rsa:2048 -subj "/C=${SSL_CA_CSR_COUNTRY}/ST=${SSL_CA_CSR_STATE}/O=${SSL_CA_CSR_ORGANIZATION_UNIT}/CN=${SSL_CA_CSR_CN}" -keyout "$KEY" -out "$CRT"

    #test -f /etc/nginx/dhparam.pem || openssl dhparam -out /etc/nginx/dhparam.pem 4096
fi
