package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pocket/internal/push"
)

func TestVapidDevolveChavePublica(t *testing.T) {
	st := abrirStoreTeste(t)
	ap := appTeste(t, st)
	ap.SetPush(&push.Fake{}, "chave-publica-teste")
	srv := New(st, ap, "dev")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/push/vapid", nil)
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decodificar resposta: %v", err)
	}
	if body["chave_publica"] != "chave-publica-teste" {
		t.Fatalf("chave_publica = %q, want %q", body["chave_publica"], "chave-publica-teste")
	}
}

func TestPushSubscribeUpsertPorEndpoint(t *testing.T) {
	st := abrirStoreTeste(t)
	ap := appTeste(t, st)
	ap.SetPush(&push.Fake{}, "chave-publica-teste")
	srv := New(st, ap, "dev")

	corpo := `{"endpoint":"https://push.exemplo/1","keys":{"p256dh":"p1","auth":"a1"}}`

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/push/subscribe", strings.NewReader(corpo))
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, corpo=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	// chamar de novo com o mesmo endpoint (o app chama em toda abertura) —
	// upsert, não duplica.
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/push/subscribe", strings.NewReader(corpo))
	srv.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("status (2ª vez) = %d, want %d", rr2.Code, http.StatusOK)
	}

	subs, err := st.PushSubListar(req.Context())
	if err != nil {
		t.Fatalf("PushSubListar: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("inscrições = %d, want 1 (upsert)", len(subs))
	}
}

func TestPushSubscribeSemEndpointE400(t *testing.T) {
	st := abrirStoreTeste(t)
	ap := appTeste(t, st)
	ap.SetPush(&push.Fake{}, "chave-publica-teste")
	srv := New(st, ap, "dev")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/push/subscribe", strings.NewReader(`{}`))
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestPushTesteEnviaParaTodasAsInscricoes(t *testing.T) {
	st := abrirStoreTeste(t)
	ap := appTeste(t, st)
	fake := &push.Fake{}
	ap.SetPush(fake, "chave-publica-teste")
	srv := New(st, ap, "dev")

	for _, ep := range []string{"https://push.exemplo/1", "https://push.exemplo/2"} {
		corpo := `{"endpoint":"` + ep + `","keys":{"p256dh":"p","auth":"a"}}`
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/push/subscribe", strings.NewReader(corpo))
		srv.ServeHTTP(rr, req)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/push/teste", nil)
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, corpo=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var body map[string]int
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decodificar resposta: %v", err)
	}
	if body["enviados"] != 2 {
		t.Fatalf("enviados = %d, want 2", body["enviados"])
	}
	if fake.Contagem() != 2 {
		t.Fatalf("Fake.Contagem() = %d, want 2", fake.Contagem())
	}
}

func TestAcaoNotificacaoInvalidaE400(t *testing.T) {
	st := abrirStoreTeste(t)
	ap := appTeste(t, st)
	ap.SetPush(&push.Fake{}, "chave-publica-teste")
	srv := New(st, ap, "dev")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/notificacoes/1/isso-nao-existe", nil)
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, corpo=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
}

func TestAcaoNotificacaoInexistenteE404(t *testing.T) {
	st := abrirStoreTeste(t)
	ap := appTeste(t, st)
	ap.SetPush(&push.Fake{}, "chave-publica-teste")
	srv := New(st, ap, "dev")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/notificacoes/999/recebida", nil)
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, corpo=%s", rr.Code, http.StatusNotFound, rr.Body.String())
	}
}
