package httpapi

// Testes ponta a ponta (httptest) dos critérios de aceite C1–C6 do
// CONTRATO §9, sobre banco em t.TempDir() e relógio falso — nenhuma rede,
// nenhum listener real. Datas fixas: 2026-09-28 é segunda (nota de
// ambiente do pedido da etapa 3).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"pocket/internal/app"
	"pocket/internal/clock"
	"pocket/internal/store"
)

// ambiente junta store+app+servidor sobre o mesmo relógio falso, para os
// testes poderem tanto bater na API (httptest) quanto avançar o tempo e
// rodar o job diário (equivalente ao boot/virada de dia — o scheduler de
// verdade só chega na etapa 4).
type ambiente struct {
	st   *store.Store
	ap   *app.App
	srv  *Server
	fake *clock.Fake
}

func seg28(hora int) time.Time {
	return time.Date(2026, 9, 28, hora, 0, 0, 0, time.UTC)
}

func novoAmbiente(t *testing.T, inicio time.Time) *ambiente {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	dir := t.TempDir()
	fake := clock.NewFake(inicio)
	st, err := store.Open(filepath.Join(dir, "pocket.db"), fake)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ap := app.New(st, fake, loc, nil)
	if err := ap.RodarJobsDiarios(context.Background()); err != nil {
		t.Fatalf("RodarJobsDiarios no boot do ambiente de teste: %v", err)
	}
	return &ambiente{st: st, ap: ap, srv: New(st, ap, "teste"), fake: fake}
}

// rodarJob roda o job diário — equivalente ao boot/virada de dia (etapa 4
// terá o scheduler de verdade).
func (amb *ambiente) rodarJob(t *testing.T) {
	t.Helper()
	if err := amb.ap.RodarJobsDiarios(context.Background()); err != nil {
		t.Fatalf("RodarJobsDiarios: %v", err)
	}
}

func (amb *ambiente) do(t *testing.T, method, caminho string, corpo any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if corpo != nil {
		b, err := json.Marshal(corpo)
		if err != nil {
			t.Fatalf("marshal corpo: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, caminho, reader)
	rr := httptest.NewRecorder()
	amb.srv.ServeHTTP(rr, req)
	return rr
}

func decodeInto(t *testing.T, rr *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rr.Body.Bytes(), v); err != nil {
		t.Fatalf("decodificar resposta: %v\nstatus=%d body=%s", err, rr.Code, rr.Body.String())
	}
}

func rawJSON(s string) *json.RawMessage {
	r := json.RawMessage(s)
	return &r
}

func criarItem(t *testing.T, amb *ambiente, req itemRequest) itemResponse {
	t.Helper()
	rr := amb.do(t, http.MethodPost, "/api/itens", req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST /api/itens (%s) status=%d body=%s", req.Titulo, rr.Code, rr.Body.String())
	}
	var resp itemResponse
	decodeInto(t, rr, &resp)
	return resp
}

func postRegistro(t *testing.T, amb *ambiente, itemID int64, req registroRequest) registroResultResponse {
	t.Helper()
	rr := amb.do(t, http.MethodPost, fmt.Sprintf("/api/itens/%d/registros", itemID), req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST /api/itens/%d/registros status=%d body=%s", itemID, rr.Code, rr.Body.String())
	}
	var resp registroResultResponse
	decodeInto(t, rr, &resp)
	return resp
}

func getHoje(t *testing.T, amb *ambiente) hojeResponse {
	t.Helper()
	rr := amb.do(t, http.MethodGet, "/api/hoje", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/hoje status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp hojeResponse
	decodeInto(t, rr, &resp)
	return resp
}

func getDetalhe(t *testing.T, amb *ambiente, itemID int64) itemDetalheResponse {
	t.Helper()
	rr := amb.do(t, http.MethodGet, fmt.Sprintf("/api/itens/%d", itemID), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/itens/%d status=%d body=%s", itemID, rr.Code, rr.Body.String())
	}
	var resp itemDetalheResponse
	decodeInto(t, rr, &resp)
	return resp
}

// --- C1: criar um hábito de cada formato e um compromisso com/sem data ---

func TestFluxoC1CriarQuatroFormatosECompromissos(t *testing.T) {
	amb := novoAmbiente(t, seg28(10))

	criarItem(t, amb, itemRequest{Tipo: "habito", Titulo: "diario", Regra: rawJSON(`{"tipo":"diario"}`)})
	criarItem(t, amb, itemRequest{Tipo: "habito", Titulo: "dias da semana", Regra: rawJSON(`{"tipo":"dias_semana","dias":[1,3,5]}`)})
	criarItem(t, amb, itemRequest{Tipo: "habito", Titulo: "vezes por semana", Regra: rawJSON(`{"tipo":"vezes_periodo","vezes":3,"periodo":"semana"}`)})
	criarItem(t, amb, itemRequest{Tipo: "habito", Titulo: "a cada 3 dias", Regra: rawJSON(`{"tipo":"intervalo","dias":3}`)})
	criarItem(t, amb, itemRequest{Tipo: "compromisso", Titulo: "dentista", Data: "2026-09-28", Hora: "15:00"})
	criarItem(t, amb, itemRequest{Tipo: "compromisso", Titulo: "tarefa solta"})

	hoje := getHoje(t, amb)
	if len(hoje.Habitos) != 4 {
		t.Fatalf("Habitos = %d, want 4: %+v", len(hoje.Habitos), hoje.Habitos)
	}
	if len(hoje.Compromissos) != 1 {
		t.Fatalf("Compromissos = %d, want 1 (dentista hoje): %+v", len(hoje.Compromissos), hoje.Compromissos)
	}
	if hoje.SemData != 1 {
		t.Fatalf("SemData = %d, want 1 (tarefa solta)", hoje.SemData)
	}
	for _, h := range hoje.Habitos {
		if h.Ocorrencia.Status != "aberta" {
			t.Errorf("hábito %q status = %q, want aberta", h.Item.Titulo, h.Ocorrencia.Status)
		}
	}
}

// --- C2: marcar e desfazer em Hoje atualiza sequência e taxa ---

func TestFluxoC2MarcarEDesfazerAtualizaSequenciaETaxa(t *testing.T) {
	amb := novoAmbiente(t, seg28(10))
	item := criarItem(t, amb, itemRequest{Tipo: "habito", Titulo: "ler", Regra: rawJSON(`{"tipo":"diario"}`)})

	resultado := postRegistro(t, amb, item.ID, registroRequest{})
	if resultado.Item.Metricas == nil || resultado.Item.Metricas.SequenciaAtual != 1 {
		t.Fatalf("metricas após marcar = %+v, want sequencia 1", resultado.Item.Metricas)
	}

	rr := amb.do(t, http.MethodDelete, fmt.Sprintf("/api/registros/%d", resultado.Registro.ID), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE /api/registros/%d status=%d body=%s", resultado.Registro.ID, rr.Code, rr.Body.String())
	}
	var itemDesfeito itemResponse
	decodeInto(t, rr, &itemDesfeito)
	if itemDesfeito.Metricas == nil || itemDesfeito.Metricas.SequenciaAtual != 0 {
		t.Fatalf("metricas após desfazer = %+v, want sequencia 0", itemDesfeito.Metricas)
	}
}

// --- C3: vezes_periodo 3×/semana: chip mostra 1/3, 2/3, 3/3 ---

func TestFluxoC3VezesPeriodoChipProgride(t *testing.T) {
	amb := novoAmbiente(t, seg28(10))
	item := criarItem(t, amb, itemRequest{
		Tipo: "habito", Titulo: "academia", Regra: rawJSON(`{"tipo":"vezes_periodo","vezes":3,"periodo":"semana"}`),
	})

	for i, want := range []int{1, 2, 3} {
		postRegistro(t, amb, item.ID, registroRequest{})

		hoje := getHoje(t, amb)
		if len(hoje.Habitos) != 1 {
			t.Fatalf("iteração %d: Habitos = %+v, want 1", i, hoje.Habitos)
		}
		oc := hoje.Habitos[0].Ocorrencia
		if oc.Feitos == nil || *oc.Feitos != want {
			t.Fatalf("iteração %d: feitos = %v, want %d", i, oc.Feitos, want)
		}
		wantStatus := "aberta"
		if want == 3 {
			wantStatus = "cumprida"
		}
		if oc.Status != wantStatus {
			t.Fatalf("iteração %d: status = %q, want %q", i, oc.Status, wantStatus)
		}
	}
}

// --- C4: intervalo 3 dias: marcar some de Hoje e volta no 3º dia; sem marcar, "atrasado 1 dia" ---

func TestFluxoC4IntervaloSomeDeHojeEVoltaNoTerceiroDia(t *testing.T) {
	amb := novoAmbiente(t, seg28(10))
	criarItem(t, amb, itemRequest{Tipo: "habito", Titulo: "regar", Regra: rawJSON(`{"tipo":"intervalo","dias":3}`)})

	hoje := getHoje(t, amb)
	if len(hoje.Habitos) != 1 {
		t.Fatalf("no dia da criação: Habitos = %+v, want 1", hoje.Habitos)
	}

	postRegistro(t, amb, hoje.Habitos[0].Item.ID, registroRequest{})

	hoje = getHoje(t, amb)
	if len(hoje.Habitos) != 0 {
		t.Fatalf("logo após marcar: Habitos = %+v, want vazio (próxima em dia)", hoje.Habitos)
	}

	amb.fake.Advance(2 * 24 * time.Hour) // +2 dias: ainda "em dia" (vence no 3º)
	hoje = getHoje(t, amb)
	if len(hoje.Habitos) != 0 {
		t.Fatalf("no 2º dia: Habitos = %+v, want vazio", hoje.Habitos)
	}

	amb.fake.Advance(24 * time.Hour) // +3 dias: vence, volta a aparecer
	hoje = getHoje(t, amb)
	if len(hoje.Habitos) != 1 {
		t.Fatalf("no 3º dia: Habitos = %+v, want 1", hoje.Habitos)
	}
}

func TestFluxoC4IntervaloSemMarcarMostraAtrasado(t *testing.T) {
	amb := novoAmbiente(t, seg28(10))
	criarItem(t, amb, itemRequest{Tipo: "habito", Titulo: "regar", Regra: rawJSON(`{"tipo":"intervalo","dias":3}`)})

	amb.fake.Advance(24 * time.Hour) // 1 dia sem marcar

	hoje := getHoje(t, amb)
	if len(hoje.Habitos) != 1 {
		t.Fatalf("Habitos = %+v, want 1", hoje.Habitos)
	}
	interv := hoje.Habitos[0].Intervalo
	if interv == nil || interv.EmDia || interv.Dias != 1 {
		t.Fatalf("intervalo = %+v, want atrasado 1 dia", interv)
	}
}

// --- C5: diário não marcado vira perdida após 00:00 e quebra a sequência; pulada não quebra ---

func TestFluxoC5DiarioNaoMarcadoViraPerdidaEQuebraSequencia(t *testing.T) {
	amb := novoAmbiente(t, seg28(10))
	item := criarItem(t, amb, itemRequest{Tipo: "habito", Titulo: "ler", Regra: rawJSON(`{"tipo":"diario"}`)})
	postRegistro(t, amb, item.ID, registroRequest{}) // dia 28 (segunda): cumprida

	amb.fake.Advance(24 * time.Hour) // dia 29
	amb.rodarJob(t)                  // gera a ocorrência de dia 29 (aberta) — não marca

	amb.fake.Advance(24 * time.Hour) // dia 30
	amb.rodarJob(t)                  // dia 29 (fim < hoje, ainda aberta) vira perdida

	det := getDetalhe(t, amb, item.ID)
	if det.Item.Metricas == nil || det.Item.Metricas.SequenciaAtual != 0 {
		t.Fatalf("sequencia após dia perdido = %+v, want 0", det.Item.Metricas)
	}
}

func TestFluxoC5PuladaNaoQuebraSequencia(t *testing.T) {
	amb := novoAmbiente(t, seg28(10))
	item := criarItem(t, amb, itemRequest{Tipo: "habito", Titulo: "ler", Regra: rawJSON(`{"tipo":"diario"}`)})
	postRegistro(t, amb, item.ID, registroRequest{}) // dia 28: cumprida

	amb.fake.Advance(24 * time.Hour) // dia 29
	amb.rodarJob(t)                  // gera a ocorrência de dia 29 (aberta)

	hoje := getHoje(t, amb)
	if len(hoje.Habitos) != 1 {
		t.Fatalf("dia 29: Habitos = %+v, want 1", hoje.Habitos)
	}
	rr := amb.do(t, http.MethodPost, fmt.Sprintf("/api/ocorrencias/%d/pular", hoje.Habitos[0].Ocorrencia.ID), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("pular status=%d body=%s", rr.Code, rr.Body.String())
	}

	amb.fake.Advance(24 * time.Hour) // dia 30
	amb.rodarJob(t)                  // dia 29 continua pulada (FecharVencidas só toca aberta); gera dia 30
	postRegistro(t, amb, item.ID, registroRequest{})

	det := getDetalhe(t, amb, item.ID)
	if det.Item.Metricas == nil || det.Item.Metricas.SequenciaAtual != 2 {
		t.Fatalf("sequencia com pulada no meio = %+v, want 2 (dia 28 + dia 30, pulada transparente)", det.Item.Metricas)
	}
}

// --- C6: marcação retroativa no calendário recupera a sequência ---

func TestFluxoC6RetroativoRecuperaSequencia(t *testing.T) {
	amb := novoAmbiente(t, seg28(10))
	item := criarItem(t, amb, itemRequest{Tipo: "habito", Titulo: "ler", Regra: rawJSON(`{"tipo":"diario"}`)})
	postRegistro(t, amb, item.ID, registroRequest{}) // dia 28: cumprida

	amb.fake.Advance(24 * time.Hour) // dia 29
	amb.rodarJob(t)                  // gera dia 29 — não marca

	amb.fake.Advance(24 * time.Hour) // dia 30
	amb.rodarJob(t)                  // dia 29 vira perdida; gera dia 30

	antes := getDetalhe(t, amb, item.ID)
	if antes.Item.Metricas == nil || antes.Item.Metricas.SequenciaAtual != 0 {
		t.Fatalf("antes do retroativo, sequencia = %+v, want 0", antes.Item.Metricas)
	}

	postRegistro(t, amb, item.ID, registroRequest{Em: "2026-09-29"}) // marca o dia perdido

	depois := getDetalhe(t, amb, item.ID)
	if depois.Item.Metricas == nil || depois.Item.Metricas.SequenciaAtual != 2 {
		t.Fatalf("depois do retroativo, sequencia = %+v, want 2 (dia 28 + dia 29)", depois.Item.Metricas)
	}
}
