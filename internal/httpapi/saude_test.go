package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pocket/internal/domain"
)

// agoraTeste é o "agora" do relógio falso usado por appTeste (2026-09-26
// 10:00 UTC) — os testes de saúde calculam validade/tick relativos a ele.
var agoraTeste = time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)

type certInfoFake struct {
	validade time.Time
	err      error
}

func (f certInfoFake) Validade() (time.Time, error) { return f.validade, f.err }

type tickInfoFake struct {
	ultimo time.Time
}

func (f tickInfoFake) UltimoTick() time.Time { return f.ultimo }

func pedirHealth(t *testing.T, srv *Server) map[string]any {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	srv.ServeHTTP(rr, req)
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decodificar resposta: %v", err)
	}
	return body
}

// TestHealthCompletoOk: cert com validade folgada, tick recente, duas
// inscrições — ok=true e os quatro campos novos presentes.
func TestHealthCompletoOk(t *testing.T) {
	st := abrirStoreTeste(t)
	if err := st.PushSubUpsert(context.Background(), domain.PushSub{Endpoint: "a", P256dh: "p", Auth: "a", CriadoEm: "2026-09-26T10:00:00Z"}); err != nil {
		t.Fatalf("PushSubUpsert: %v", err)
	}
	if err := st.PushSubUpsert(context.Background(), domain.PushSub{Endpoint: "b", P256dh: "p", Auth: "a", CriadoEm: "2026-09-26T10:00:00Z"}); err != nil {
		t.Fatalf("PushSubUpsert: %v", err)
	}

	srv := New(st, appTeste(t, st), "1.2.3")
	srv.SetSaude(certInfoFake{validade: agoraTeste.Add(30 * 24 * time.Hour)}, tickInfoFake{ultimo: agoraTeste})

	body := pedirHealth(t, srv)
	if ok, _ := body["ok"].(bool); !ok {
		t.Errorf("ok = %v, want true", body["ok"])
	}
	if body["tick_em"] == nil {
		t.Error("tick_em nulo, want preenchido")
	}
	if body["cert_expira_em"] == nil {
		t.Error("cert_expira_em nulo, want preenchido")
	}
	if subs, _ := body["subs"].(float64); subs != 2 {
		t.Errorf("subs = %v, want 2", body["subs"])
	}
}

// TestHealthCertVenceEmMenosDeSeteDiasFicaNaoOk cobre o primeiro dos dois
// gatilhos de ok=false (etapa 5).
func TestHealthCertVenceEmMenosDeSeteDiasFicaNaoOk(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "1.2.3")
	srv.SetSaude(certInfoFake{validade: agoraTeste.Add(5 * 24 * time.Hour)}, tickInfoFake{ultimo: agoraTeste})

	body := pedirHealth(t, srv)
	if ok, _ := body["ok"].(bool); ok {
		t.Errorf("ok = %v, want false (cert vence em 5 dias)", body["ok"])
	}
}

// TestHealthTickAntigoFicaNaoOk cobre o segundo gatilho: último tick há
// mais de 5 min.
func TestHealthTickAntigoFicaNaoOk(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "1.2.3")
	srv.SetSaude(certInfoFake{validade: agoraTeste.Add(30 * 24 * time.Hour)}, tickInfoFake{ultimo: agoraTeste.Add(-10 * time.Minute)})

	body := pedirHealth(t, srv)
	if ok, _ := body["ok"].(bool); ok {
		t.Errorf("ok = %v, want false (tick há 10 min)", body["ok"])
	}
}

// TestHealthSemSetSaudeMantemComportamentoAntigo: sem SetSaude (etapas
// 1–4), tick_em/cert_expira_em ficam nulos e ok não é afetado por eles.
func TestHealthSemSetSaudeMantemComportamentoAntigo(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "1.2.3")

	body := pedirHealth(t, srv)
	if ok, _ := body["ok"].(bool); !ok {
		t.Errorf("ok = %v, want true", body["ok"])
	}
	if body["tick_em"] != nil {
		t.Errorf("tick_em = %v, want nil sem SetSaude", body["tick_em"])
	}
	if body["cert_expira_em"] != nil {
		t.Errorf("cert_expira_em = %v, want nil sem SetSaude", body["cert_expira_em"])
	}
}
