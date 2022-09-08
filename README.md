# rpm2docserv

## Goals

rpm2docserv extracts manual pages from RPM packages and makes them accessible in a web browser.
The result should be able to run in a container, so that every customer can run it's own instance.
Reading manpages should be possible to do without the need to login to a specific machine and convenience features (e.g. permalinks, URL redirects, easy navigation) should be available.

rpm2docserv is based on [debiman](https://github.com/Debian/debiman)

## Prerequisites

* mandoc
* zypper registred to the right product if not build in a container
or
* local RPM cache

## Build

### As container

This builds the binaries, fetches the RPMs and extracts the manual page and
builds a nginx container with the data:

```sh
sudo podman build -t docserv .
```

### From source on local system

This builds only the binaries

```sh
git clone https://github.com/thkukuk/rpm2docserv
cd rpm2docserv
make
```

## Run

### As container

This runs the container and makes the webserver available on port 80 and 443.
If no own certificates are provided, self signed ones will be generated:

```sh
sudo podman run -it --rm --name docserv -p 80:80 -p 443:443 localhost/docserv
```

The default path for certificates are:
```
        ssl_certificate      /etc/ssl/certs/nginx.crt;
        ssl_certificate_key  /etc/ssl/private/nginx.key;
        ssl_dhparam          /etc/nginx/dhparam.pem;
```

### From local directory

```sh
rpm2docserv
```

Will generate `/srv/docserv` with a default set of patterns resolved.

## Customization

A copy of the `assets/` directory can be created and modified. Start
`rpm2docserv` with `-inject-assets` pointing to the new directory.
Any files whose name does not end in .tmpl are treated as static files
and will be placed in -serving-dir uncompressed.

## TODO

* Optimze speed when gathering RPMs
* Broken manual pages are not rendered but still part of xref/auxserver
* https://www.sitemaps.org/ - Write extra tool to create sitemaps.xml files. Since this files contain the hostname, but we want to be independ in a container, create this files on demand when starting the container
