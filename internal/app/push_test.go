package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"pocket/internal/clock"
	"pocket/internal/domain"
	"pocket/internal/store"
)

// novaAppComObservador é novaAppTeste (app_test.go) com o Observador de
// notificações de verdade — novaAppTeste usa nil (observadorNulo) porque a
// maioria dos testes de app_test.go não olha para notificacao; os testes
// deste arquivo precisam que ItemMudou/OcorrenciaMudou gerem de fato as
// notificações pendentes (mesmo padrão de internal/scheduler/
// scheduler_test.go:novoAmbiente).
func novaAppComObservador(t *testing.T, inicio time.Time) (*App, *clock.Fake) {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	dir := t.TempDir()
	fake := clock.NewFake(inicio.In(loc))
	st, err := store.Open(filepath.Join(dir, "pocket.db"), fake)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	obs := NovoObservadorNotificacoes(fake, loc)
	return New(st, fake, loc, obs), fake
}

// TestAcaoAdiarDepoisDeConcluirDevolve409SemNovaPendente reproduz o bug de
// produção corrigido em F3-1: o compromisso "C10 teste" foi concluído às
// 11:31:47 (item.estado=concluido, ocorrência cumprida) e às 11:31:49 o
// usuário tocou "Adiar 30 min" numa notificação que já tinha sido
// ENTREGUE ao telefone (status="enviada") antes da conclusão — o caminho
// normal de conclusão só cancela notificações ainda "pendente"
// (notificacaoCancelar), então a já enviada sobrevive e "Adiar" criava uma
// pendente nova para um item que não existe mais como compromisso aberto.
func TestAcaoAdiarDepoisDeConcluirDevolve409SemNovaPendente(t *testing.T) {
	a, fake := novaAppComObservador(t, segunda28)
	ctx := context.Background()

	item, err := a.CriarItem(ctx, ItemEntrada{
		Tipo: domain.TipoCompromisso, Titulo: "C10 teste",
		Data: "2026-09-28", Hora: "11:31",
	})
	if err != nil {
		t.Fatalf("CriarItem: %v", err)
	}

	pendentes, err := a.st.NotificacoesPendentesAte(ctx, domain.FormatInstante(fake.Now().Add(24*time.Hour)))
	if err != nil {
		t.Fatalf("NotificacoesPendentesAte: %v", err)
	}
	if len(pendentes) != 1 {
		t.Fatalf("pendentes após criar = %+v, want 1 (lembrete das 11:31)", pendentes)
	}
	notifID := pendentes[0].ID

	// simula a entrega: a notificação já foi "enviada" ao telefone antes
	// do usuário concluir o compromisso (mesma ordem do bug: 11:31:47
	// concluído, 11:31:49 "Adiar" na notificação que já estava na tela).
	if _, err := a.st.NotificacaoMarcarEnviada(ctx, notifID, domain.FormatInstante(fake.Now())); err != nil {
		t.Fatalf("NotificacaoMarcarEnviada: %v", err)
	}

	if _, err := a.MudarEstado(ctx, item.ID, domain.EstadoConcluido); err != nil {
		t.Fatalf("MudarEstado(concluido): %v", err)
	}

	err = a.AcaoNotificacao(ctx, notifID, domain.AcaoAdiar)
	if _, ok := err.(ErrConflito); !ok {
		t.Fatalf("AcaoNotificacao(adiar) após concluir: err = %v, want ErrConflito", err)
	}

	// a ação é gravada mesmo assim (o toque aconteceu de verdade).
	n, err := a.st.NotificacaoBuscar(ctx, notifID)
	if err != nil {
		t.Fatalf("NotificacaoBuscar: %v", err)
	}
	if n.Acao == nil || *n.Acao != domain.AcaoAdiar {
		t.Fatalf("notificacao.Acao = %v, want %q (gravada mesmo com 409)", n.Acao, domain.AcaoAdiar)
	}

	// nenhuma notificação nova (+30min) foi criada.
	depois, err := a.st.NotificacoesPendentesAte(ctx, domain.FormatInstante(fake.Now().Add(24*time.Hour)))
	if err != nil {
		t.Fatalf("NotificacoesPendentesAte (depois): %v", err)
	}
	if len(depois) != 0 {
		t.Fatalf("pendentes após adiar em item concluído = %+v, want nenhuma", depois)
	}
}

// TestAcaoFeitoDepoisDeConcluirDevolve409SemRegistrarDeNovo é a mesma
// checagem para "Feito" (não registra de novo — a ação é só gravada, com
// 409).
func TestAcaoFeitoDepoisDeConcluirDevolve409SemRegistrarDeNovo(t *testing.T) {
	a, fake := novaAppComObservador(t, segunda28)
	ctx := context.Background()

	item, err := a.CriarItem(ctx, ItemEntrada{
		Tipo: domain.TipoCompromisso, Titulo: "C10 teste",
		Data: "2026-09-28", Hora: "11:31",
	})
	if err != nil {
		t.Fatalf("CriarItem: %v", err)
	}
	pendentes, err := a.st.NotificacoesPendentesAte(ctx, domain.FormatInstante(fake.Now().Add(24*time.Hour)))
	if err != nil || len(pendentes) != 1 {
		t.Fatalf("pendentes após criar = %+v, err=%v, want 1", pendentes, err)
	}
	notifID := pendentes[0].ID
	if _, err := a.st.NotificacaoMarcarEnviada(ctx, notifID, domain.FormatInstante(fake.Now())); err != nil {
		t.Fatalf("NotificacaoMarcarEnviada: %v", err)
	}

	if _, err := a.MudarEstado(ctx, item.ID, domain.EstadoConcluido); err != nil {
		t.Fatalf("MudarEstado(concluido): %v", err)
	}
	det, err := a.DetalheItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("DetalheItem: %v", err)
	}
	registrosAntes := len(det.Registros)

	err = a.AcaoNotificacao(ctx, notifID, domain.AcaoFeito)
	if _, ok := err.(ErrConflito); !ok {
		t.Fatalf("AcaoNotificacao(feito) após concluir: err = %v, want ErrConflito", err)
	}

	det, err = a.DetalheItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("DetalheItem (depois): %v", err)
	}
	if len(det.Registros) != registrosAntes {
		t.Fatalf("registros = %d, want %d (feito não deveria registrar de novo)", len(det.Registros), registrosAntes)
	}
}
