package scheduler

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"pocket/internal/app"
	"pocket/internal/domain"
)

// pendentesDaLista é pendentesDoItem (scheduler_test.go), mas para lista
// (F3-2).
func pendentesDaLista(t *testing.T, e *ambiente, listaID int64, ate time.Time) []domain.Notificacao {
	t.Helper()
	todas, err := e.st.NotificacoesPendentesAte(context.Background(), domain.FormatInstante(ate))
	if err != nil {
		t.Fatalf("NotificacoesPendentesAte: %v", err)
	}
	var out []domain.Notificacao
	for _, n := range todas {
		if n.ListaID != nil && *n.ListaID == listaID {
			out = append(out, n)
		}
	}
	return out
}

// criarListaComLembrete cria uma lista com o lembrete dado e devolve seu id.
func criarListaComLembrete(t *testing.T, e *ambiente, nome string, regra []byte, horarios []string) domain.Lista {
	t.Helper()
	l, err := e.ap.CriarLista(context.Background(), app.ListaEntrada{Nome: nome})
	if err != nil {
		t.Fatalf("CriarLista: %v", err)
	}
	l, err = e.ap.EditarLista(context.Background(), l.ID, app.ListaPatch{
		LembretePresente: true,
		Lembrete:         &app.ListaLembreteEntrada{Regra: regra, Horarios: horarios},
	})
	if err != nil {
		t.Fatalf("EditarLista (lembrete): %v", err)
	}
	return l
}

// ultimoEnvioListaPayload decodifica o payload do último envio como
// listaPayload (mesmo espírito de ultimoNotificacaoID).
func ultimoEnvioListaPayload(t *testing.T, e *ambiente) listaPayload {
	t.Helper()
	if len(e.pusher.Envios) == 0 {
		t.Fatalf("nenhum envio registrado")
	}
	var p listaPayload
	if err := json.Unmarshal(e.pusher.Envios[len(e.pusher.Envios)-1].Payload, &p); err != nil {
		t.Fatalf("decodificar payload de lista: %v", err)
	}
	return p
}

// TestLembreteListaGeraSoNosDiasCertos: regra dias_semana só sábado — 2026-
// 09-28 é segunda, então não deve gerar nada; 2026-10-03 (sábado) sim.
func TestLembreteListaGeraSoNosDiasCertos(t *testing.T) {
	loc := mustLoc(t)
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	segunda := time.Date(2026, 9, 28, 8, 0, 0, 0, loc)
	e := novoAmbiente(t, dbPath, segunda)

	l := criarListaComLembrete(t, e, "Mercado teste",
		[]byte(`{"tipo":"dias_semana","dias":[6]}`), []string{"09:00"})

	pend := pendentesDaLista(t, e, l.ID, segunda.Add(24*time.Hour))
	if len(pend) != 0 {
		t.Fatalf("pendentes na segunda = %+v, want nenhuma (regra só sábado)", pend)
	}

	sabado := time.Date(2026, 10, 3, 8, 0, 0, 0, loc)
	e.fake.Set(sabado)
	e.sch.Tick(context.Background()) // roda o job diário do dia (idempotente).

	pend = pendentesDaLista(t, e, l.ID, sabado.Add(24*time.Hour))
	if len(pend) != 1 {
		t.Fatalf("pendentes no sábado = %+v, want 1", pend)
	}
	if pend[0].AgendadaPara != domain.FormatInstante(time.Date(2026, 10, 3, 9, 0, 0, 0, loc)) {
		t.Fatalf("agendada_para = %q, want 09:00 do sábado", pend[0].AgendadaPara)
	}
}

// TestLembreteListaGeracaoIdempotente: rodar o job diário de novo no mesmo
// dia não duplica a notificação pendente (mesmo índice único parcial
// notificacao_lista_unica).
func TestLembreteListaGeracaoIdempotente(t *testing.T) {
	loc := mustLoc(t)
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	d := time.Date(2026, 9, 28, 8, 0, 0, 0, loc)
	e := novoAmbiente(t, dbPath, d)

	l := criarListaComLembrete(t, e, "Mercado teste", []byte(`{"tipo":"diario"}`), []string{"09:00"})

	if err := e.ap.RodarJobsDiarios(context.Background()); err != nil {
		t.Fatalf("RodarJobsDiarios (2a chamada, mesmo dia): %v", err)
	}
	e.sch.Tick(context.Background())

	pend := pendentesDaLista(t, e, l.ID, d.Add(24*time.Hour))
	if len(pend) != 1 {
		t.Fatalf("pendentes após rodar o job 2x no mesmo dia = %+v, want 1 (idempotente)", pend)
	}
}

// TestLembreteListaK6EnviaComResumo: lista com itens pendentes em 2 seções
// — envia no horário, payload com tipo="lista", lista_id e o resumo certo.
func TestLembreteListaK6EnviaComResumo(t *testing.T) {
	loc := mustLoc(t)
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	d := time.Date(2026, 9, 28, 8, 0, 0, 0, loc)
	e := novoAmbiente(t, dbPath, d)
	inscreverPush(t, e, "https://push.exemplo/1")

	l := criarListaComLembrete(t, e, "Mercado teste", []byte(`{"tipo":"diario"}`), []string{"09:00"})
	det, err := e.ap.DetalheLista(context.Background(), l.ID)
	if err != nil {
		t.Fatalf("DetalheLista: %v", err)
	}
	geral := det.Secoes[0].ID
	frios, err := e.ap.CriarSecao(context.Background(), l.ID, "Frios e laticínios")
	if err != nil {
		t.Fatalf("CriarSecao: %v", err)
	}

	adicionar := func(nome string, secaoID int64) {
		if _, _, err := e.ap.AdicionarItem(context.Background(), l.ID, app.ItemCompraEntrada{Nome: nome, SecaoID: &secaoID}); err != nil {
			t.Fatalf("AdicionarItem(%s): %v", nome, err)
		}
	}
	adicionar("tomate", geral)
	adicionar("batata", geral)
	adicionar("leite", frios.ID)

	e.fake.Set(time.Date(2026, 9, 28, 9, 0, 0, 0, loc))
	e.sch.Tick(context.Background())

	if n := e.pusher.Contagem(); n != 1 {
		t.Fatalf("envios = %d, want 1", n)
	}
	p := ultimoEnvioListaPayload(t, e)
	if p.Tipo != domain.TipoNotifLista || p.ListaID != l.ID {
		t.Fatalf("payload = %+v, want tipo=lista lista_id=%d", p, l.ID)
	}
	if p.Title != "Compras — Mercado teste" {
		t.Fatalf("title = %q, want %q", p.Title, "Compras — Mercado teste")
	}
	wantBody := "3 itens: geral 2, frios e laticínios 1"
	if p.Body != wantBody {
		t.Fatalf("body = %q, want %q", p.Body, wantBody)
	}
}

// TestLembreteListaK7VaziaCancela: lista sem itens pendentes — no horário,
// a notificação cancela, ninguém recebe push.
func TestLembreteListaK7VaziaCancela(t *testing.T) {
	loc := mustLoc(t)
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	d := time.Date(2026, 9, 28, 8, 0, 0, 0, loc)
	e := novoAmbiente(t, dbPath, d)
	inscreverPush(t, e, "https://push.exemplo/1")

	l := criarListaComLembrete(t, e, "Mercado teste", []byte(`{"tipo":"diario"}`), []string{"09:00"})
	pend := pendentesDaLista(t, e, l.ID, d.Add(24*time.Hour))
	if len(pend) != 1 {
		t.Fatalf("pendentes após criar = %+v, want 1", pend)
	}
	notifID := pend[0].ID

	e.fake.Set(time.Date(2026, 9, 28, 9, 0, 0, 0, loc))
	e.sch.Tick(context.Background())

	if n := e.pusher.Contagem(); n != 0 {
		t.Fatalf("envios = %d, want 0 (lista vazia)", n)
	}
	n, err := e.st.NotificacaoBuscar(context.Background(), notifID)
	if err != nil {
		t.Fatalf("NotificacaoBuscar: %v", err)
	}
	if n.Status != domain.StatusNotifCancelada {
		t.Fatalf("status = %q, want %q", n.Status, domain.StatusNotifCancelada)
	}
}

// TestLembreteListaArquivarCancelaPendenteDoDia: arquivar a lista cancela
// a notificação de hoje que ainda não disparou.
func TestLembreteListaArquivarCancelaPendenteDoDia(t *testing.T) {
	loc := mustLoc(t)
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	d := time.Date(2026, 9, 28, 8, 0, 0, 0, loc)
	e := novoAmbiente(t, dbPath, d)

	l := criarListaComLembrete(t, e, "Mercado teste", []byte(`{"tipo":"diario"}`), []string{"09:00"})
	if pend := pendentesDaLista(t, e, l.ID, d.Add(24*time.Hour)); len(pend) != 1 {
		t.Fatalf("pendentes após criar = %+v, want 1", pend)
	}

	if _, err := e.ap.MudarEstadoLista(context.Background(), l.ID, true); err != nil {
		t.Fatalf("MudarEstadoLista(arquivar): %v", err)
	}

	if pend := pendentesDaLista(t, e, l.ID, d.Add(24*time.Hour)); len(pend) != 0 {
		t.Fatalf("pendentes após arquivar = %+v, want nenhuma", pend)
	}
}

// TestLembreteListaEditarHorarioTrocaAPendente: mudar o horário do
// lembrete cancela a pendente do horário antigo e cria a do novo.
func TestLembreteListaEditarHorarioTrocaAPendente(t *testing.T) {
	loc := mustLoc(t)
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	d := time.Date(2026, 9, 28, 8, 0, 0, 0, loc)
	e := novoAmbiente(t, dbPath, d)

	l := criarListaComLembrete(t, e, "Mercado teste", []byte(`{"tipo":"diario"}`), []string{"09:00"})
	antes := pendentesDaLista(t, e, l.ID, d.Add(24*time.Hour))
	if len(antes) != 1 || antes[0].AgendadaPara != domain.FormatInstante(time.Date(2026, 9, 28, 9, 0, 0, 0, loc)) {
		t.Fatalf("pendentes antes = %+v, want 1 às 09:00", antes)
	}

	if _, err := e.ap.EditarLista(context.Background(), l.ID, app.ListaPatch{
		LembretePresente: true,
		Lembrete:         &app.ListaLembreteEntrada{Regra: []byte(`{"tipo":"diario"}`), Horarios: []string{"20:00"}},
	}); err != nil {
		t.Fatalf("EditarLista (novo horário): %v", err)
	}

	depois := pendentesDaLista(t, e, l.ID, d.Add(24*time.Hour))
	if len(depois) != 1 || depois[0].AgendadaPara != domain.FormatInstante(time.Date(2026, 9, 28, 20, 0, 0, 0, loc)) {
		t.Fatalf("pendentes depois = %+v, want 1 às 20:00", depois)
	}
}

// TestLembreteListaFeitoAdiarDevolve409: /api/notificacoes/{id}/{feito|
// adiar} numa notificação de lista é 409 (§2.6: "sem ações de Feito/
// Adiar").
func TestLembreteListaFeitoAdiarDevolve409(t *testing.T) {
	loc := mustLoc(t)
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	d := time.Date(2026, 9, 28, 8, 0, 0, 0, loc)
	e := novoAmbiente(t, dbPath, d)

	l := criarListaComLembrete(t, e, "Mercado teste", []byte(`{"tipo":"diario"}`), []string{"09:00"})
	pend := pendentesDaLista(t, e, l.ID, d.Add(24*time.Hour))
	if len(pend) != 1 {
		t.Fatalf("pendentes após criar = %+v, want 1", pend)
	}
	notifID := pend[0].ID

	if err := e.ap.AcaoNotificacao(context.Background(), notifID, domain.AcaoFeito); !isErrConflito(err) {
		t.Fatalf("AcaoNotificacao(feito) em lista: err = %v, want ErrConflito", err)
	}
	if err := e.ap.AcaoNotificacao(context.Background(), notifID, domain.AcaoAdiar); !isErrConflito(err) {
		t.Fatalf("AcaoNotificacao(adiar) em lista: err = %v, want ErrConflito", err)
	}

	// recebida/abrir continuam funcionando.
	if err := e.ap.AcaoNotificacao(context.Background(), notifID, "recebida"); err != nil {
		t.Fatalf("AcaoNotificacao(recebida) em lista: %v", err)
	}
	if err := e.ap.AcaoNotificacao(context.Background(), notifID, domain.AcaoAbrir); err != nil {
		t.Fatalf("AcaoNotificacao(abrir) em lista: %v", err)
	}
}

func isErrConflito(err error) bool {
	_, ok := err.(app.ErrConflito)
	return ok
}
