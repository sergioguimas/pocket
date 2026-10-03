package httpapi

// Testes de comportamento geral da API além dos critérios C1–C6: mapeamento
// de erro (400/404/409), edição de item (regra muda -> refaz ocorrências
// abertas), transições de estado de compromisso, calendário e pular/
// despular.

import (
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestCriarItemTipoInvalidoDevolve400(t *testing.T) {
	amb := novoAmbiente(t, seg28(10))
	rr := amb.do(t, http.MethodPost, "/api/itens", itemRequest{Tipo: "xyz", Titulo: "algo"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	decodeInto(t, rr, &body)
	if body["erro"] == "" {
		t.Fatalf("corpo do erro = %v, want campo erro preenchido", body)
	}
}

func TestCriarItemHabitoSemRegraDevolve400(t *testing.T) {
	amb := novoAmbiente(t, seg28(10))
	rr := amb.do(t, http.MethodPost, "/api/itens", itemRequest{Tipo: "habito", Titulo: "ler"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
}

func TestDetalheItemInexistenteDevolve404(t *testing.T) {
	amb := novoAmbiente(t, seg28(10))
	rr := amb.do(t, http.MethodGet, "/api/itens/999", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rr.Code, rr.Body.String())
	}
}

func TestRegistrarEmItemArquivadoDevolve409(t *testing.T) {
	amb := novoAmbiente(t, seg28(10))
	item := criarItem(t, amb, itemRequest{Tipo: "habito", Titulo: "ler", Regra: rawJSON(`{"tipo":"diario"}`)})

	rr := amb.do(t, http.MethodPost, "/api/itens/"+itoa(item.ID)+"/estado", estadoRequest{Estado: "arquivado"})
	if rr.Code != http.StatusOK {
		t.Fatalf("arquivar status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr2 := amb.do(t, http.MethodPost, "/api/itens/"+itoa(item.ID)+"/registros", registroRequest{})
	if rr2.Code != http.StatusConflict {
		t.Fatalf("registrar em arquivado status=%d, want 409: %s", rr2.Code, rr2.Body.String())
	}
}

func TestRegistrarNoFuturoDevolve400(t *testing.T) {
	amb := novoAmbiente(t, seg28(10))
	item := criarItem(t, amb, itemRequest{Tipo: "habito", Titulo: "ler", Regra: rawJSON(`{"tipo":"diario"}`)})

	rr := amb.do(t, http.MethodPost, "/api/itens/"+itoa(item.ID)+"/registros", registroRequest{Em: "2026-10-15T10:00:00-03:00"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
}

func TestEditarItemComTipoNoCorpoDevolve400(t *testing.T) {
	amb := novoAmbiente(t, seg28(10))
	item := criarItem(t, amb, itemRequest{Tipo: "habito", Titulo: "ler", Regra: rawJSON(`{"tipo":"diario"}`)})

	tipo := "compromisso"
	rr := amb.do(t, http.MethodPatch, "/api/itens/"+itoa(item.ID), itemPatchRequest{Tipo: &tipo})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
}

func TestEditarItemMudaRegraRefazAbertasMantemFechadas(t *testing.T) {
	amb := novoAmbiente(t, seg28(10))
	item := criarItem(t, amb, itemRequest{Tipo: "habito", Titulo: "ler", Regra: rawJSON(`{"tipo":"diario"}`)})
	postRegistro(t, amb, item.ID, registroRequest{}) // dia 28: cumprida (fechada)

	amb.fake.Advance(24 * time.Hour) // dia 29
	amb.rodarJob(t)                  // gera a aberta de dia 29

	nova := rawJSON(`{"tipo":"dias_semana","dias":[1,3,5]}`)
	rr := amb.do(t, http.MethodPatch, "/api/itens/"+itoa(item.ID), itemPatchRequest{Regra: nova})
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH status=%d body=%s", rr.Code, rr.Body.String())
	}

	det := getDetalhe(t, amb, item.ID)
	// dia 28 (cumprida, fechada) continua intocado; a sequência de 1 dia
	// cumprido não pode ter sumido.
	achouDia28 := false
	for _, r := range det.Registros {
		if r.Em[:10] == "2026-09-28" {
			achouDia28 = true
		}
	}
	if !achouDia28 {
		t.Fatalf("registro do dia 28 sumiu: %+v", det.Registros)
	}
	if det.Item.Metricas == nil || det.Item.Metricas.MelhorSequencia < 1 {
		t.Fatalf("melhor sequencia após editar regra = %+v, want >= 1 (dia 28 preservado)", det.Item.Metricas)
	}
}

func TestMudarEstadoCompromissoConcluirEDesfazerReabre(t *testing.T) {
	amb := novoAmbiente(t, seg28(10))
	item := criarItem(t, amb, itemRequest{Tipo: "compromisso", Titulo: "dentista", Data: "2026-09-28"})

	rr := amb.do(t, http.MethodPost, "/api/itens/"+itoa(item.ID)+"/estado", estadoRequest{Estado: "concluido"})
	if rr.Code != http.StatusOK {
		t.Fatalf("concluir status=%d body=%s", rr.Code, rr.Body.String())
	}
	var concluido itemResponse
	decodeInto(t, rr, &concluido)
	if concluido.Estado != "concluido" {
		t.Fatalf("estado = %q, want concluido", concluido.Estado)
	}

	// concluido -> ativo é 409 ("desfaça o registro para reabrir")
	rrInvalido := amb.do(t, http.MethodPost, "/api/itens/"+itoa(item.ID)+"/estado", estadoRequest{Estado: "ativo"})
	if rrInvalido.Code != http.StatusConflict {
		t.Fatalf("concluido->ativo status=%d, want 409: %s", rrInvalido.Code, rrInvalido.Body.String())
	}

	det := getDetalhe(t, amb, item.ID)
	if len(det.Registros) != 1 {
		t.Fatalf("registros = %+v, want 1 (criado por concluir)", det.Registros)
	}

	rrDesfazer := amb.do(t, http.MethodDelete, "/api/registros/"+itoa(det.Registros[0].ID), nil)
	if rrDesfazer.Code != http.StatusOK {
		t.Fatalf("desfazer status=%d body=%s", rrDesfazer.Code, rrDesfazer.Body.String())
	}
	var reaberto itemResponse
	decodeInto(t, rrDesfazer, &reaberto)
	if reaberto.Estado != "ativo" {
		t.Fatalf("estado após desfazer = %q, want ativo", reaberto.Estado)
	}
}

func TestPularEDespularOcorrencia(t *testing.T) {
	amb := novoAmbiente(t, seg28(10))
	item := criarItem(t, amb, itemRequest{Tipo: "habito", Titulo: "ler", Regra: rawJSON(`{"tipo":"diario"}`)})

	hoje := getHoje(t, amb)
	ocID := hoje.Habitos[0].Ocorrencia.ID

	rr := amb.do(t, http.MethodPost, "/api/ocorrencias/"+itoa(ocID)+"/pular", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("pular status=%d body=%s", rr.Code, rr.Body.String())
	}

	det := getDetalhe(t, amb, item.ID)
	if det.OcorrenciaAberta != nil {
		t.Fatalf("ocorrencia_aberta após pular = %+v, want nil (pulada não é aberta)", det.OcorrenciaAberta)
	}

	rr2 := amb.do(t, http.MethodPost, "/api/ocorrencias/"+itoa(ocID)+"/despular", nil)
	if rr2.Code != http.StatusOK {
		t.Fatalf("despular status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	det2 := getDetalhe(t, amb, item.ID)
	if det2.OcorrenciaAberta == nil || det2.OcorrenciaAberta.Status != "aberta" {
		t.Fatalf("ocorrencia_aberta após despular = %+v, want aberta", det2.OcorrenciaAberta)
	}
}

func TestCalendarioMostraStatusERegistros(t *testing.T) {
	amb := novoAmbiente(t, seg28(10))
	item := criarItem(t, amb, itemRequest{Tipo: "habito", Titulo: "ler", Regra: rawJSON(`{"tipo":"diario"}`)})
	postRegistro(t, amb, item.ID, registroRequest{})

	rr := amb.do(t, http.MethodGet, "/api/itens/"+itoa(item.ID)+"/calendario?de=2026-09-28&ate=2026-09-28", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var dias []calendarioDiaResponse
	decodeInto(t, rr, &dias)
	if len(dias) != 1 || dias[0].Dia != "2026-09-28" || dias[0].Status != "cumprida" || dias[0].Registros != 1 {
		t.Fatalf("calendario = %+v, want 1 dia cumprida com 1 registro", dias)
	}
}

func TestCalendarioIntervaloMuitoGrandeDevolve400(t *testing.T) {
	amb := novoAmbiente(t, seg28(10))
	item := criarItem(t, amb, itemRequest{Tipo: "habito", Titulo: "ler", Regra: rawJSON(`{"tipo":"diario"}`)})

	rr := amb.do(t, http.MethodGet, "/api/itens/"+itoa(item.ID)+"/calendario?de=2020-01-01&ate=2026-09-28", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400: %s", rr.Code, rr.Body.String())
	}
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}
