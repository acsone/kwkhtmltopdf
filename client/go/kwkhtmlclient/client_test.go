package kwkhtmlclient

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_ServerURLNotSet(t *testing.T) {
	var out bytes.Buffer
	err := Run("", "/pdf", []string{"-h"}, &out)
	if err != ErrServerURLNotSet {
		t.Fatalf("expected ErrServerURLNotSet, got %v", err)
	}
}

func TestRun_SendsOptionsAndWritesStdout(t *testing.T) {
	var gotPath string
	var gotOptions []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		gotOptions = append([]string(nil), r.MultipartForm.Value["option"]...)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	var out bytes.Buffer
	if err := Run(ts.URL, "/pdf", []string{"-h"}, &out); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if gotPath != "/pdf" {
		t.Fatalf("expected path /pdf, got %q", gotPath)
	}
	if out.String() != "ok" {
		t.Fatalf("expected stdout %q, got %q", "ok", out.String())
	}
	if len(gotOptions) != 1 || gotOptions[0] != "-h" {
		t.Fatalf("expected options [-h], got %v", gotOptions)
	}
}

func TestRun_SendsFileArgumentAsMultipartFile(t *testing.T) {
	inPath := filepath.Join(t.TempDir(), "input.html")
	if err := os.WriteFile(inPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var gotFilename string
	var gotContent []byte

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		files := r.MultipartForm.File["file"]
		if len(files) != 1 {
			http.Error(w, "expected one file", http.StatusBadRequest)
			return
		}
		gotFilename = files[0].Filename
		f, err := files[0].Open()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer f.Close()
		gotContent, _ = io.ReadAll(f)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	var out bytes.Buffer
	if err := Run(ts.URL, "/pdf", []string{inPath}, &out); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if out.String() != "ok" {
		t.Fatalf("expected stdout %q, got %q", "ok", out.String())
	}
	if !strings.HasSuffix(gotFilename, filepath.Base(inPath)) {
		t.Fatalf("expected filename to end with %q, got %q", filepath.Base(inPath), gotFilename)
	}
	if string(gotContent) != "hello" {
		t.Fatalf("expected file content %q, got %q", "hello", string(gotContent))
	}
}

func TestRun_WritesToOutputFile(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "out.bin")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/image" {
			http.Error(w, "wrong path", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data"))
	}))
	defer ts.Close()

	// Two trailing non-dash args: last one is treated as output file.
	args := []string{"https://example.invalid", outPath}
	var stdout bytes.Buffer
	if err := Run(ts.URL, "/image", args, &stdout); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", stdout.String())
	}

	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != "data" {
		t.Fatalf("expected output file %q, got %q", "data", string(b))
	}
}

func TestRun_StdinNotImplemented(t *testing.T) {
	var stdout bytes.Buffer
	err := Run("http://example.invalid", "/pdf", []string{"-"}, &stdout)
	if err == nil || err.Error() != "stdin/stdout input is not implemented" {
		t.Fatalf("expected stdin/stdout error, got %v", err)
	}
}
