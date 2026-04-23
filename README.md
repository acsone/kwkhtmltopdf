# kwkhtmltopdf

A [wkhtmlpdf](https://wkhtmltopdf.org) server with drop-in client.

Why?

- avoid deploying wkhtmltopdf and it's dependencies in your application image
- keep the memory requirement of your application pods low while delegating
  memory hungry wkhtmltopdf jobs to dedicated pods
- easily select the wkhtmltopdf version to use at runtime

## WARNING

The server is not meant to be exposed to untrusted clients.

Several attack vectors exist (local file access being the most obvious).
Mitigating them is not a priority, since the main use case is
to use it as a private service.

## kwkhtmltopdf_server

A web server accepting [wkhtmlpdf](https://wkhtmltopdf.org) options and files
to convert as multipart form data.

It is written in go.

## kwkhtmltopdf_client

A drop-in replacement for [wkhtmlpdf](https://wkhtmltopdf.org) which invokes
the above server defined in the `KWKHTMLTOPDF_SERVER_URL` environment variable.

There are two clients:

- go clients (preferred):
  - PDF: `client/go/pdf/kwkhtmltopdf_client.go`
  - Image: `client/go/image/kwkhtmltoimage_client.go`
- a python client, which only depends on the `requests` library.
  It should work with any python version supported by `requests`.

## Quick start

### Run the server

```
$ docker run --rm -p 8080:8080 ghcr.io/acsone/kwkhtmltopdf:0.12.6.1-latest
```

or

```
$ go run server/kwkhtmltopdf_server.go
```

The server should now listen on http://localhost:8080.

#### Note for Apple Silicon users

The docker image is built for amd64. If you are on Apple Silicon,
you can use it by disabling the `Use Rosetta for x86_64/amd64 emulation on Apple Silicon` option
in the Docker Desktop general settings first.

### Run the client

Any of the following should generate a printout of the wkhtmltopdf home page to /tmp/test.pdf.

#### Using the built binary

```sh
$ go build -o client/go/kwkhtmltopdf_client client/go/pdf/kwkhtmltopdf_client.go
$ go build -o client/go/kwkhtmltoimage_client client/go/image/kwkhtmltoimage_client.go
$ env KWKHTMLTOPDF_SERVER_URL=http://localhost:8080 \
    client/go/kwkhtmltopdf_client https://wkhtmltopdf.org /tmp/test.pdf

$ env KWKHTMLTOPDF_SERVER_URL=http://localhost:8080 \
  client/go/kwkhtmltoimage_client https://wkhtmltopdf.org /tmp/test.png
```

#### Using the Go client

```sh
$ env KWKHTMLTOPDF_SERVER_URL=http://localhost:8080 \
  go run client/go/pdf/kwkhtmltopdf_client.go https://wkhtmltopdf.org /tmp/test.pdf

$ env KWKHTMLTOPDF_SERVER_URL=http://localhost:8080 \
  go run client/go/image/kwkhtmltoimage_client.go https://wkhtmltopdf.org /tmp/test.png
```

#### Using the Python client

```sh
$ env KWKHTMLTOPDF_SERVER_URL=http://localhost:8080 \
    client/python/kwkhtmltopdf_client.py https://wkhtmltopdf.org /tmp/test.pdf
```

## Run tests

1. Start the server.
2. Set and export `KWKHTMLTOPDF_SERVER_URL` environment variable.
3. Run `tox`.

This will run the same tests against the the native wkhtmltopdf executable,
as well as against the server using the python and go clients.

### Alternative test

Using "act" you can run the github action "test" locally

Note: requires docker and [act](https://github.com/nektos/act)

```sh
DOCKER_GRUOP=$(getent group docker | cut -d ":" -f 3)
act -W .github/workflows/test.yml -j test -P ubuntu-22.04=ghcr.io/catthehacker/ubuntu:full-22.04 --container-options "--privileged --group-add $DOCKER_GRUOP" --container-daemon-socket unix:///var/run/docker.sock --container-architecture linux/amd64
```

If you need to test a new server side you need to build it locally first

```sh
docker build -f Dockerfile-0.12.6.2 -t kwkhtmltopdf:0.12.6.2 .
```

## Roadmap

See [issues on GitHub](<https://github.com/acsone/kwkhtmltopdf/issues>)
as well as some TODO's in the source code.

## Releasing

Push the master branch and ensure tests pass on GitHub.

Build the go client and server as explained above. Create and tag a release on GitHub
and attach the client and server you just built to it.

Images are built and pushed to ghcr.io by a GitHub action.

## Credits

Author: stephane.bidoul@acsone.eu.

Contributors are visible on
[GitHub](https://github.com/acsone/kwkhtmltopdf/graphs/contributors).

## License

MIT
