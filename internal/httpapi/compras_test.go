package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func fazerJSON(t *testing.T, srv *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		r = httptest.NewRequest(method, path, bytes.NewReader(b))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, r)
	return rr
}

func decodeBody(t *testing.T, rr *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.NewDecoder(rr.Body).Decode(v); err != nil {
		t.Fatalf("decodificar resposta (%s): %v", rr.Body.String(), err)
	}
}

func TestListasCRUDViaHTTP(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "1.2.3-teste")

	rr := fazerJSON(t, srv, http.MethodPost, "/api/listas", map[string]string{"nome": "Farmácia"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST /api/listas status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var lista listaResponse
	decodeBody(t, rr, &lista)
	if lista.Nome != "Farmácia" || lista.Arquivada {
		t.Fatalf("lista criada = %+v", lista)
	}

	rr = fazerJSON(t, srv, http.MethodGet, "/api/listas", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/listas status = %d", rr.Code)
	}
	var todas []listaResponse
	decodeBody(t, rr, &todas)
	// migração 0002 semeia "Mercado" — Farmácia + Mercado = 2.
	if len(todas) != 2 {
		t.Fatalf("GET /api/listas = %d listas, want 2 (Mercado + Farmácia): %+v", len(todas), todas)
	}

	rr = fazerJSON(t, srv, http.MethodGet, "/api/listas/"+itoa(lista.ID), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/listas/{id} status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var det listaDetalheResponse
	decodeBody(t, rr, &det)
	if len(det.Secoes) != 1 || det.Secoes[0].Nome != "Geral" {
		t.Fatalf("detalhe.Secoes = %+v, want só 'Geral'", det.Secoes)
	}

	rr = fazerJSON(t, srv, http.MethodPost, "/api/listas/"+itoa(lista.ID)+"/estado", map[string]bool{"arquivada": true})
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /estado status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var arquivada listaResponse
	decodeBody(t, rr, &arquivada)
	if !arquivada.Arquivada {
		t.Fatalf("arquivada.Arquivada = false após POST /estado")
	}

	// arquivada não aparece mais em GET /api/listas.
	rr = fazerJSON(t, srv, http.MethodGet, "/api/listas", nil)
	decodeBody(t, rr, &todas)
	if len(todas) != 1 {
		t.Fatalf("GET /api/listas após arquivar = %d, want 1 (só Mercado)", len(todas))
	}
}

func TestListaLembreteViaHTTPNullRemove(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "1.2.3-teste")

	rr := fazerJSON(t, srv, http.MethodPost, "/api/listas", map[string]string{"nome": "Farmácia"})
	var lista listaResponse
	decodeBody(t, rr, &lista)

	rr = fazerJSON(t, srv, http.MethodPatch, "/api/listas/"+itoa(lista.ID), map[string]any{
		"lembrete": map[string]any{"regra": map[string]string{"tipo": "diario"}, "horarios": []string{"09:00"}},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH lembrete status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var comLembrete listaResponse
	decodeBody(t, rr, &comLembrete)
	if comLembrete.Lembrete == nil || len(comLembrete.Lembrete.Horarios) != 1 {
		t.Fatalf("lembrete não gravado: %+v", comLembrete)
	}

	rr = fazerJSON(t, srv, http.MethodPatch, "/api/listas/"+itoa(lista.ID), map[string]any{"lembrete": nil})
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH lembrete:null status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var semLembrete listaResponse
	decodeBody(t, rr, &semLembrete)
	if semLembrete.Lembrete != nil {
		t.Fatalf("lembrete = %+v, want nil após lembrete:null", semLembrete.Lembrete)
	}

	// PATCH sem o campo "lembrete" não mexe (continua nil).
	rr = fazerJSON(t, srv, http.MethodPatch, "/api/listas/"+itoa(lista.ID), map[string]any{"ordem": 3})
	decodeBody(t, rr, &semLembrete)
	if semLembrete.Lembrete != nil || semLembrete.Ordem != 3 {
		t.Fatalf("após PATCH sem lembrete = %+v", semLembrete)
	}
}

func TestItensDedupeERiscarViaHTTP(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "1.2.3-teste")

	rr := fazerJSON(t, srv, http.MethodPost, "/api/listas", map[string]string{"nome": "Mercado teste"})
	var lista listaResponse
	decodeBody(t, rr, &lista)

	rr = fazerJSON(t, srv, http.MethodPost, "/api/listas/"+itoa(lista.ID)+"/itens", map[string]string{"nome": "leite", "quantidade": "2"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST itens (novo) status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var leite itemCompraResponse
	decodeBody(t, rr, &leite)

	rr = fazerJSON(t, srv, http.MethodPost, "/api/listas/"+itoa(lista.ID)+"/itens", map[string]string{"nome": "Leite", "quantidade": "3"})
	if rr.Code != http.StatusOK {
		t.Fatalf("POST itens (dedupe) status = %d, want 200, body=%s", rr.Code, rr.Body.String())
	}
	var leite2 itemCompraResponse
	decodeBody(t, rr, &leite2)
	if leite2.ID != leite.ID || leite2.Quantidade != "3" {
		t.Fatalf("leite2 = %+v, want id=%d quantidade=3", leite2, leite.ID)
	}

	rr = fazerJSON(t, srv, http.MethodPost, "/api/compras/"+itoa(leite.ID)+"/comprado", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST comprado status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var riscado itemCompraResponse
	decodeBody(t, rr, &riscado)
	if !riscado.Comprado {
		t.Fatalf("riscado.Comprado = false")
	}

	// idempotente: riscar de novo não muda nada nem quebra.
	rr = fazerJSON(t, srv, http.MethodPost, "/api/compras/"+itoa(leite.ID)+"/comprado", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST comprado (repetido) status = %d", rr.Code)
	}

	rr = fazerJSON(t, srv, http.MethodPost, "/api/listas/"+itoa(lista.ID)+"/limpar-comprados", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST limpar-comprados status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var limpeza map[string]int
	decodeBody(t, rr, &limpeza)
	if limpeza["removidos"] != 1 {
		t.Fatalf("limpeza = %+v, want removidos=1", limpeza)
	}
}

func TestApagarUltimaSecaoDa409ViaHTTP(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "1.2.3-teste")

	rr := fazerJSON(t, srv, http.MethodPost, "/api/listas", map[string]string{"nome": "Farmácia"})
	var lista listaResponse
	decodeBody(t, rr, &lista)

	rr = fazerJSON(t, srv, http.MethodGet, "/api/listas/"+itoa(lista.ID), nil)
	var det listaDetalheResponse
	decodeBody(t, rr, &det)
	geral := det.Secoes[0]

	rr = fazerJSON(t, srv, http.MethodDelete, "/api/secoes/"+itoa(geral.ID), nil)
	if rr.Code != http.StatusConflict {
		t.Fatalf("DELETE última seção status = %d, want 409, body=%s", rr.Code, rr.Body.String())
	}
	var corpo map[string]string
	decodeBody(t, rr, &corpo)
	if corpo["erro"] == "" {
		t.Fatalf("corpo do erro 409 sem 'erro': %+v", corpo)
	}
}

func TestHojeComComprasViaHTTP(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "1.2.3-teste")

	rr := fazerJSON(t, srv, http.MethodPost, "/api/listas", map[string]string{"nome": "Mercado teste"})
	var lista listaResponse
	decodeBody(t, rr, &lista)
	fazerJSON(t, srv, http.MethodPost, "/api/listas/"+itoa(lista.ID)+"/itens", map[string]string{"nome": "leite"})

	rr = fazerJSON(t, srv, http.MethodGet, "/api/hoje", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/hoje status = %d", rr.Code)
	}
	var hoje hojeResponse
	decodeBody(t, rr, &hoje)
	achou := false
	for _, c := range hoje.Compras {
		if c.ListaID == lista.ID && c.Pendentes == 1 {
			achou = true
		}
	}
	if !achou {
		t.Fatalf("hoje.Compras = %+v, want a lista %d com pendentes=1", hoje.Compras, lista.ID)
	}
}
