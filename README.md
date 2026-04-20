# kwkhtmltopdf

A [wkhtmlpdf](https://wkhtmltopdf.org) server.

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

## Usage

cURL command - 
```curl
  curl --location 'http://localhost:8081/pdf' \
   --header 'X-Trace-ID: 123' \
   --form 'file=@"/Users/kshitizagrawal/Desktop/repositories/docker-wkhtmltopdf-aas/assets/header.html"' \
   --form 'file=@"/Users/kshitizagrawal/Desktop/repositories/docker-wkhtmltopdf-aas/assets/index.html"' \
   --form 'file=@"/Users/kshitizagrawal/Desktop/repositories/docker-wkhtmltopdf-aas/assets/footer.html"' \
   --form 'margin-top="20"' \
   --form 'page-size="A4"' \
   --form 'margin-bottom="10"' \
   --output "output.pdf"
```


## Quick start

### Run the server

```
$ docker compose up
```

The server should now listen on http://localhost:8080.

#### Note for Apple Silicon users

The docker image is built for amd64. If you are on Apple Silicon,
you can use it by disabling the `Use Rosetta for x86_64/amd64 emulation on Apple Silicon` option
in the Docker Desktop general settings first.

## HTML to image (`POST /image`)

The server also exposes **`POST /image`** for HTML → raster image using
[`wkhtmltoimage`](https://wkhtmltopdf.org) (same packaging as `wkhtmltopdf` in the Docker images).

- **Multipart** works like `/pdf`: `file` parts use the **basename** of the filename; other fields become `--<name>` and optional value (empty value = flag only).
- You must upload a **`index.html`** file part. Extra parts such as `header.html` are written to the temp dir but only **`index.html`** is passed as the main input to `wkhtmltoimage`.
- Common options via form fields: `format`, `width`, `height`, `quality` (mapped to `wkhtmltoimage` CLI options). If **`format` is omitted**, the server defaults to **`png`**.
- The server appends **`--enable-local-file-access`**, runs `wkhtmltoimage`, and returns the image bytes. **`Content-Type`** reflects the format (e.g. `image/png`, `image/jpeg`). Success with an **empty** output file is rejected with HTTP **500**.
- Override the binary with **`KWKHTMLTOIMAGE_BIN`** (default: `wkhtmltoimage` on `PATH`). **`KWKHTMLTOPDF_BIN`** is unchanged for `/pdf`.

Prometheus metrics for this route use the **`image_*`** names (`image_requests_total`, `image_request_duration_seconds`, `image_active_requests`, `image_errors_total`, `image_size_bytes`).

Example:

```bash
curl --location 'http://localhost:18080/image' \
  --header 'X-Trace-ID: 123' \
  --form 'file=@index.html;filename=index.html' \
  --form 'format=png' \
  --form 'width=1024' \
  --output out.png
```

Local sample (read HTML from disk, call **`POST /image`**, save **`samples/hello-image-output.png`**): see **`samples/README.md`** and run **`go run ./cmd/htmltoimagesample`** while **`docker compose up`** is running.

Tests:

```bash
go test ./... -count=1 -race
```

Optional integration test (real `wkhtmltoimage` on `PATH`):

```bash
WKHTMLTOIMAGE_INTEGRATION=1 go test ./server/... -run TestWkhtmltoimage_integrationRealBinary -v
```

## Releasing

### Login to ECR

```sh
$ aws ecr get-login-password --region ap-south-1 | docker login --username AWS --password-stdin 909798297030.dkr.ecr.ap-south-1.amazonaws.com
```

### Build and push to ECR

```sh
$ docker buildx build -f Dockerfile-0.12.6.1 --platform linux/x86_64 --load --tag wkhtmltopdf-x86_64:0.0.17 .
$ docker tag wkhtmltopdf-x86_64:0.0.17 909798297030.dkr.ecr.ap-south-1.amazonaws.com/wkhtmltopdf-x86_64:0.0.17
$ docker push 909798297030.dkr.ecr.ap-south-1.amazonaws.com/wkhtmltopdf-x86_64:0.0.17
```

## Credits

Author: stephane.bidoul@acsone.eu.

Contributors are visible on
[GitHub](https://github.com/acsone/kwkhtmltopdf/graphs/contributors).

## License

MIT
