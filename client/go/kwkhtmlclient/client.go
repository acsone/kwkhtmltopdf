// Copyright (c) 2019 ACSONE SA/NV
// Distributed under the MIT License (http://opensource.org/licenses/MIT)

package kwkhtmlclient

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
)

const chunkSize = 32 * 1024

var ErrServerURLNotSet = errors.New("KWKHTMLTOPDF_SERVER_URL not set")

func ServerURLFromEnv() (string, error) {
	serverURL := os.Getenv("KWKHTMLTOPDF_SERVER_URL")
	if serverURL == "" {
		return "", ErrServerURLNotSet
	}
	return serverURL, nil
}

func addOption(w *multipart.Writer, option string) error {
	return w.WriteField("option", option)
}

func addFile(w *multipart.Writer, filename string) error {
	writer, err := w.CreateFormFile("file", filename)
	if err != nil {
		return err
	}
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(writer, file)
	return err
}

// Run performs a request against the given endpoint (e.g. "/pdf" or "/image")
// on the server at serverURL.
//
// The behavior matches the original single-file Go client:
// - if args is empty, "-h" is sent
// - if the last argument looks like an output file, it is created and used
// - file arguments are sent as multipart file parts
func Run(serverURL, endpointPath string, args []string, stdout io.Writer) error {
	if serverURL == "" {
		return ErrServerURLNotSet
	}
	if len(args) == 0 {
		args = []string{"-h"}
	}

	out := stdout
	if len(args) >= 2 && !strings.HasPrefix(args[len(args)-1], "-") && !strings.HasPrefix(args[len(args)-2], "-") {
		file, err := os.Create(args[len(args)-1])
		if err != nil {
			return err
		}
		defer file.Close()
		out = file
		args = args[:len(args)-1]
	}

	var postBuf bytes.Buffer
	w := multipart.NewWriter(&postBuf)
	for _, arg := range args {
		var err error
		if arg == "-" {
			return errors.New("stdin/stdout input is not implemented")
		} else if strings.HasPrefix(arg, "-") {
			err = addOption(w, arg)
		} else if strings.HasPrefix(arg, "https://") {
			err = addOption(w, arg)
		} else if strings.HasPrefix(arg, "http://") {
			err = addOption(w, arg)
		} else if strings.HasPrefix(arg, "file://") {
			err = addFile(w, arg[7:])
		} else if _, statErr := os.Stat(arg); statErr == nil {
			// TODO: better way to detect file arguments
			err = addFile(w, arg)
		} else {
			err = addOption(w, arg)
		}
		if err != nil {
			return err
		}
	}
	_ = w.Close()

	endpoint := serverURL + endpointPath
	resp, err := http.Post(endpoint, w.FormDataContentType(), &postBuf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("server error, consult server log for details")
	}

	respBuf := make([]byte, chunkSize)
	for {
		nr, er := resp.Body.Read(respBuf)
		if er != nil && er != io.EOF {
			return errors.New("server error, consult server log for details")
		}
		if nr > 0 {
			_, ew := out.Write(respBuf[0:nr])
			if ew != nil {
				return ew
			}
		}
		if er == io.EOF {
			break
		}
	}

	return nil
}
