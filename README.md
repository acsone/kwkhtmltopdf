# kwkhtmltopdf_server

A web server accepting wkhtmlpdf options and files to convert
as multipart form data. 

It requires python 3.6 or greater.

# kwkhtmltopdf_client

A drop-in replacement for wkhtmltopdf, which invokes the above server
defined in the KWKHTMLTOPDF_SERVER_URL environment variable.

It's only dependency is the `requests` library.
It should work with any python version supported by `requests`.

# TODO

- more tests
- see TODO source code for some (important) improvements

# WARNING

The server is not meant to be exposed to untrusted clients.

Several attack vectors exist (local file access being the most obvious).
Mitigating them is not currently a priority, since the main use case is
to use it as a private service.

# Credits

Author: stephane.bidoul@acsone.eu.

# License

MIT
