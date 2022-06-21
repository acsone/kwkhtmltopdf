package main

import (
	"bytes"
	"errors"
	"io"
	"time"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"net"
	"os/exec"
	"path/filepath"
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

func httpError(w http.ResponseWriter, err error, code int, addr string) {
	log.Printf("%s - from host %s", err, addr)
	http.Error(w, err.Error(), code)
}

func httpAbort(w http.ResponseWriter, err error, addr string) {
	log.Printf("%s - from host %s", err, addr)
	// abort chunked encoding response as crude way to report error to client
	wh, ok := w.(http.Hijacker)
	if !ok {
		log.Println("cannot abort connection, error not reported to client: http.Hijacker not supported")
		return
	}
	c, _, err := wh.Hijack()
	if err != nil {
		log.Printf("cannot abort connection, error not reported to client: %s from host %s", err, addr)
		return
	}
	c.Close()
}

func handler(w http.ResponseWriter, r *http.Request) {
	debug_enabled := os.Getenv("DEBUG")
	if debug_enabled != "" {
		log.Printf("%s %s", r.Method, r.URL.Path)
	}

	hostArg, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		log.Println("Cannot get remote ip addr")
		return
	}
	addr_array, err := net.LookupAddr(hostArg)
	addr := ""
	if err != nil {
		log.Println("Cannot get resolve DNS addr, use IP as fallback")
		addr = r.RemoteAddr
	} else {
		addr = addr_array[0]
	}
	log.Printf("Request from : %s", addr)

	if r.Method != http.MethodPost {
		httpError(w, errors.New("http method not allowed: "+r.Method), http.StatusMethodNotAllowed, addr)
		return
	}
	if r.URL.Path != "/" && r.URL.Path != "/pdf" {
		// handle / and /pdf, keep the rest for future use
		httpError(w, errors.New("path not found: "+r.URL.Path), http.StatusNotFound, addr)
		return
	}

	// temp dir for files
	tmpdir, err := ioutil.TempDir("", "kwk")
	if err != nil {
		httpError(w, err, http.StatusNotFound, addr)
		return
	}
	defer os.RemoveAll(tmpdir)

	// parse request
	reader, err := r.MultipartReader()
	if err != nil {
		httpError(w, err, http.StatusBadRequest, addr)
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
			httpError(w, err, http.StatusBadRequest, addr)
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
				httpError(w, err, http.StatusBadRequest, addr)
				return
			}
			_, err = io.Copy(file, part)
			file.Close()
			if err != nil {
				httpError(w, err, http.StatusBadRequest, addr)
				return
			}
			args = append(args, path)
		} else {
			httpError(w, errors.New("unpexpected part name: "+part.FormName()), http.StatusBadRequest, addr)
			return
		}
	}

	if docOutput {
		w.Header().Add("Content-Type", "text/plain")
	} else {
		w.Header().Add("Content-Type", "application/pdf")
		args = append(args, "-")
	}
	if debug_enabled != "" {
		log.Println(args, "starting") // TODO better logging, hide sensitve options
	}
	cmd := exec.Command(wkhtmltopdfBin(), args...)
	cmdStdout, err := cmd.StdoutPipe()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError, addr)
		return
	}
	cmd.Stderr = os.Stderr
	start := time.Now()
	err = cmd.Start()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError, addr)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, err = io.Copy(w, cmdStdout)
	if err != nil {
		httpAbort(w, err, addr)
		return
	}
	err = cmd.Wait()
	elapsed := time.Since(start).Seconds()
	log.Printf("Print from %s took %.6f s", addr, elapsed)
	if err != nil {
		httpAbort(w, err, addr)
		return
	}
	if debug_enabled != "" {
		log.Println(args, "success")
	}

}

func main() {
	http.HandleFunc("/", handler)
	log.Println("kwkhtmltopdf server listening on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
