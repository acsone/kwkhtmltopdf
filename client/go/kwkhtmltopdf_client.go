// Copyright (c) 2019 ACSONE SA/NV
// Distributed under the MIT License (http://opensource.org/licenses/MIT)

package main

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

func do() error {
	var err error
	var out *os.File

	serverURL := os.Getenv("KWKHTMLTOPDF_SERVER_URL")
	if serverURL == "" {
		return errors.New("KWKHTMLTOPDF_SERVER_URL not set")
	} else if serverURL == "MOCK" {
		os.Stdout.WriteString("wkhtmltopdf 0.12.5 (mock)\n")
		return nil
	}

	// detect if last argument is output file, and create it
	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"-h"}
	}
	if len(args) >= 2 && !strings.HasPrefix(args[len(args)-1], "-") && !strings.HasPrefix(args[len(args)-2], "-") {
		out, err = os.Create(args[len(args)-1])
		if err != nil {
			return err
		}
		defer out.Close()
		args = args[:len(args)-1]
	} else {
		out = os.Stdout
	}

	// prepare request
	var postBuf bytes.Buffer
	w := multipart.NewWriter(&postBuf)
	for _, arg := range args {
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
		} else if _, err := os.Stat(arg); err == nil {
			// TODO: better way to detect file arguments
			err = addFile(w, arg)
		} else {
			err = addOption(w, arg)
		}
		if err != nil {
			return err
		}
	}
	w.Close()

	// post request
	resp, err := http.Post(serverURL, w.FormDataContentType(), &postBuf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("server error, consult server log for details")
	}

	// read response
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

func main() {
	err := do()
	if err != nil {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
		os.Exit(-1)
	}
}
