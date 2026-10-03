package app

import (
	"context"

	"pocket/internal/domain"
	"pocket/internal/store"
)

// nomeJobGerarOcorrencias é o nome (coluna job.nome) do job diário de
// geração/fechamento de ocorrências (§3.3 regra 1–2, decisão 7).
const nomeJobGerarOcorrencias = "gerar_ocorrencias"

// RodarJobsDiarios roda os jobs do dia se ainda não rodaram (idempotente
// por (nome, dia), mesmo mecanismo do Smaug). Chamado no boot (main.go) e,
// desde a etapa 4, a cada tick do scheduler (idempotente — vira um SELECT
// barato depois que já rodou hoje; é assim que "roda quando o dia vira"
// acontece sem um relógio de virada de dia dedicado).
//
// DECISÃO (Opus, etapa 4): um job só ("gerar_ocorrencias") cobre ocorrências
// e notificações do dia — mesma cadência (uma vez por dia), então dois jobs
// separados só duplicariam o mecanismo de (nome, dia) sem ganho.
func (a *App) RodarJobsDiarios(ctx context.Context) error {
	hoje := a.hoje()
	dataRef := string(hoje)
	return a.st.WithTx(ctx, func(tx *store.Tx) error {
		ja, err := tx.JobExecutado(ctx, nomeJobGerarOcorrencias, dataRef)
		if err != nil {
			return err
		}
		if ja {
			return nil
		}

		if err := a.gerarOcorrenciasDoDia(ctx, tx, hoje); err != nil {
			return err
		}
		if err := a.gerarNotificacoesDoDia(ctx, tx, hoje); err != nil {
			return err
		}
		if err := a.gerarNotificacoesListaDoDia(ctx, tx, hoje); err != nil {
			return err
		}
		// Poda de captura (fase 1.5, §5): mantém as 500 mais recentes.
		if err := a.podarCapturasNaTx(ctx, tx); err != nil {
			return err
		}

		// Marca só depois de sucesso (decisão 7) — se o job falhar no meio,
		// a próxima chamada (próximo boot, próximo tick) tenta de novo.
		return tx.JobMarcar(ctx, nomeJobGerarOcorrencias, dataRef, domain.FormatInstante(a.agora()))
	})
}

// gerarOcorrenciasDoDia aplica, para cada hábito ativo, FecharVencidas +
// GerarOcorrencias(hoje) (decisão 7). Compromisso não entra aqui — sua
// única ocorrência já nasce em CriarItem.
func (a *App) gerarOcorrenciasDoDia(ctx context.Context, tx *store.Tx, hoje domain.Dia) error {
	itens, err := tx.ItemListar(ctx, domain.TipoHabito, domain.EstadoAtivo)
	if err != nil {
		return err
	}

	for _, item := range itens {
		ocorrencias, err := tx.OcorrenciasPorItem(ctx, item.ID)
		if err != nil {
			return err
		}

		for _, venc := range domain.FecharVencidas(item, ocorrencias, hoje) {
			if err := tx.OcorrenciaAtualizar(ctx, venc); err != nil {
				return err
			}
			a.obs.OcorrenciaMudou(tx, venc)
		}

		for _, nova := range domain.GerarOcorrencias(item, ocorrencias, hoje) {
			id, inserida, err := tx.OcorrenciaInserir(ctx, nova)
			if err != nil {
				return err
			}
			if inserida {
				nova.ID = id
				a.obs.OcorrenciaMudou(tx, nova)
			}
		}
	}
	return nil
}

// gerarNotificacoesDoDia é a parte do job diário que trata §3.5 ("ao criar/
// editar item" já cobre o resto — este é o gatilho "job diário"): para
// cada item ativo (hábito ou compromisso), gera as notificações de hoje a
// partir da ocorrência aberta corrente. Cobre os casos que OcorrenciaMudou
// sozinho não pegaria: um hábito vezes_periodo/intervalo cuja ocorrência já
// existia (não mudou hoje) mas cujo dia de hoje ainda não tinha lembrete
// gerado (etapa 4).
func (a *App) gerarNotificacoesDoDia(ctx context.Context, tx *store.Tx, hoje domain.Dia) error {
	itens, err := tx.ItemListar(ctx, "", domain.EstadoAtivo)
	if err != nil {
		return err
	}
	for _, item := range itens {
		if err := gerarNotificacoesHoje(ctx, tx, item, hoje, a.loc); err != nil {
			return err
		}
	}
	return nil
}
