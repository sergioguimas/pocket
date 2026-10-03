// Lembrete de lista (CONTRATO-FASE3.md §2.6) — etapa F3-2. Mesmo padrão do
// lembrete de item (internal/app/observador.go): o job diário
// (RodarJobsDiarios) gera as notificações de hoje para cada lista com
// lembrete, e editar o lembrete ou arquivar a lista sincroniza na hora
// (mesmo espírito de ItemMudou/OcorrenciaMudou, mas chamado direto pelos
// casos de uso de lista — listas não têm um gancho de Observador próprio,
// não valia a pena introduzir uma segunda interface só para isto).
package app

import (
	"context"

	"pocket/internal/domain"
	"pocket/internal/store"
)

// gerarNotificacoesListaDoDia é a parte do job diário para listas: para
// cada lista não arquivada, sincroniza as notificações de hoje com o
// lembrete (ou a ausência dele).
func (a *App) gerarNotificacoesListaDoDia(ctx context.Context, tx *store.Tx, hoje domain.Dia) error {
	listas, err := tx.ListaListar(ctx, true) // só não arquivadas
	if err != nil {
		return err
	}
	for _, l := range listas {
		if err := a.sincronizarNotificacoesListaHoje(ctx, tx, l, hoje); err != nil {
			return err
		}
	}
	return nil
}

// sincronizarNotificacoesListaHoje grava a diferença entre o que devia
// existir hoje (domain.AgendasListaDoDia, vazio se a lista está arquivada
// ou sem lembrete) e o que já existe "pendente" — cancela o que não vale
// mais e insere o que falta. Idempotente por natureza: reexecutar no mesmo
// estado não muda nada (o índice único parcial de notificacao_lista_unica
// garante isso mesmo sob corrida).
//
// DECISÃO (F3-2): gera a notificação de hoje mesmo que o horário já tenha
// passado quando o lembrete é criado/editado — mesmo espírito da fase 1
// (gerarNotificacoesHoje de item também não filtra passado/futuro; ver
// observador.go). O scheduler já expira, sem enviar, quem venceu há mais
// de 30 min (mesma tolerância de sempre, decisão A3) — reaproveitar esse
// mecanismo é mais simples e mais consistente do que duplicar uma regra de
// "não gera se já passou" aqui.
func (a *App) sincronizarNotificacoesListaHoje(ctx context.Context, tx *store.Tx, l domain.Lista, hoje domain.Dia) error {
	var desejadas map[string]bool
	if !l.Arquivada && l.Lembrete != nil {
		agendas := domain.AgendasListaDoDia(l.Lembrete, hoje, a.loc)
		desejadas = make(map[string]bool, len(agendas))
		for _, t := range agendas {
			desejadas[domain.FormatInstante(t)] = true
		}
	}

	pendentes, err := tx.NotificacoesListaPendentesDoDia(ctx, l.ID, string(hoje))
	if err != nil {
		return err
	}
	for _, n := range pendentes {
		if desejadas[n.AgendadaPara] {
			continue
		}
		if err := tx.NotificacaoCancelar(ctx, n.ID); err != nil {
			return err
		}
	}

	listaID := l.ID
	for ts := range desejadas {
		n := domain.Notificacao{
			ListaID: &listaID, Tipo: domain.TipoNotifLista,
			AgendadaPara: ts, Status: domain.StatusNotifPendente,
		}
		if _, _, err := tx.NotificacaoInserir(ctx, n); err != nil {
			return err
		}
	}
	return nil
}
