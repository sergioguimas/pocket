package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pocket/internal/captura"
)

func modelJSONDesconhecidoTeste() []byte {
	return []byte(`{"acao":"desconhecido","item":{"tipo":null,"titulo":null,"notas":null,"regra_tipo":null,"regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":[]},"registro":{"item_id":null,"titulo":null,"dia":null,"nota":null},"compras":{"lista_nome":null,"itens":[]},"resto":null}`)
}

func TestHandleCapturaAtivaFalseSemSetCaptura(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "teste")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/captura", nil)
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if ativa, _ := body["ativa"].(bool); ativa {
		t.Errorf("ativa = %v, want false", body["ativa"])
	}
}

func TestHandleCapturaAtivaTrueComSetCaptura(t *testing.T) {
	st := abrirStoreTeste(t)
	ap := appTeste(t, st)
	ap.SetCaptura(nil, &captura.FakeInterpretador{}, "modelo-teste")
	srv := New(st, ap, "teste")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/captura", nil)
	srv.ServeHTTP(rr, req)

	var body map[string]any
	json.NewDecoder(rr.Body).Decode(&body)
	if ativa, _ := body["ativa"].(bool); !ativa {
		t.Errorf("ativa = %v, want true", body["ativa"])
	}
}

func TestHandleCriarCapturaDesligadaDevolve503(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "teste")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/captura", strings.NewReader(`{"texto":"fiz o devocional"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
}

func TestHandleCriarCapturaTextoVazioDevolve400(t *testing.T) {
	st := abrirStoreTeste(t)
	ap := appTeste(t, st)
	ap.SetCaptura(nil, &captura.FakeInterpretador{ModelJSON: modelJSONDesconhecidoTeste()}, "modelo-teste")
	srv := New(st, ap, "teste")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/captura", strings.NewReader(`{"texto":""}`))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCriarCapturaTextoValidoDevolve200(t *testing.T) {
	st := abrirStoreTeste(t)
	ap := appTeste(t, st)
	interp := &captura.FakeInterpretador{ModelJSON: modelJSONDesconhecidoTeste()}
	ap.SetCaptura(nil, interp, "modelo-teste")
	srv := New(st, ap, "teste")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/captura", strings.NewReader(`{"texto":"hmm deixa eu ver"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if body["acao"] != "desconhecido" {
		t.Errorf("acao = %v", body["acao"])
	}
	if body["texto"] != "hmm deixa eu ver" {
		t.Errorf("texto = %v", body["texto"])
	}
	if _, ok := body["captura_id"]; !ok {
		t.Error("captura_id ausente")
	}
}

func TestHandleCriarCapturaJSONInvalidoDevolve400(t *testing.T) {
	st := abrirStoreTeste(t)
	ap := appTeste(t, st)
	ap.SetCaptura(nil, &captura.FakeInterpretador{}, "modelo-teste")
	srv := New(st, ap, "teste")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/captura", strings.NewReader(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCriarCapturaMultipartAudio(t *testing.T) {
	st := abrirStoreTeste(t)
	ap := appTeste(t, st)
	transc := &captura.FakeTranscritor{Texto: "fiz o devocional"}
	interp := &captura.FakeInterpretador{ModelJSON: modelJSONDesconhecidoTeste()}
	ap.SetCaptura(transc, interp, "modelo-teste")
	srv := New(st, ap, "teste")

	// CreateFormFile fixa Content-Type em application/octet-stream —
	// usa CreatePart com header customizado para mandar o mime real
	// (MediaRecorder manda "audio/webm;codecs=opus").
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="audio"; filename="audio.webm"`}
	h["Content-Type"] = []string{"audio/webm;codecs=opus"}
	pw, err := mw.CreatePart(h)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	pw.Write([]byte("audio-fake-bytes"))
	mw.Close()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/captura", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if len(transc.Chamadas) != 1 || string(transc.Chamadas[0].Audio) != "audio-fake-bytes" {
		t.Fatalf("transc.Chamadas = %+v", transc.Chamadas)
	}
	if transc.Chamadas[0].Mime != "audio/webm" {
		t.Fatalf("Mime = %q, want audio/webm (sem ;codecs=)", transc.Chamadas[0].Mime)
	}

	var respBody map[string]any
	json.NewDecoder(rr.Body).Decode(&respBody)
	if respBody["texto"] != "fiz o devocional" {
		t.Errorf("texto = %v", respBody["texto"])
	}
}

func TestHandleCriarCapturaMultipartMimeInvalidoDevolve400(t *testing.T) {
	st := abrirStoreTeste(t)
	ap := appTeste(t, st)
	ap.SetCaptura(&captura.FakeTranscritor{}, &captura.FakeInterpretador{}, "modelo-teste")
	srv := New(st, ap, "teste")

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="audio"; filename="audio.wav"`}
	h["Content-Type"] = []string{"audio/wav"}
	pw, _ := mw.CreatePart(h)
	pw.Write([]byte("x"))
	mw.Close()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/captura", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCriarCapturaMultipartMuitoGrandeDevolve400(t *testing.T) {
	st := abrirStoreTeste(t)
	ap := appTeste(t, st)
	ap.SetCaptura(&captura.FakeTranscritor{}, &captura.FakeInterpretador{}, "modelo-teste")
	srv := New(st, ap, "teste")

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="audio"; filename="audio.webm"`}
	h["Content-Type"] = []string{"audio/webm"}
	pw, _ := mw.CreatePart(h)
	grande := make([]byte, maxAudioBytes+1024)
	pw.Write(grande)
	mw.Close()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/captura", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCriarCapturaErroDeLimiteDevolve429(t *testing.T) {
	st := abrirStoreTeste(t)
	ap := appTeste(t, st)
	interp := &captura.FakeInterpretador{Err: captura.ErrLimite{Msg: "limite do Groq atingido — espere um minuto ou use o formulário"}}
	ap.SetCaptura(nil, interp, "modelo-teste")
	srv := New(st, ap, "teste")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/captura", strings.NewReader(`{"texto":"dentista amanhã"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rr.Code)
	}
}

func TestHandleCriarCapturaErroDeProvedorDevolve502(t *testing.T) {
	st := abrirStoreTeste(t)
	ap := appTeste(t, st)
	interp := &captura.FakeInterpretador{Err: captura.ErrProvedor{Msg: "groq indisponível"}}
	ap.SetCaptura(nil, interp, "modelo-teste")
	srv := New(st, ap, "teste")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/captura", strings.NewReader(`{"texto":"dentista amanhã"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rr.Code)
	}
}

// V3: a transcrição já feita volta junto do 502 quando só a interpretação
// falhou — o front oferece "Salvar como tarefa" com ela.
func TestHandleCriarCapturaErroDeProvedorDevolveTexto(t *testing.T) {
	st := abrirStoreTeste(t)
	ap := appTeste(t, st)
	ap.SetCaptura(&captura.FakeTranscritor{Texto: "ligar pro João"},
		&captura.FakeInterpretador{Err: captura.ErrProvedor{Msg: "groq indisponível"}}, "modelo-teste")
	srv := New(st, ap, "teste")

	req := requisicaoMultipartAudioTeste(t, "audio/webm", []byte("OggS-fake"))
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	var body map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if rr.Code != http.StatusBadGateway || body["texto"] != "ligar pro João" || body["erro"] == "" {
		t.Fatalf("status = %d, body = %v", rr.Code, body)
	}
}

func TestHandleCapturaDesfecho(t *testing.T) {
	st := abrirStoreTeste(t)
	ap := appTeste(t, st)
	interp := &captura.FakeInterpretador{ModelJSON: modelJSONDesconhecidoTeste()}
	ap.SetCaptura(nil, interp, "modelo-teste")
	srv := New(st, ap, "teste")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/captura", strings.NewReader(`{"texto":"hmm"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rr, req)
	var body map[string]any
	json.NewDecoder(rr.Body).Decode(&body)
	id := int64(body["captura_id"].(float64))

	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/captura/"+itoaHTTP(id)+"/desfecho", strings.NewReader(`{"desfecho":"confirmada"}`))
	req2.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr2.Code, rr2.Body.String())
	}
}

func TestHandleCapturaDesfechoInexistenteDevolve404(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "teste")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/captura/999/desfecho", strings.NewReader(`{"desfecho":"confirmada"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestHandleCapturaDesfechoInvalidoDevolve400(t *testing.T) {
	st := abrirStoreTeste(t)
	ap := appTeste(t, st)
	interp := &captura.FakeInterpretador{ModelJSON: modelJSONDesconhecidoTeste()}
	ap.SetCaptura(nil, interp, "modelo-teste")
	srv := New(st, ap, "teste")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/captura", strings.NewReader(`{"texto":"hmm"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rr, req)
	var body map[string]any
	json.NewDecoder(rr.Body).Decode(&body)
	id := int64(body["captura_id"].(float64))

	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/captura/"+itoaHTTP(id)+"/desfecho", strings.NewReader(`{"desfecho":"sei_la"}`))
	req2.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr2.Code)
	}
}

func itoaHTTP(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

// requisicaoMultipartAudioTeste monta um POST /api/captura multipart com o
// campo "audio" no mime dado.
func requisicaoMultipartAudioTeste(t *testing.T, mime string, dados []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="audio"; filename="audio.webm"`}
	h["Content-Type"] = []string{mime}
	pw, err := mw.CreatePart(h)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	pw.Write(dados)
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/captura", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// §3: avisos sai como [] (nunca null) — o front lê .length direto.
func TestHandleCriarCapturaAvisosNuncaNull(t *testing.T) {
	st := abrirStoreTeste(t)
	ap := appTeste(t, st)
	proposta := `{"acao":"compras","item":{"tipo":null,"titulo":null,"notas":null,"regra_tipo":null,"regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":[]},"registro":{"item_id":null,"titulo":null,"dia":null,"nota":null},"compras":{"lista_nome":null,"itens":[{"nome":"leite","quantidade":null}]},"resto":null}`
	ap.SetCaptura(nil, &captura.FakeInterpretador{ModelJSON: []byte(proposta)}, "modelo-teste")
	srv := New(st, ap, "teste")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/captura", strings.NewReader(`{"texto":"comprar leite"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"avisos":[]`) {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}
