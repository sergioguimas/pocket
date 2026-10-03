package app

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"pocket/internal/domain"
	"pocket/internal/store"
)

// ItemEntrada é o corpo de POST /api/itens já separado em campos — o
// httpapi decodifica o JSON (§5) e passa a regra/data crus para o app
// validar (DECISÃO Opus, decisão 10/12: "validação no app, não no
// handler" — até o parse de domain.Regra e domain.Dia acontece aqui).
type ItemEntrada struct {
	Tipo     string
	Titulo   string
	Notas    string
	Regra    []byte // JSON de §3.2; só para habito
	Data     string // "YYYY-MM-DD" ou ""; só para compromisso
	Hora     string // "HH:MM" ou ""; só para compromisso
	Horarios []string
	Cor      string
	Ordem    int
}

// ItemPatch é o corpo de PATCH /api/itens/{id} (§5, decisão 6): todo campo é
// opcional — ponteiro nil (ou "null" explícito no JSON, que o encoding/json
// já colapsa em nil — DECISÃO: tratamos os dois como "não veio") significa
// "não mude este campo". TipoPresente é a exceção: tipo nunca é editável,
// então a mera presença da chave já é erro (§5/decisão 6: "tipo não muda →
// 400"), independente do valor.
type ItemPatch struct {
	TipoPresente bool
	Titulo       *string
	Notas        *string
	Regra        *[]byte
	Data         *string // ponteiro para "" apaga a data do compromisso
	Hora         *string // ponteiro para "" apaga a hora
	Horarios     *[]string
	Cor          *string
	Ordem        *int
}

// montarItem valida e monta um domain.Item novo a partir de in (§3.1, §3.2,
// §3.5) — nenhuma escrita ainda, só validação (via domain para regra/data/
// horários; o resto — forma dos campos, combinação com o tipo — é do app,
// que é quem conhece o formato de entrada da API).
func montarItem(agora time.Time, in ItemEntrada) (domain.Item, error) {
	titulo := strings.TrimSpace(in.Titulo)
	if utf8.RuneCountInString(titulo) < 1 || utf8.RuneCountInString(titulo) > 120 {
		return domain.Item{}, erroValidacao("titulo deve ter entre 1 e 120 caracteres")
	}

	item := domain.Item{
		Titulo: titulo, Notas: in.Notas, Cor: in.Cor, Ordem: in.Ordem,
		Estado: domain.EstadoAtivo, Horarios: in.Horarios,
		CriadoEm: agora, AtualizadoEm: agora,
	}
	if item.Horarios == nil {
		item.Horarios = []string{}
	}

	switch in.Tipo {
	case domain.TipoHabito:
		item.Tipo = domain.TipoHabito
		if len(in.Regra) == 0 {
			return domain.Item{}, erroValidacao("hábito precisa de uma regra de frequência")
		}
		regra, err := domain.ParseRegra(in.Regra)
		if err != nil {
			return domain.Item{}, ErrValidacao{Msg: err.Error()}
		}
		item.Regra = regra
		if in.Data != "" || in.Hora != "" {
			return domain.Item{}, erroValidacao("hábito não tem data nem hora — só regra de frequência")
		}

	case domain.TipoCompromisso:
		item.Tipo = domain.TipoCompromisso
		if len(in.Regra) > 0 {
			return domain.Item{}, erroValidacao("compromisso não tem regra de frequência")
		}
		if in.Data != "" {
			d, err := domain.ParseDia(in.Data)
			if err != nil {
				return domain.Item{}, ErrValidacao{Msg: err.Error()}
			}
			item.Data = &d
		}
		item.Hora = in.Hora
		if item.Hora != "" && item.Data == nil {
			return domain.Item{}, erroValidacao("hora requer data")
		}

	default:
		return domain.Item{}, erroValidacao(`tipo deve ser "habito" ou "compromisso" (veio %q)`, in.Tipo)
	}

	if err := domain.ValidarHorarios(item); err != nil {
		return domain.Item{}, ErrValidacao{Msg: err.Error()}
	}
	return item, nil
}

// CriarItem valida in, grava o item e gera as ocorrências que já valem na
// hora (§5 "gera ocorrências e notificações do dia na hora"; DECISÃO Opus,
// decisão 7: compromisso ganha a única, hábito ganha a de hoje/período).
func (a *App) CriarItem(ctx context.Context, in ItemEntrada) (domain.Item, error) {
	item, err := montarItem(a.agora(), in)
	if err != nil {
		return domain.Item{}, err
	}

	var criado domain.Item
	err = a.st.WithTx(ctx, func(tx *store.Tx) error {
		id, err := tx.ItemInserir(ctx, item)
		if err != nil {
			return err
		}
		item.ID = id
		criado = item

		hoje := a.hoje()
		for _, nova := range domain.GerarOcorrencias(item, nil, hoje) {
			ocID, inserida, err := tx.OcorrenciaInserir(ctx, nova)
			if err != nil {
				return err
			}
			if inserida {
				nova.ID = ocID
				a.obs.OcorrenciaMudou(tx, nova)
			}
		}
		a.obs.ItemMudou(tx, criado)
		return nil
	})
	return criado, err
}

// EditarItem aplica patch a um item existente (§5, decisão 6). Mudança de
// regra (hábito) ou de data (compromisso) não reescreve ocorrências
// fechadas — só apaga as ainda "aberta" com fim nulo ou fim >= hoje e as
// regenera, recalculando o status das novas com os registros que já
// existirem no intervalo delas.
func (a *App) EditarItem(ctx context.Context, id int64, patch ItemPatch) (domain.Item, error) {
	if patch.TipoPresente {
		return domain.Item{}, erroValidacao("tipo não pode ser alterado depois de criado")
	}

	var atualizado domain.Item
	err := a.st.WithTx(ctx, func(tx *store.Tx) error {
		atual, err := tx.ItemBuscar(ctx, id)
		if err != nil {
			return mapearErroStore(err, fmt.Sprintf("item %d não encontrado", id))
		}

		novo := atual
		if patch.Titulo != nil {
			t := strings.TrimSpace(*patch.Titulo)
			if utf8.RuneCountInString(t) < 1 || utf8.RuneCountInString(t) > 120 {
				return erroValidacao("titulo deve ter entre 1 e 120 caracteres")
			}
			novo.Titulo = t
		}
		if patch.Notas != nil {
			novo.Notas = *patch.Notas
		}
		if patch.Cor != nil {
			novo.Cor = *patch.Cor
		}
		if patch.Ordem != nil {
			novo.Ordem = *patch.Ordem
		}

		regraMudou := false
		if patch.Regra != nil {
			if novo.Tipo != domain.TipoHabito {
				return erroValidacao("compromisso não tem regra de frequência")
			}
			regra, err := domain.ParseRegra(*patch.Regra)
			if err != nil {
				return ErrValidacao{Msg: err.Error()}
			}
			if novo.Regra == nil || !regraIgual(*novo.Regra, *regra) {
				regraMudou = true
			}
			novo.Regra = regra
		}

		dataMudou := false
		if patch.Data != nil {
			if novo.Tipo != domain.TipoCompromisso {
				return erroValidacao("hábito não tem data")
			}
			if *patch.Data == "" {
				if novo.Data != nil {
					dataMudou = true
				}
				novo.Data = nil
			} else {
				d, err := domain.ParseDia(*patch.Data)
				if err != nil {
					return ErrValidacao{Msg: err.Error()}
				}
				if novo.Data == nil || *novo.Data != d {
					dataMudou = true
				}
				novo.Data = &d
			}
		}
		if patch.Hora != nil {
			if novo.Tipo != domain.TipoCompromisso {
				return erroValidacao("hábito não tem hora")
			}
			novo.Hora = *patch.Hora
		}
		if novo.Hora != "" && novo.Data == nil {
			return erroValidacao("hora requer data")
		}
		if patch.Horarios != nil {
			novo.Horarios = *patch.Horarios
		}

		if err := domain.ValidarHorarios(novo); err != nil {
			return ErrValidacao{Msg: err.Error()}
		}

		hoje := a.hoje()
		novo.AtualizadoEm = a.agora()
		if err := tx.ItemAtualizar(ctx, novo); err != nil {
			return err
		}

		mudouGeracao := (novo.Tipo == domain.TipoHabito && regraMudou) ||
			(novo.Tipo == domain.TipoCompromisso && dataMudou)
		if mudouGeracao {
			if err := a.refazerOcorrenciasAbertas(ctx, tx, novo, hoje); err != nil {
				return err
			}
		}

		a.obs.ItemMudou(tx, novo)
		atualizado = novo
		return nil
	})
	return atualizado, err
}

// refazerOcorrenciasAbertas apaga as ocorrências "não fechadas" (aberta com
// fim nulo ou fim >= hoje — as fechadas ficam intocadas, mesmo com fim >=
// hoje) e gera de novo para o item já editado, recalculando o status de
// cada nova com os registros que já existirem no intervalo dela (decisão 6).
func (a *App) refazerOcorrenciasAbertas(ctx context.Context, tx *store.Tx, item domain.Item, hoje domain.Dia) error {
	ocorrencias, err := tx.OcorrenciasPorItem(ctx, item.ID)
	if err != nil {
		return err
	}

	var restantes []domain.Ocorrencia
	for _, oc := range ocorrencias {
		if oc.Status == domain.StatusAberta && (oc.Fim == nil || *oc.Fim >= hoje) {
			// ATENÇÃO (decisão 8): avisa o Observador ANTES do DELETE —
			// notificacao.ocorrencia_id não tem ON DELETE.
			a.obs.OcorrenciaApagando(tx, oc)
			if err := tx.OcorrenciaApagar(ctx, oc.ID); err != nil {
				return err
			}
			continue
		}
		restantes = append(restantes, oc)
	}

	novas := domain.GerarOcorrencias(item, restantes, hoje)
	if len(novas) == 0 {
		return nil
	}

	registros, err := tx.RegistrosPorItem(ctx, item.ID)
	if err != nil {
		return err
	}

	for _, nova := range novas {
		n := contarRegistrosNoIntervalo(registros, nova, a.loc)
		nova.Status = domain.StatusApos(item, nova, n, hoje)
		id, inserida, err := tx.OcorrenciaInserir(ctx, nova)
		if err != nil {
			return err
		}
		if inserida {
			nova.ID = id
			a.obs.OcorrenciaMudou(tx, nova)
		}
	}
	return nil
}

// MudarEstado aplica uma transição de estado (§5 POST /api/itens/{id}/
// estado, decisão 5). Transições válidas dependem do tipo do item — ver
// mudarEstadoHabito e mudarEstadoCompromisso.
func (a *App) MudarEstado(ctx context.Context, id int64, novoEstado string) (domain.Item, error) {
	switch novoEstado {
	case domain.EstadoAtivo, domain.EstadoPausado, domain.EstadoArquivado, domain.EstadoConcluido:
	default:
		return domain.Item{}, erroValidacao("estado inválido (%q)", novoEstado)
	}

	var atualizado domain.Item
	err := a.st.WithTx(ctx, func(tx *store.Tx) error {
		item, err := tx.ItemBuscar(ctx, id)
		if err != nil {
			return mapearErroStore(err, fmt.Sprintf("item %d não encontrado", id))
		}

		var novo domain.Item
		switch item.Tipo {
		case domain.TipoHabito:
			novo, err = a.mudarEstadoHabito(ctx, tx, item, novoEstado)
		case domain.TipoCompromisso:
			novo, err = a.mudarEstadoCompromisso(ctx, tx, item, novoEstado)
		}
		if err != nil {
			return err
		}
		atualizado = novo
		return nil
	})
	return atualizado, err
}

// transicoesHabito são as transições de estado válidas para hábito
// (decisão 5): ativo<->pausado; qualquer (ativo/pausado) -> arquivado;
// arquivado -> ativo. Qualquer outra (inclui arquivado->pausado e
// permanecer no mesmo estado) é 409.
var transicoesHabito = map[string]map[string]bool{
	domain.EstadoAtivo:     {domain.EstadoPausado: true, domain.EstadoArquivado: true},
	domain.EstadoPausado:   {domain.EstadoAtivo: true, domain.EstadoArquivado: true},
	domain.EstadoArquivado: {domain.EstadoAtivo: true},
}

func (a *App) mudarEstadoHabito(ctx context.Context, tx *store.Tx, item domain.Item, novo string) (domain.Item, error) {
	if !transicoesHabito[item.Estado][novo] {
		return domain.Item{}, erroConflito("hábito não pode ir de %q para %q", item.Estado, novo)
	}

	// Ao pausar ou arquivar: ocorrências aberta com fim não nulo viram
	// pulada — senão virariam perdida (job diário) e quebrariam a sequência
	// durante a pausa. A aberta de intervalo (fim nulo) fica como está.
	if novo == domain.EstadoPausado || novo == domain.EstadoArquivado {
		if err := a.pularAbertasComFim(ctx, tx, item); err != nil {
			return domain.Item{}, err
		}
	}

	item.Estado = novo
	item.AtualizadoEm = a.agora()
	if err := tx.ItemAtualizar(ctx, item); err != nil {
		return domain.Item{}, err
	}

	// Ao retomar: gera a ocorrência de hoje/do período corrente se estiver
	// faltando (ficou pausado e perdeu a geração do job diário nesse meio
	// tempo).
	if novo == domain.EstadoAtivo {
		hoje := a.hoje()
		ocorrencias, err := tx.OcorrenciasPorItem(ctx, item.ID)
		if err != nil {
			return domain.Item{}, err
		}
		for _, nova := range domain.GerarOcorrencias(item, ocorrencias, hoje) {
			ocID, inserida, err := tx.OcorrenciaInserir(ctx, nova)
			if err != nil {
				return domain.Item{}, err
			}
			if inserida {
				nova.ID = ocID
				a.obs.OcorrenciaMudou(tx, nova)
			}
		}
	}

	a.obs.ItemMudou(tx, item)
	return item, nil
}

func (a *App) pularAbertasComFim(ctx context.Context, tx *store.Tx, item domain.Item) error {
	ocorrencias, err := tx.OcorrenciasPorItem(ctx, item.ID)
	if err != nil {
		return err
	}
	for _, oc := range ocorrencias {
		if oc.Status == domain.StatusAberta && oc.Fim != nil {
			oc.Status = domain.StatusPulada
			if err := tx.OcorrenciaAtualizar(ctx, oc); err != nil {
				return err
			}
			a.obs.OcorrenciaMudou(tx, oc)
		}
	}
	return nil
}

// mudarEstadoCompromisso (decisão 5): ativo->concluido é o mesmo que
// Registrar agora (origem app); concluido->ativo é 409 ("desfaça o
// registro para reabrir"); qualquer estado -> arquivado e arquivado ->
// ativo são permitidos; o resto é 409.
func (a *App) mudarEstadoCompromisso(ctx context.Context, tx *store.Tx, item domain.Item, novo string) (domain.Item, error) {
	switch {
	case item.Estado == domain.EstadoAtivo && novo == domain.EstadoConcluido:
		return a.concluirCompromissoNaTx(ctx, tx, item)

	case item.Estado == domain.EstadoConcluido && novo == domain.EstadoAtivo:
		return domain.Item{}, erroConflito("desfaça o registro para reabrir")

	case novo == domain.EstadoArquivado && item.Estado != domain.EstadoArquivado:
		return a.trocarEstadoSimples(ctx, tx, item, novo)

	case item.Estado == domain.EstadoArquivado && novo == domain.EstadoAtivo:
		return a.trocarEstadoSimples(ctx, tx, item, novo)

	default:
		return domain.Item{}, erroConflito("compromisso não pode ir de %q para %q", item.Estado, novo)
	}
}

// trocarEstadoSimples troca o estado sem tocar em ocorrências (arquivar e
// desarquivar compromisso — decisão 5 só fala de recalcular ocorrências ao
// pausar/arquivar hábito).
func (a *App) trocarEstadoSimples(ctx context.Context, tx *store.Tx, item domain.Item, novo string) (domain.Item, error) {
	item.Estado = novo
	item.AtualizadoEm = a.agora()
	if err := tx.ItemAtualizar(ctx, item); err != nil {
		return domain.Item{}, err
	}
	a.obs.ItemMudou(tx, item)
	return item, nil
}
