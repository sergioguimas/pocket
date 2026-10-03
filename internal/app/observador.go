package app

import (
	"context"
	"log/slog"
	"time"

	"pocket/internal/clock"
	"pocket/internal/domain"
	"pocket/internal/store"
)

// observadorNotificacoes é a implementação de verdade do gancho Observador
// (app.go, deixado pela etapa 3) — gera e cancela notificações (CONTRATO
// §3.5) dentro da mesma transação de cada caso de uso que muda item ou
// ocorrência.
//
// DECISÃO (Opus, etapa 4): os métodos de Observador não devolvem erro (a
// interface foi fixada assim na etapa 3 — ver ATENÇÃO em app.go). Uma falha
// aqui (SQLite, na mesma tx que acabou de escrever com sucesso — bem raro)
// só é logada, não derruba a operação principal (criar item, registrar
// etc.): a confiabilidade do módulo de notificação não deveria travar a API
// síncrona do usuário. O preço é que, nesse caso raro, um lembrete pode não
// ser gerado/cancelado corretamente — o próximo evento que tocar o mesmo
// item (edição, o job diário) tende a corrigir.
type observadorNotificacoes struct {
	clock clock.Clock
	loc   *time.Location
}

// NovoObservadorNotificacoes cria o Observador de verdade. main.go injeta o
// resultado em app.New; nil (observadorNulo) segue valendo para as etapas
// 1–3 e para testes que não precisam de notificação.
func NovoObservadorNotificacoes(c clock.Clock, loc *time.Location) Observador {
	return &observadorNotificacoes{clock: c, loc: loc}
}

func (o *observadorNotificacoes) hoje() domain.Dia {
	return domain.Hoje(o.clock, o.loc)
}

// ItemMudou gera as notificações de hoje do item (§3.5: "ao criar/editar
// item") — cobre criação e qualquer edição (regra, horários, data, hora),
// mesmo quando a ocorrência em si não mudou (ex.: só os horários mudaram).
func (o *observadorNotificacoes) ItemMudou(tx *store.Tx, item domain.Item) {
	if err := gerarNotificacoesHoje(context.Background(), tx, item, o.hoje(), o.loc); err != nil {
		slog.Error("gerar notificações de hoje (item mudou)", "item_id", item.ID, "erro", err)
	}
}

// OcorrenciaMudou cobre dois gatilhos de §3.5: "ao abrir ocorrência nova"
// (oc virou "aberta" — pode ter notificação de hoje) e "cancelar pendentes
// quando a ocorrência é cumprida" (C9: oc deixou de ser "aberta" —
// cumprida, perdida ou pulada — os lembretes que ainda não dispararam não
// fazem mais sentido).
func (o *observadorNotificacoes) OcorrenciaMudou(tx *store.Tx, oc domain.Ocorrencia) {
	ctx := context.Background()

	if oc.Status == domain.StatusAberta {
		item, err := tx.ItemBuscar(ctx, oc.ItemID)
		if err != nil {
			slog.Error("buscar item para gerar notificação", "item_id", oc.ItemID, "erro", err)
			return
		}
		if err := gerarNotificacoesHoje(ctx, tx, item, o.hoje(), o.loc); err != nil {
			slog.Error("gerar notificações de hoje (ocorrência aberta)", "item_id", oc.ItemID, "erro", err)
		}
		return
	}

	if err := tx.NotificacoesCancelarPorOcorrencia(ctx, oc.ID); err != nil {
		slog.Error("cancelar notificações da ocorrência", "ocorrencia_id", oc.ID, "erro", err)
	}
}

// OcorrenciaApagando desvincula (e cancela as ainda pendentes) as
// notificações de oc ANTES do DELETE — ATENÇÃO em app.go:
// notificacao.ocorrencia_id não tem ON DELETE, então apagar a ocorrência
// sem isto deixaria referência solta.
func (o *observadorNotificacoes) OcorrenciaApagando(tx *store.Tx, oc domain.Ocorrencia) {
	ctx := context.Background()
	if err := tx.NotificacoesCancelarPorOcorrencia(ctx, oc.ID); err != nil {
		slog.Error("cancelar notificações da ocorrência apagada", "ocorrencia_id", oc.ID, "erro", err)
	}
	if err := tx.NotificacoesDesvincularOcorrencia(ctx, oc.ID); err != nil {
		slog.Error("desvincular notificações da ocorrência apagada", "ocorrencia_id", oc.ID, "erro", err)
	}
}

// gerarNotificacoesHoje calcula as notificações de hoje de item, a partir
// da ocorrência aberta corrente (ocorrenciaAbertaCorrente, consultas.go), e
// grava a diferença: cancela as "pendente" de hoje que não valem mais
// (horário mudou, ou não há mais ocorrência aberta — item pausado/
// arquivado/sem ocorrência) e insere as que faltam (idempotente por
// UNIQUE(item_id, agendada_para, tipo) — reexecutar não duplica).
func gerarNotificacoesHoje(ctx context.Context, tx *store.Tx, item domain.Item, hoje domain.Dia, loc *time.Location) error {
	if item.Estado != domain.EstadoAtivo {
		return cancelarPendentesDeHoje(ctx, tx, item.ID, hoje, nil)
	}

	ocorrencias, err := tx.OcorrenciasPorItem(ctx, item.ID)
	if err != nil {
		return err
	}
	aberta := ocorrenciaAbertaCorrente(item, ocorrencias, hoje)
	if aberta == nil {
		return cancelarPendentesDeHoje(ctx, tx, item.ID, hoje, nil)
	}

	agendas := domain.AgendasDoDia(item, hoje, aberta, loc)
	desejadas := make(map[string]bool, len(agendas))
	for _, t := range agendas {
		desejadas[domain.FormatInstante(t)] = true
	}

	if err := cancelarPendentesDeHoje(ctx, tx, item.ID, hoje, desejadas); err != nil {
		return err
	}

	ocID := aberta.ID
	itemID := item.ID
	for ts := range desejadas {
		n := domain.Notificacao{
			ItemID: &itemID, OcorrenciaID: &ocID, Tipo: domain.TipoNotifLembrete,
			AgendadaPara: ts, Status: domain.StatusNotifPendente,
		}
		if _, _, err := tx.NotificacaoInserir(ctx, n); err != nil {
			return err
		}
	}
	return nil
}

// cancelarPendentesDeHoje cancela as notificações "pendente" de hoje de
// item que não estão em manter (nil = cancela todas).
func cancelarPendentesDeHoje(ctx context.Context, tx *store.Tx, itemID int64, hoje domain.Dia, manter map[string]bool) error {
	pendentes, err := tx.NotificacoesPendentesDoDia(ctx, itemID, string(hoje))
	if err != nil {
		return err
	}
	for _, n := range pendentes {
		if manter != nil && manter[n.AgendadaPara] {
			continue
		}
		if err := tx.NotificacaoCancelar(ctx, n.ID); err != nil {
			return err
		}
	}
	return nil
}
