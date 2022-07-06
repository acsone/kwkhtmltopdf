package main

import (
	"bytes"
	"errors"
	"io"
	"time"
	"io/ioutil"
	"net/http"
	"os"
	"net"
	"os/exec"
	"path/filepath"
	"github.com/rs/zerolog/pkgerrors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
	log.Error().Str("From", addr).Stack().Err(err).Msg("")
	http.Error(w, err.Error(), code)
}

func httpAbort(w http.ResponseWriter, err error, addr string) {
	log.Error().Str("From", addr).Stack().Err(err).Msg("")
	// abort chunked encoding response as crude way to report error to client
	wh, ok := w.(http.Hijacker)
	if !ok {
		log.Fatal().Msg("cannot abort connection, error not reported to client: http.Hijacker not supported")
		return
	}
	c, _, err := wh.Hijack()
	if err != nil {
		log.Fatal().Str("From", addr).Err(err).Msg("cannot abort connection, error not reported to client: http.Hijacker not supported")
		return
	}
	c.Close()
}

func handler(w http.ResponseWriter, r *http.Request) {
	debug_enabled := os.Getenv("DEBUG")
	if debug_enabled != "" {
		log.Info().Str("Method", r.Method).Str("Url",r.URL.Path).Msg("Parameters")
	}

	hostArg, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		log.Warn().Err(err).Msg("Cannot get remote ip addr")
		return
	}
	addr_array, err := net.LookupAddr(hostArg)
	addr := ""
	if err != nil {
		log.Warn().Err(err).Msg("Cannot get resolve DNS addr, use IP as fallback")
		addr = hostArg
	} else {
		addr = addr_array[0]
	}
	log.Info().Str("From",addr).Msg("Request Received")

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
	}
	if debug_enabled != "" {
		log.Info().Msg("starting")
	}
	// Create output file
	outputfile := filepath.Join(tmpdir, "output.pdf")
	if !(docOutput)  {
		args = append(args, outputfile)
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
	if docOutput {
		_, err = io.Copy(w, cmdStdout)
        if err != nil {
            httpAbort(w, err, addr)
            return
	    }
        err = cmd.Wait()
        if err != nil {
            httpAbort(w, err, addr)
            return
	    }

    } else {
        err = cmd.Wait()
        if err != nil {
            httpAbort(w, err, addr)
            return
	    }
		file_copy, err := os.Open(outputfile)
		if err != nil {
			httpError(w, err, http.StatusNotFound, addr)
			return
		}
		_, err = io.Copy(w, file_copy)
		if err != nil {
			httpAbort(w, err, addr)
			return
		}
		file_copy.Close()
	}
	elapsed := time.Since(start).Seconds()
	log.Info().Float64("Duration", elapsed).Str("From", addr).Msg("Printing duration")
	if debug_enabled != "" {
		log.Info().Msg("success")
	}

}

func main() {
	// Setup loggin structure
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack
	http.HandleFunc("/", handler)
	log.Info().Msg("kwkhtmltopdf server listening on port 8080")
	log.Fatal().Err((http.ListenAndServe(":8080", nil))).Msg("")
}
