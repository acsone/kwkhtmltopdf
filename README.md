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

- tests
- container images for the server and various wkhtmltopdf versions
- see TODO source code for some (important) improvements

# Credits

Author: stephane.bidoul@acsone.eu.

# License

MIT
