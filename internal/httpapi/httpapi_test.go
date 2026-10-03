package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"pocket/internal/app"
	"pocket/internal/clock"
	"pocket/internal/store"
)

func abrirStoreTeste(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "pocket.db"), clock.NewFake(time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// appTeste monta um *app.App sobre o mesmo relógio/fuso usados nos testes
// deste pacote (America/Sao_Paulo, como pede a nota de ambiente do pedido).
func appTeste(t *testing.T, st *store.Store) *app.App {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	return app.New(st, clock.NewFake(time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)), loc, nil)
}

func TestHealthOk(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "1.2.3-teste")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decodificar resposta: %v", err)
	}
	if ok, _ := body["ok"].(bool); !ok {
		t.Errorf("ok = %v, want true", body["ok"])
	}
	if versao, _ := body["versao"].(string); versao != "1.2.3-teste" {
		t.Errorf("versao = %v, want %q", body["versao"], "1.2.3-teste")
	}
}

func TestHealthAposFecharBancoFicaNaoOk(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "dev")
	_ = st.Close()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	srv.ServeHTTP(rr, req)

	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decodificar resposta: %v", err)
	}
	if ok, _ := body["ok"].(bool); ok {
		t.Errorf("ok = %v, want false com o banco fechado", body["ok"])
	}
}

func TestRotaDesconhecidaDevolve404(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "dev")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/isso-nao-existe", nil)
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}
