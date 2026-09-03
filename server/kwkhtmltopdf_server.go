package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// a render that outlives this has no reader left: the caller is long gone
const defaultRenderTimeout = 120 * time.Second

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

// KWKHTMLTOPDF_TIMEOUT is in seconds, 0 disables the timeout
func renderTimeout() time.Duration {
	value := os.Getenv("KWKHTMLTOPDF_TIMEOUT")
	if value == "" {
		return defaultRenderTimeout
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 0 {
		log.Printf("ignoring invalid KWKHTMLTOPDF_TIMEOUT %q, using %s", value, defaultRenderTimeout)
		return defaultRenderTimeout
	}
	return time.Duration(seconds) * time.Second
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

	if r.URL.Path == "/status" {
		w.WriteHeader(http.StatusOK)
		return
	} else {
		// don't log status
		log.Printf("%s %s", r.Method, r.URL.Path)
	}
	if r.Method != http.MethodPost {
		httpError(w, errors.New("http method not allowed: "+r.Method), http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/" && r.URL.Path != "/pdf" && r.URL.Path != "/image" {
		// handle /, /pdf, and /image, keep the rest for future use
		httpError(w, errors.New("path not found: "+r.URL.Path), http.StatusNotFound)
		return
	}

	// temp dir for files
	tmpdir, err := ioutil.TempDir("", "kwk")
	if err != nil {
		httpError(w, err, http.StatusNotFound)
		return
	}
	defer os.RemoveAll(tmpdir)

	// parse request
	reader, err := r.MultipartReader()
	if err != nil {
		httpError(w, err, http.StatusBadRequest)
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
			httpError(w, err, http.StatusBadRequest)
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
				httpError(w, err, http.StatusBadRequest)
				return
			}
			_, err = io.Copy(file, part)
			file.Close()
			if err != nil {
				httpError(w, err, http.StatusBadRequest)
				return
			}
			args = append(args, path)
		} else {
			httpError(w, errors.New("unpexpected part name: "+part.FormName()), http.StatusBadRequest)
			return
		}
	}

	// determine if this is an image request
	isImageRequest := r.URL.Path == "/image"

	if docOutput {
		w.Header().Add("Content-Type", "text/plain")
	} else if isImageRequest {
		w.Header().Add("Content-Type", "image/png")
		args = append(args, "-")
	} else {
		w.Header().Add("Content-Type", "application/pdf")
		args = append(args, "-")
	}

	var redactedArgs = redactArgs(args)

	log.Println(redactedArgs, "starting")

	timeout := renderTimeout()
	// the request context is already done when the caller disconnects
	ctx := r.Context()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var cmd *exec.Cmd
	if isImageRequest {
		cmd = exec.Command(wkhtmltoimageBin(), args...)
	} else {
		cmd = exec.Command(wkhtmltopdfBin(), args...)
	}
	setProcessGroup(cmd)
	cmdStdout, err := cmd.StdoutPipe()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	cmd.Stderr = os.Stderr
	err = cmd.Start()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	// kill the render when the caller is gone or the timeout is over
	rendered := make(chan struct{})
	defer close(rendered)
	go func() {
		select {
		case <-ctx.Done():
			killProcessTree(cmd)
		case <-rendered:
		}
	}()
	// reap the child on the error paths too, or it is left as a zombie
	reaped := false
	defer func() {
		if reaped {
			return
		}
		killProcessTree(cmd)
		cmd.Wait()
	}()
	w.WriteHeader(http.StatusOK)
	_, err = io.Copy(w, cmdStdout)
	if err != nil {
		httpAbort(w, err)
		return
	}
	err = cmd.Wait()
	reaped = true
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			err = errors.New("render timed out after " + timeout.String() + ": " + err.Error())
		}
		httpAbort(w, err)
		return
	}

	log.Println(redactedArgs, "success")
}

func main() {
	http.HandleFunc("/", handler)
	http.HandleFunc("/pdf", handler)
	http.HandleFunc("/image", handler)
	log.Println("kwkhtmltopdf server listening on port 8080")
	log.Println("Available endpoints: / (PDF), /pdf (PDF), /image (Image), /status (Health check)")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
