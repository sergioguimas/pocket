package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"pocket/internal/app"
	"pocket/internal/captura"
)

// maxAudioBytes é o teto de §3 ("audio ≤ 2 MB"). maxMultipartBody dá uma
// folga para o resto do multipart (boundary, os outros campos, nenhum aqui
// mas o padrão do formato tem overhead) — a checagem que importa de
// verdade é o len(dados) explícito em decodeCapturaAudio.
const (
	maxAudioBytes    = 2 * 1024 * 1024
	maxMultipartBody = maxAudioBytes + 64*1024
)

// mimesAceitos são os três tipos de áudio que o front grava (§3) — sempre
// comparado só com a parte antes de ";" (MediaRecorder manda
// "audio/webm;codecs=opus").
var mimesAceitos = map[string]bool{"audio/webm": true, "audio/ogg": true, "audio/mp4": true}

// handleCapturaAtiva responde GET /api/captura (§3): esconde a barra no
// front quando GROQ_API_KEY está vazia.
func (s *Server) handleCapturaAtiva(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ativa": s.ap.CapturaAtiva()})
}

// capturaResponse funde o CapturaResultado do app (captura_id/texto) com
// captura.Resposta (acao/item/registro/compras/resto/avisos, embutida) —
// json.Marshal promove os campos do embutido para o mesmo nível (§3).
type capturaResponse struct {
	CapturaID int64  `json:"captura_id"`
	Texto     string `json:"texto"`
	captura.Resposta
}

// handleCriarCaptura responde POST /api/captura (§3): JSON {"texto"} ou
// multipart "audio". 503 antes de tocar no corpo se a captura está
// desligada (evita ler/parsear um multipart de até 2 MB à toa).
func (s *Server) handleCriarCaptura(w http.ResponseWriter, r *http.Request) {
	if !s.ap.CapturaAtiva() {
		writeErro(w, http.StatusServiceUnavailable, "captura desligada")
		return
	}

	entrada, msgErro, status := decodeCapturaEntrada(w, r)
	if msgErro != "" {
		writeErro(w, status, msgErro)
		return
	}

	res, err := s.ap.Capturar(r.Context(), entrada)
	if err != nil {
		writeErroApp(w, err)
		return
	}
	// §3: avisos é sempre lista — [] e não null (o front lê .length).
	if res.Resposta.Avisos == nil {
		res.Resposta.Avisos = []string{}
	}
	writeJSON(w, http.StatusOK, capturaResponse{CapturaID: res.ID, Texto: res.Texto, Resposta: res.Resposta})
}

// decodeCapturaEntrada decide pelo Content-Type se o corpo é o multipart de
// voz ou o JSON de texto (§3).
func decodeCapturaEntrada(w http.ResponseWriter, r *http.Request) (app.CapturaEntrada, string, int) {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		return decodeCapturaAudio(w, r)
	}
	return decodeCapturaTexto(r)
}

type capturaTextoRequest struct {
	Texto string `json:"texto"`
}

func decodeCapturaTexto(r *http.Request) (app.CapturaEntrada, string, int) {
	var req capturaTextoRequest
	if err := decodeJSON(r, &req); err != nil {
		return app.CapturaEntrada{}, "corpo JSON inválido", http.StatusBadRequest
	}
	texto := strings.TrimSpace(req.Texto)
	n := utf8.RuneCountInString(texto)
	if n < 1 || n > 500 {
		return app.CapturaEntrada{}, "texto deve ter entre 1 e 500 caracteres", http.StatusBadRequest
	}
	return app.CapturaEntrada{Texto: texto}, "", 0
}

// decodeCapturaAudio lê o campo "audio" do multipart (§3: ≤ 2 MB, ≤ 60 s —
// o limite de duração é responsabilidade do front, aqui só o de tamanho).
// MaxBytesReader corta a leitura do corpo inteiro antes de estourar a
// memória; o len(dados) explícito é o que decide o 400 de verdade.
func decodeCapturaAudio(w http.ResponseWriter, r *http.Request) (app.CapturaEntrada, string, int) {
	r.Body = http.MaxBytesReader(w, r.Body, maxMultipartBody)
	if err := r.ParseMultipartForm(maxMultipartBody); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return app.CapturaEntrada{}, "áudio maior que 2 MB", http.StatusBadRequest
		}
		return app.CapturaEntrada{}, "corpo multipart inválido", http.StatusBadRequest
	}

	file, header, err := r.FormFile("audio")
	if err != nil {
		return app.CapturaEntrada{}, `campo "audio" ausente`, http.StatusBadRequest
	}
	defer file.Close()

	mimeBase, _, _ := strings.Cut(header.Header.Get("Content-Type"), ";")
	mimeBase = strings.TrimSpace(mimeBase)
	if !mimesAceitos[mimeBase] {
		return app.CapturaEntrada{}, "tipo de áudio não suportado (use webm, ogg ou mp4)", http.StatusBadRequest
	}

	dados, err := io.ReadAll(file)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return app.CapturaEntrada{}, "áudio maior que 2 MB", http.StatusBadRequest
		}
		return app.CapturaEntrada{}, "corpo inválido", http.StatusBadRequest
	}
	if len(dados) == 0 {
		return app.CapturaEntrada{}, "áudio vazio", http.StatusBadRequest
	}
	if len(dados) > maxAudioBytes {
		return app.CapturaEntrada{}, "áudio maior que 2 MB", http.StatusBadRequest
	}

	return app.CapturaEntrada{Audio: dados, Mime: mimeBase}, "", 0
}

type capturaDesfechoRequest struct {
	Desfecho string `json:"desfecho"`
}

// handleCapturaDesfecho responde POST /api/captura/{id}/desfecho (§3, V5).
func (s *Server) handleCapturaDesfecho(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}

	var req capturaDesfechoRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErro(w, http.StatusBadRequest, "corpo JSON inválido")
		return
	}

	if err := s.ap.RegistrarDesfecho(r.Context(), id, req.Desfecho); err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
