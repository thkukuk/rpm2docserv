#!/bin/bash


VERSION=${VERSION:-$(git show -s --format=%cd --date=format:%Y%m%d%h%m)}
BUILDTIME=${BUILDTIME:-$(date +%Y-%m-%dT%TZ)}

sudo podman build --rm --no-cache --build-arg VERSION="${VERSION}" --build-arg BUILDTIME="${BUILDTIME}" -t docserv $@ .
