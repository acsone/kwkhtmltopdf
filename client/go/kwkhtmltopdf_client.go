package main

import (
	"bytes"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
)

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
	io.Copy(writer, file)
	return err
}

func main() {
	var err error
	var out *os.File

	serverURL := os.Getenv("KWKHTMLTOPDF_SERVER_URL")
	if serverURL == "" {
		log.Fatal("KWKHTMLTOPDF_SERVER_URL not set")
	}

	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"-h"}
	}
	if len(args) >= 2 && !strings.HasPrefix(args[len(args)-1], "-") && !strings.HasPrefix(args[len(args)-2], "-") {
		out, err = os.Create(args[len(args)-1])
		if err != nil {
			log.Fatal(err)
		}
		defer out.Close()
		args = args[:len(args)-1]
	} else {
		out = os.Stdout
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, arg := range args {
		if arg == "-" {
			log.Fatal("stdin/stdout input is not implemented")
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
			log.Fatal(err)
		}
	}
	w.Close()

	resp, err := http.Post(serverURL, w.FormDataContentType(), &buf)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Fatal("server error, consult server log for details")
	}

	// TODO detection of chunked encoding read errors,
	// TODO which indicate server error
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		log.Fatal(err)
	}
}
