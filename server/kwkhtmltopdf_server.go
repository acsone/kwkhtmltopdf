package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.statusCode = code
	sr.ResponseWriter.WriteHeader(code)
}

func wkhtmltopdfBin() string {
	bin := os.Getenv("KWKHTMLTOPDF_BIN")
	if bin != "" {
		return bin
	}
	return "wkhtmltopdf"
}

func httpError(ctx context.Context, w http.ResponseWriter, err error, code int) {
	logger := loggerFromContext(ctx)

	logger.Errorf("HTTP error: %v", err)

	if sr, ok := w.(*statusRecorder); ok {
		sr.statusCode = code
	}

	http.Error(w, err.Error(), code)
}

func httpAbort(ctx context.Context, w http.ResponseWriter, err error) {
	logger := loggerFromContext(ctx)

	logger.Errorf("HTTP abort: %v", err)

	if sr, ok := w.(*statusRecorder); ok {
		sr.statusCode = http.StatusInternalServerError
	}

	wh, ok := w.(http.Hijacker)
	if !ok {
		errorTotal.WithLabelValues("hijack_unsupported", err.Error()).Inc()
		logger.Errorln("cannot abort connection, error not reported to client: http.Hijacker not supported")
		return
	}
	c, _, err := wh.Hijack()
	if err != nil {
		errorTotal.WithLabelValues("hijack_failed", err.Error()).Inc()
		logger.Errorln("cannot abort connection, error not reported to client: ", err)
		return
	}
	c.Close()
}
func pdfHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	logger := loggerFromContext(ctx)

	if r.Method != http.MethodPost {
		errorTotal.WithLabelValues("method_not_allowed", r.Method).Inc()
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()
	activeRequests.Inc()
	defer activeRequests.Dec()

	rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
	defer func() {
		duration := time.Since(start).Seconds()
		requestDuration.WithLabelValues(r.URL.Path).Observe(duration)
		requestsTotal.WithLabelValues(r.URL.Path, fmt.Sprintf("%d", rec.statusCode)).Inc()
	}()

	tmpdir, err := os.MkdirTemp("", "kwk")
	if err != nil {
		errorTotal.WithLabelValues("tempdir_creation_failed", err.Error()).Inc()
		httpError(ctx, w, err, http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpdir)

	logger.Infof("Temporary directory created: %s", tmpdir)

	reader, err := r.MultipartReader()
	if err != nil {
		errorTotal.WithLabelValues("multipart_reader_creation_failed", err.Error()).Inc()
		logger.Errorf("Failed to create multipart reader: %v", err)
		httpError(ctx, w, err, http.StatusBadRequest)
		return
	}

	args, endArgs, indexPath, err := parseMultipartForm(ctx, reader, tmpdir)
	if err != nil {
		errorTotal.WithLabelValues("parse_multipart_form_failed", err.Error()).Inc()
		logger.Errorf("Failed to parse multipart form: %v", err)
		httpError(ctx, w, err, http.StatusBadRequest)
		return
	}

	if indexPath == "" {
		errorTotal.WithLabelValues("index_html_file_not_found", "").Inc()
		logger.Errorln("index.html file is required but not found")
		httpError(ctx, w, errors.New("index.html file is required"), http.StatusBadRequest)
		return
	}

	endArgs = append(endArgs, indexPath)
	args = append(args, endArgs...)

	runWkhtmltopdf(ctx, rec, args)
}

func parseMultipartForm(ctx context.Context, reader *multipart.Reader, tmpdir string) (args []string, endArgs []string, indexPath string, err error) {
	logger := loggerFromContext(ctx)

	defer func() {
		// Track errors
		if err != nil {
			errorTotal.WithLabelValues("parse_multipart_form", err.Error()).Inc()
		}
	}()

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			logger.Errorln(err)
			return nil, nil, "", err
		}

		if part.FormName() == "file" {
			path := filepath.Join(tmpdir, filepath.Base(part.FileName()))
			file, err := os.Create(path)
			if err != nil {
				logger.Errorln(err)
				return nil, nil, "", err
			}
			_, err = io.Copy(file, part)
			file.Close()
			if err != nil {
				logger.Errorln(err)
				return nil, nil, "", err
			}

			switch part.FileName() {
			case "header.html":
				endArgs = append(endArgs, "--header-html", path)
			case "footer.html":
				endArgs = append(endArgs, "--footer-html", path)
			case "index.html":
				indexPath = path
			}
		} else {
			buf := new(bytes.Buffer)
			buf.ReadFrom(part)
			arg := buf.String()
			if arg == "" {
				args = append(args, fmt.Sprintf("--%s", part.FormName()))
			} else {
				args = append(args, fmt.Sprintf("--%s", part.FormName()), arg)
			}
		}
	}

	return args, endArgs, indexPath, nil
}

func runWkhtmltopdf(ctx context.Context, w http.ResponseWriter, args []string) {
	logger := loggerFromContext(ctx)

	args = append(args, "--enable-local-file-access") // https://github.com/wkhtmltopdf/wkhtmltopdf/issues/4460#issuecomment-661345113
	args = append(args, "-")
	logger.Infoln("Args", args)

	logger.Infoln("Starting wkhtmltopdf process")
	cmd := exec.Command(wkhtmltopdfBin(), args...)
	cmdStdout, err := cmd.StdoutPipe()
	if err != nil {
		errorTotal.WithLabelValues("stdout_pipe_failed", err.Error()).Inc()
		httpError(ctx, w, err, http.StatusInternalServerError)
		return
	}
	cmd.Stderr = os.Stderr
	done := make(chan error, 1)

	err = cmd.Start()
	if err != nil {
		errorTotal.WithLabelValues("process_start_failed", err.Error()).Inc()
		httpError(ctx, w, err, http.StatusInternalServerError)
		return
	}

	logger.Infoln("wkhtmltopdf process started")

	// Buffer the output
	var pdfBuffer bytes.Buffer

	go func() {
		_, err := io.Copy(&pdfBuffer, cmdStdout)
		if err != nil {
			errorTotal.WithLabelValues("copy_output_failed", err.Error()).Inc()
			logger.Errorf("Error copying command output: %v", err)
			done <- err
			return
		}
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		if ctx.Err() != nil {
			errorTotal.WithLabelValues("context_cancelled", ctx.Err().Error()).Inc()
		}
		logger.Errorln("Context cancelled, killing wkhtmltopdf process")
		if err := cmd.Process.Kill(); err != nil {
			logger.Errorf("Failed to kill process: %v", err)
		}
		httpError(ctx, w, ctx.Err(), http.StatusRequestTimeout)
		return
	case err := <-done:
		if err != nil {
			logger.Errorf("wkhtmltopdf process failed: %v", err)
			errorTotal.WithLabelValues("process_failed", err.Error()).Inc()
			httpError(ctx, w, err, http.StatusInternalServerError)
			return
		}
	}

	// Only set the content type header when the process is successful
	w.Header().Set("Content-Type", "application/pdf")
	// Write the PDF to the client
	_, err = w.Write(pdfBuffer.Bytes())
	if err != nil {
		logger.Errorf("Failed to write PDF to response: %v", err)
		httpAbort(ctx, w, err)
		return
	}

	// Log and track the size of the generated PDF
	logger.Infof("Generated PDF size: %d bytes", pdfBuffer.Len())
	pdfSize.Observe(float64(pdfBuffer.Len()))
	logger.Infoln("wkhtmltopdf process completed successfully")
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func main() {
	log := NewProductionLogger()
	router := http.NewServeMux()
	router.HandleFunc("/status", withTraceID(statusHandler))
	router.HandleFunc("/pdf", withTraceID(pdfHandler))
	router.HandleFunc("/image", withTraceID(imageHandler))
	router.Handle("/metrics", promhttp.Handler())

	log.Println("kwkhtmltopdf server listening on port 8080")

	server := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}
	log.Fatal(server.ListenAndServe())

}
