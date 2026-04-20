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
	"strings"
	"time"
)

func imageAbort(ctx context.Context, w http.ResponseWriter, err error) {
	logger := loggerFromContext(ctx)

	logger.Errorf("HTTP abort: %v", err)

	if sr, ok := w.(*statusRecorder); ok {
		sr.statusCode = http.StatusInternalServerError
	}

	wh, ok := w.(http.Hijacker)
	if !ok {
		imageErrorTotal.WithLabelValues("hijack_unsupported", err.Error()).Inc()
		logger.Errorln("cannot abort connection, error not reported to client: http.Hijacker not supported")
		return
	}
	c, _, err := wh.Hijack()
	if err != nil {
		imageErrorTotal.WithLabelValues("hijack_failed", err.Error()).Inc()
		logger.Errorln("cannot abort connection, error not reported to client: ", err)
		return
	}
	c.Close()
}

func wkhtmltoimageBin() string {
	b := os.Getenv("KWKHTMLTOIMAGE_BIN")
	if b != "" {
		return b
	}
	return "wkhtmltoimage"
}

func imageHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := loggerFromContext(ctx)

	if r.Method != http.MethodPost {
		imageErrorTotal.WithLabelValues("method_not_allowed", r.Method).Inc()
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()
	imageActiveRequests.Inc()
	defer imageActiveRequests.Dec()

	rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
	defer func() {
		duration := time.Since(start).Seconds()
		imageRequestDuration.WithLabelValues(r.URL.Path).Observe(duration)
		imageRequestsTotal.WithLabelValues(r.URL.Path, fmt.Sprintf("%d", rec.statusCode)).Inc()
	}()

	tmpdir, err := os.MkdirTemp("", "kwkimg")
	if err != nil {
		imageErrorTotal.WithLabelValues("tempdir_creation_failed", err.Error()).Inc()
		httpError(ctx, w, err, http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpdir)

	logger.Infof("Temporary directory created: %s", tmpdir)

	reader, err := r.MultipartReader()
	if err != nil {
		imageErrorTotal.WithLabelValues("multipart_reader_creation_failed", err.Error()).Inc()
		logger.Errorf("Failed to create multipart reader: %v", err)
		httpError(ctx, w, err, http.StatusBadRequest)
		return
	}

	args, indexPath, err := parseMultipartFormImage(ctx, reader, tmpdir)
	if err != nil {
		imageErrorTotal.WithLabelValues("parse_multipart_form_failed", err.Error()).Inc()
		logger.Errorf("Failed to parse multipart form: %v", err)
		httpError(ctx, w, err, http.StatusBadRequest)
		return
	}

	if indexPath == "" {
		imageErrorTotal.WithLabelValues("index_html_file_not_found", "").Inc()
		logger.Errorln("index.html file is required but not found")
		httpError(ctx, w, errors.New("index.html file is required"), http.StatusBadRequest)
		return
	}

	ensureImageFormatDefault(&args)
	runWkhtmltoimage(ctx, rec, args, indexPath, tmpdir)
}

func parseMultipartFormImage(ctx context.Context, reader *multipart.Reader, tmpdir string) (args []string, indexPath string, err error) {
	logger := loggerFromContext(ctx)

	defer func() {
		if err != nil {
			imageErrorTotal.WithLabelValues("parse_multipart_form", err.Error()).Inc()
		}
	}()

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			logger.Errorln(err)
			return nil, "", err
		}

		if part.FormName() == "file" {
			base := filepath.Base(part.FileName())
			path := filepath.Join(tmpdir, base)
			file, err := os.Create(path)
			if err != nil {
				logger.Errorln(err)
				return nil, "", err
			}
			_, err = io.Copy(file, part)
			file.Close()
			if err != nil {
				logger.Errorln(err)
				return nil, "", err
			}
			if base == "index.html" {
				indexPath = path
			}
		} else {
			buf := new(bytes.Buffer)
			if _, err := io.Copy(buf, part); err != nil {
				logger.Errorln(err)
				return nil, "", err
			}
			arg := buf.String()
			if arg == "" {
				args = append(args, fmt.Sprintf("--%s", part.FormName()))
			} else {
				args = append(args, fmt.Sprintf("--%s", part.FormName()), arg)
			}
		}
	}

	return args, indexPath, nil
}

func hasImageFormatOption(args []string) bool {
	for _, s := range args {
		if s == "--format" {
			return true
		}
	}
	return false
}

func ensureImageFormatDefault(args *[]string) {
	if hasImageFormatOption(*args) {
		return
	}
	*args = append([]string{"--format", "png"}, *args...)
}

func imageOutputExt(args []string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] != "--format" {
			continue
		}
		switch strings.ToLower(args[i+1]) {
		case "jpg", "jpeg":
			return "jpg"
		case "png":
			return "png"
		case "bmp":
			return "bmp"
		case "svg":
			return "svg"
		default:
			return "png"
		}
	}
	return "png"
}

func imageContentType(ext string) string {
	switch ext {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "bmp":
		return "image/bmp"
	case "svg":
		return "image/svg+xml"
	default:
		return "image/png"
	}
}

func runWkhtmltoimage(ctx context.Context, w http.ResponseWriter, args []string, indexPath, tmpdir string) {
	logger := loggerFromContext(ctx)

	ext := imageOutputExt(args)
	outPath := filepath.Join(tmpdir, "output."+ext)

	runArgs := append(append([]string{}, args...), "--enable-local-file-access", indexPath, outPath)
	logger.Infoln("Args", runArgs)

	logger.Infoln("Starting wkhtmltoimage process")
	cmd := exec.Command(wkhtmltoimageBin(), runArgs...)
	cmd.Stderr = os.Stderr
	done := make(chan error, 1)

	err := cmd.Start()
	if err != nil {
		imageErrorTotal.WithLabelValues("process_start_failed", err.Error()).Inc()
		httpError(ctx, w, err, http.StatusInternalServerError)
		return
	}

	logger.Infoln("wkhtmltoimage process started")

	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		if ctx.Err() != nil {
			imageErrorTotal.WithLabelValues("context_cancelled", ctx.Err().Error()).Inc()
		}
		logger.Errorln("Context cancelled, killing wkhtmltoimage process")
		if err := cmd.Process.Kill(); err != nil {
			logger.Errorf("Failed to kill process: %v", err)
		}
		httpError(ctx, w, ctx.Err(), http.StatusRequestTimeout)
		return
	case err := <-done:
		if err != nil {
			logger.Errorf("wkhtmltoimage process failed: %v", err)
			imageErrorTotal.WithLabelValues("process_failed", err.Error()).Inc()
			httpError(ctx, w, err, http.StatusInternalServerError)
			return
		}
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		imageErrorTotal.WithLabelValues("read_output_failed", err.Error()).Inc()
		httpError(ctx, w, err, http.StatusInternalServerError)
		return
	}
	if len(data) == 0 {
		err := errors.New("wkhtmltoimage produced empty output")
		imageErrorTotal.WithLabelValues("empty_output", "").Inc()
		httpError(ctx, w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", imageContentType(ext))
	_, err = w.Write(data)
	if err != nil {
		logger.Errorf("Failed to write image to response: %v", err)
		imageAbort(ctx, w, err)
		return
	}

	logger.Infof("Generated image size: %d bytes", len(data))
	imageSize.Observe(float64(len(data)))
	logger.Infoln("wkhtmltoimage process completed successfully")
}
