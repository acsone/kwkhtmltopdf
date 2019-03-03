# kwkhtmltopdf_server

A web server accepting [wkhtmlpdf](https://wkhtmltopdf.org) options and files
to convert as multipart form data.

It requires python 3.6 or greater.

# kwkhtmltopdf_client

A drop-in replacement for [wkhtmlpdf](https://wkhtmltopdf.org) which invokes
the above server defined in the `KWKHTMLTOPDF_SERVER_URL` environment variable.

It's only dependency is the `requests` library.
It should work with any python version supported by `requests`.

# Quick start

## Run the server

```
$ docker run -it --rm -p 8080:8080 acsone/kwhkhtmltopdf
```

The server should now listen on http://localhost:8080.

## Run the client

```
$ env KWKHTMLTOPDF_SERVER_URL=http://localhost:8080 \
    client/python/kwkhtmltopdf_client.py https://wkhtmltopdf.org /tmp/test.pdf
```

This should generate a printout of the wkhtmltopdf home page to /tmp/test.pdf.

# Run tests

1. Start the server.
2. Set and export `KWKHTMLTOPDF_SERVER_URL` environment variable.
3. Run `tox`.

# TODO

- A few more tests.
- See also some TODO in source code (most important one is probably
  detection of file arguments in the client).
- Write an alternative client (in go?) that is easier to deploy
  and starts faster than the python one.

# WARNING

The server is not meant to be exposed to untrusted clients.

Several attack vectors exist (local file access being the most obvious).
Mitigating them is not a priority, since the main use case is
to use it as a private service.

# Credits

Author: stephane.bidoul@acsone.eu.

# License

MIT
