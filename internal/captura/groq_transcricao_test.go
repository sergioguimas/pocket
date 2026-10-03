package captura

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func semEspera(context.Context, time.Duration) error { return nil }

func TestGroqTranscritorSucesso(t *testing.T) {
	var recebido struct {
		auth, model, language, responseFormat string
		fileBytes                             []byte
		fileContentType                       string
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recebido.auth = r.Header.Get("Authorization")
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("Content-Type: %v", err)
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("NextPart: %v", err)
			}
			switch part.FormName() {
			case "file":
				recebido.fileBytes, _ = io.ReadAll(part)
				recebido.fileContentType = part.Header.Get("Content-Type")
			case "model":
				b, _ := io.ReadAll(part)
				recebido.model = string(b)
			case "language":
				b, _ := io.ReadAll(part)
				recebido.language = string(b)
			case "response_format":
				b, _ := io.ReadAll(part)
				recebido.responseFormat = string(b)
			}
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fiz o devocional"))
	}))
	defer srv.Close()

	g := &GroqTranscritor{APIKey: "gsk_teste", Modelo: "whisper-large-v3-turbo", URL: srv.URL, HTTPClient: srv.Client()}
	texto, err := g.Transcrever(context.Background(), []byte("audio-fake"), "audio/webm;codecs=opus")
	if err != nil {
		t.Fatalf("Transcrever: %v", err)
	}
	if texto != "fiz o devocional" {
		t.Fatalf("texto = %q", texto)
	}
	if recebido.auth != "Bearer gsk_teste" {
		t.Fatalf("Authorization = %q", recebido.auth)
	}
	if recebido.model != "whisper-large-v3-turbo" || recebido.language != "pt" || recebido.responseFormat != "text" {
		t.Fatalf("campos = %+v", recebido)
	}
	if string(recebido.fileBytes) != "audio-fake" {
		t.Fatalf("fileBytes = %q", recebido.fileBytes)
	}
	if !strings.Contains(recebido.fileContentType, "audio/webm") {
		t.Fatalf("fileContentType = %q", recebido.fileContentType)
	}
}

func TestGroqTranscritorRetentaUmaVezEmStatus500(t *testing.T) {
	var chamadas int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&chamadas, 1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	g := &GroqTranscritor{APIKey: "k", Modelo: "m", URL: srv.URL, HTTPClient: srv.Client(), sleep: semEspera}
	texto, err := g.Transcrever(context.Background(), []byte("a"), "audio/ogg")
	if err != nil {
		t.Fatalf("Transcrever: %v", err)
	}
	if texto != "ok" {
		t.Fatalf("texto = %q", texto)
	}
	if atomic.LoadInt32(&chamadas) != 2 {
		t.Fatalf("chamadas = %d, want 2", chamadas)
	}
}

func TestGroqTranscritorFalhaFinal429ViraErrLimite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	g := &GroqTranscritor{APIKey: "k", Modelo: "m", URL: srv.URL, HTTPClient: srv.Client(), sleep: semEspera}
	_, err := g.Transcrever(context.Background(), []byte("a"), "audio/mp4")
	var errLimite ErrLimite
	if !errors.As(err, &errLimite) {
		t.Fatalf("err = %v, want ErrLimite", err)
	}
}

func TestGroqTranscritorFalha400NaoRetenta(t *testing.T) {
	var chamadas int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&chamadas, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	g := &GroqTranscritor{APIKey: "k", Modelo: "m", URL: srv.URL, HTTPClient: srv.Client()}
	_, err := g.Transcrever(context.Background(), []byte("a"), "audio/webm")
	var errProvedor ErrProvedor
	if !errors.As(err, &errProvedor) {
		t.Fatalf("err = %v, want ErrProvedor", err)
	}
	if atomic.LoadInt32(&chamadas) != 1 {
		t.Fatalf("chamadas = %d, want 1 (400 não tenta de novo)", chamadas)
	}
}
