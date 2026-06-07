package main

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// TODO ignore opts?
// --log-level, -q, --quiet, --read-args-from-stdin, --dump-default-toc-xsl
// --dump-outline <file>, --allow <path>, --cache-dir <path>,
// --disable-local-file-access, --enable-local-file-access

// TODO sensitive opts to be hidden from log
// --cookie <name> <value>, --password <password>,
// --ssl-key-password <password>

func wkhtmltopdfBin() string {
	bin := os.Getenv("KWKHTMLTOPDF_BIN")
	if bin != "" {
		return bin
	}
	return "wkhtmltopdf"
}

func wkhtmltoimageBin() string {
	bin := os.Getenv("KWKHTMLTOIMAGE_BIN")
	if bin != "" {
		return bin
	}
	return "wkhtmltoimage"
}

func isDocOption(arg string) bool {
	switch arg {
	case
		"-h",
		"--help",
		"-H",
		"--extended-help",
		"-V",
		"--version",
		"--readme",
		"--license",
		"--htmldoc",
		"--manpage":
		return true
	}
	return false
}

func httpError(w http.ResponseWriter, err error, code int) {
	log.Println(err)
	http.Error(w, err.Error(), code)
}

func httpAbort(w http.ResponseWriter, err error) {
	log.Println(err)
	// abort chunked encoding response as crude way to report error to client
	wh, ok := w.(http.Hijacker)
	if !ok {
		log.Println("cannot abort connection, error not reported to client: http.Hijacker not supported")
		return
	}
	c, _, err := wh.Hijack()
	if err != nil {
		log.Println("cannot abort connection, error not reported to client: ", err)
		return
	}
	c.Close()
}

type metricsResponseWriter struct {
	http.ResponseWriter
	bytes  int64
}

func (w *metricsResponseWriter) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *metricsResponseWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	return n, err
}

var (
	conversionsTotal = NewCounterVec(
		CounterOpts{
			Name: "kwkhtmltopdf_conversions_total",
			Help: "Total number of conversions attempted.",
		},
		[]string{"type", "domain", "result"},
	)
	conversionDurationSeconds = NewHistogramVec(
		HistogramOpts{
			Name:    "kwkhtmltopdf_conversion_duration_seconds",
			Help:    "Conversion duration in seconds.",
			Buckets: DefBuckets,
		},
		[]string{"type", "domain", "result"},
	)
	conversionOutputBytesTotal = NewCounterVec(
		CounterOpts{
			Name: "kwkhtmltopdf_conversion_output_bytes_total",
			Help: "Total number of bytes written in conversion responses.",
		},
		[]string{"type", "domain", "result"},
	)
)

func extractCookieDomainFromReportCookieJar(path string) (string, error) {
	// Read only a limited amount: cookie jar files are expected to be small.
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	// Some cookie jars can be multiline; search globally.
	content := string(data)
	idx := strings.Index(content, "domain=")
	if idx < 0 {
		return "", nil
	}
	rest := content[idx+len("domain="):]
	// Domain value ends at ';' or whitespace/newline.
	end := len(rest)
	if semi := strings.IndexByte(rest, ';'); semi >= 0 {
		end = semi
	}
	if ws := strings.IndexAny(rest, " \t\r\n"); ws >= 0 && ws < end {
		end = ws
	}
	domain := strings.TrimSpace(rest[:end])
	// Be defensive: normalize weird casing/spaces.
	domain = strings.Trim(domain, ".")
	return domain, nil
}

func redactArgs(args []string) []string {
	redacted := make([]string, 0, len(args))
	i := 0
	for i < len(args) {
		if args[i] == "--cookie" && i+2 < len(args) {
			redacted = append(redacted, args[i], args[i+1], "***")
			i += 3
		} else {
			redacted = append(redacted, args[i])
			i++
		}
	}
	return redacted
}

func handler(w http.ResponseWriter, r *http.Request) {
	mw := &metricsResponseWriter{ResponseWriter: w}
	result := "success"
	conversionType := ""
	domainLabel := "unknown"
	conversionStarted := false
	conversionStart := time.Time{}
	defer func() {
		if conversionStarted {
			conversionsTotal.WithLabelValues(conversionType, domainLabel, result).Inc()
			conversionDurationSeconds.WithLabelValues(conversionType, domainLabel, result).Observe(time.Since(conversionStart).Seconds())
			conversionOutputBytesTotal.WithLabelValues(conversionType, domainLabel, result).Add(float64(mw.bytes))
		}
	}()

	if r.URL.Path == "/status" {
		mw.WriteHeader(http.StatusOK)
		return
	} else if r.URL.Path == "/metrics" || r.URL.Path == "/metrics/" {
		MetricsHandler(mw, r)
		return
	} else {
		// don't log status
		log.Printf("%s %s", r.Method, r.URL.Path)
	}
	if r.Method != http.MethodPost {
		result = "error"
		httpError(mw, errors.New("http method not allowed: "+r.Method), http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/" && r.URL.Path != "/pdf" && r.URL.Path != "/image" {
		// handle /, /pdf, and /image, keep the rest for future use
		result = "error"
		httpError(mw, errors.New("path not found: "+r.URL.Path), http.StatusNotFound)
		return
	}

	// temp dir for files
	tmpdir, err := ioutil.TempDir("", "kwk")
	if err != nil {
		result = "error"
		httpError(mw, err, http.StatusNotFound)
		return
	}
	defer os.RemoveAll(tmpdir)

	// parse request
	reader, err := r.MultipartReader()
	if err != nil {
		result = "error"
		httpError(mw, err, http.StatusBadRequest)
		return
	}
	var docOutput bool
	var args []string
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			result = "error"
			httpError(mw, err, http.StatusBadRequest)
			return
		}
		if part.FormName() == "option" {
			buf := new(bytes.Buffer)
			buf.ReadFrom(part)
			arg := buf.String()
			args = append(args, arg)
			if isDocOption(arg) {
				docOutput = true
			}
		} else if part.FormName() == "file" {
			// It's important to preserve as much as possible of the
			// original filename because some javascript can depend on it
			// through document.location.
			path := filepath.Join(tmpdir, filepath.Base(part.FileName()))
			// TODO what if multiple files with same basename?
			file, err := os.Create(path)
			if err != nil {
				result = "error"
				httpError(mw, err, http.StatusBadRequest)
				return
			}
			_, err = io.Copy(file, part)
			file.Close()
			if err != nil {
				result = "error"
				httpError(mw, err, http.StatusBadRequest)
				return
			}
			if domainLabel == "unknown" {
				base := filepath.Base(part.FileName())
				if strings.HasPrefix(base, "report.cookie_jar") {
					domain, derr := extractCookieDomainFromReportCookieJar(path)
					if derr != nil {
						log.Println("failed to read cookie jar domain:", derr)
					} else if domain != "" {
						domainLabel = domain
					}
				}
			}
			args = append(args, path)
		} else {
			result = "error"
			httpError(mw, errors.New("unpexpected part name: "+part.FormName()), http.StatusBadRequest)
			return
		}
	}

	// determine if this is an image request
	isImageRequest := r.URL.Path == "/image"

	if docOutput {
		conversionType = "doc"
		mw.Header().Add("Content-Type", "text/plain")
	} else if isImageRequest {
		conversionType = "image"
		mw.Header().Add("Content-Type", "image/png")
		args = append(args, "-")
	} else {
		conversionType = "pdf"
		mw.Header().Add("Content-Type", "application/pdf")
		args = append(args, "-")
	}
	conversionStarted = true
	conversionStart = time.Now()

	var redactedArgs = redactArgs(args)

	log.Println(redactedArgs, "starting")

	var cmd *exec.Cmd
	if isImageRequest {
		cmd = exec.Command(wkhtmltoimageBin(), args...)
	} else {
		cmd = exec.Command(wkhtmltopdfBin(), args...)
	}
	cmdStdout, err := cmd.StdoutPipe()
	if err != nil {
		result = "error"
		httpError(mw, err, http.StatusInternalServerError)
		return
	}
	cmd.Stderr = os.Stderr
	err = cmd.Start()
	if err != nil {
		result = "error"
		httpError(mw, err, http.StatusInternalServerError)
		return
	}
	mw.WriteHeader(http.StatusOK)
	_, err = io.Copy(mw, cmdStdout)
	if err != nil {
		result = "abort"
		httpAbort(w, err)
		return
	}
	err = cmd.Wait()
	if err != nil {
		result = "abort"
		httpAbort(w, err)
		return
	}

	log.Println(redactedArgs, "success")
}

func main() {
	http.HandleFunc("/metrics", MetricsHandler)
	http.HandleFunc("/", handler)
	http.HandleFunc("/pdf", handler)
	http.HandleFunc("/image", handler)
	log.Println("kwkhtmltopdf server listening on port 8080")
	log.Println("Available endpoints: / (PDF), /pdf (PDF), /image (Image), /status (Health check), /metrics (Prometheus)")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
