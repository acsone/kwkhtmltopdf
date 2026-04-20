# 1.1 (2026-04-20)

- Server: add `POST /image` for HTML → image via `wkhtmltoimage` (multipart API aligned
  with `/pdf`, default `format=png`, reject empty output). Prometheus metrics `image_*`.
  Environment variable `KWKHTMLTOIMAGE_BIN` overrides the binary path.

# 1.0 (2024-12-01)

- Dockerfile: add 0.12.6.1 image.
- Dockerfile: remove large temporary file.
- Server: add `/status` endpoint.

# 0.9.3 (2020-04-15)

Dockerfile: remove gsfonts and xfonts-100dpi. Use fonts-liberation2 instead as
fonts with the same metrics as Times, Arial and Courier.

# 0.9.2 (2020-04-15)

Dockerfile: add gsfonts and xfonts-100dpi

# 0.9.1 (2019-03-22)

Rewrite server in go.

# 0.9.0 (2019-03-16)

First release.
