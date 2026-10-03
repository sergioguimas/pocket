package captura

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"
)

// timeoutTranscricao é o prazo por chamada de transcrição (§4).
const timeoutTranscricao = 20 * time.Second

const urlTranscricaoGroq = "https://api.groq.com/openai/v1/audio/transcriptions"

// GroqTranscritor implementa Transcritor com a API de transcrição do Groq
// (§4: multipart/form-data cru — cópia adaptada de
// servicos/smaug/internal/transcricao/groq.go, módulos Go separados, não dá
// para importar).
type GroqTranscritor struct {
	APIKey     string
	Modelo     string
	URL        string // "" usa urlTranscricaoGroq — só testes trocam
	HTTPClient *http.Client
	sleep      sleepFunc // nil usa defaultSleep; injetável no teste
}

var _ Transcritor = (*GroqTranscritor)(nil)

// NovoGroqTranscritor cria o cliente com o http.Client default (timeout por
// chamada aplicado via context.WithTimeout em Transcrever, não aqui — o
// mesmo client serve outras chamadas com prazos diferentes).
func NovoGroqTranscritor(apiKey, modelo string) *GroqTranscritor {
	return &GroqTranscritor{APIKey: apiKey, Modelo: modelo, HTTPClient: &http.Client{}}
}

// Transcrever monta o multipart (file/model/language=pt/response_format=
// text), autentica com Bearer e tenta de novo 1x em 429/5xx/erro de rede
// (§4). 429 final vira ErrLimite; qualquer outra falha final vira
// ErrProvedor.
func (g *GroqTranscritor) Transcrever(ctx context.Context, audio []byte, mimetype string) (string, error) {
	sleep := g.sleep
	if sleep == nil {
		sleep = defaultSleep
	}
	url := g.URL
	if url == "" {
		url = urlTranscricaoGroq
	}
	client := g.HTTPClient
	if client == nil {
		client = &http.Client{}
	}

	ctx, cancel := context.WithTimeout(ctx, timeoutTranscricao)
	defer cancel()

	var lastErr error
	lastStatus := 0
	for tentativa := 1; tentativa <= 2; tentativa++ {
		body, contentType, err := montarMultipartTranscricao(audio, mimetype, g.Modelo)
		if err != nil {
			return "", fmt.Errorf("transcrição: montar multipart: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
		if err != nil {
			return "", fmt.Errorf("transcrição: montar request: %w", err)
		}
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Authorization", "Bearer "+g.APIKey)

		resp, doErr := client.Do(req)
		if doErr != nil {
			lastErr = doErr
			lastStatus = 0
		} else {
			respBody, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				lastErr = readErr
				lastStatus = resp.StatusCode
			} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return strings.TrimSpace(string(respBody)), nil
			} else {
				lastErr = fmt.Errorf("groq status %d: %s", resp.StatusCode, truncar(string(respBody), 300))
				lastStatus = resp.StatusCode
				if !retryableStatus(resp.StatusCode) {
					return "", ErrProvedor{Msg: fmt.Sprintf("transcrição: %v", lastErr)}
				}
			}
		}

		if tentativa == 2 {
			break
		}
		if serr := sleep(ctx, backoffRetry); serr != nil {
			return "", ErrProvedor{Msg: fmt.Sprintf("transcrição: %v", serr)}
		}
	}

	if lastStatus == http.StatusTooManyRequests {
		return "", ErrLimite{Msg: "limite do Groq atingido — espere um minuto ou use o formulário"}
	}
	return "", ErrProvedor{Msg: fmt.Sprintf("transcrição: groq indisponível: %v", lastErr)}
}

// montarMultipartTranscricao monta o corpo multipart/form-data (§4: file,
// model, language=pt, response_format=text).
func montarMultipartTranscricao(audio []byte, mimetype, modelo string) (*bytes.Buffer, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	nome, ext := "audio.webm", extensaoDoMime(mimetype)
	if ext != "" {
		nome = "audio." + ext
	}
	part, err := criarParteArquivo(w, "file", nome, mimetype)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(audio); err != nil {
		return nil, "", err
	}

	campos := []struct{ nome, valor string }{
		{"model", modelo},
		{"language", "pt"},
		{"response_format", "text"},
	}
	for _, c := range campos {
		if err := w.WriteField(c.nome, c.valor); err != nil {
			return nil, "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return &buf, w.FormDataContentType(), nil
}

func criarParteArquivo(w *multipart.Writer, fieldname, filename, contentType string) (io.Writer, error) {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldname, filename))
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	return w.CreatePart(h)
}

// extensaoDoMime devolve a extensão de arquivo para os três mimes aceitos
// pela captura (§3: audio/webm, audio/ogg, audio/mp4) — ignora parâmetros
// tipo ";codecs=opus".
func extensaoDoMime(mimetype string) string {
	base, _, _ := strings.Cut(mimetype, ";")
	base = strings.TrimSpace(base)
	switch base {
	case "audio/webm":
		return "webm"
	case "audio/ogg":
		return "ogg"
	case "audio/mp4":
		return "mp4"
	default:
		return ""
	}
}

func truncar(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
