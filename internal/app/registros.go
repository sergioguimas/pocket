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

// RegistroEntrada é o corpo de POST /api/itens/{id}/registros (§5): em e
// nota são opcionais; origem é decidida por quem chama o app (httpapi
// sempre manda domain.OrigemApp; a etapa 4 manda domain.OrigemNotificacao
// para a ação "Feito" da notificação).
type RegistroEntrada struct {
	Em     string // RFC3339, "YYYY-MM-DD" ou "" (agora) — decisão 3
	Nota   string
	Origem string
}

// Registrar aplica um toque em "feito" (§3.4, decisão 3): acha/decide a
// ocorrência pelo dia local de em, recalcula o status dela, e devolve o
// item já com métricas atualizadas.
func (a *App) Registrar(ctx context.Context, itemID int64, in RegistroEntrada) (ItemComMetricas, domain.Registro, error) {
	em, err := a.resolverEm(in.Em)
	if err != nil {
		return ItemComMetricas{}, domain.Registro{}, err
	}

	origem := in.Origem
	if origem == "" {
		origem = domain.OrigemApp
	}
	if origem != domain.OrigemApp && origem != domain.OrigemNotificacao {
		return ItemComMetricas{}, domain.Registro{}, erroValidacao("origem inválida (%q)", origem)
	}

	var itemFinal domain.Item
	var registro domain.Registro
	err = a.st.WithTx(ctx, func(tx *store.Tx) error {
		item, err := tx.ItemBuscar(ctx, itemID)
		if err != nil {
			return mapearErroStore(err, fmt.Sprintf("item %d não encontrado", itemID))
		}
		if item.Estado == domain.EstadoArquivado {
			return erroConflito("item arquivado")
		}

		r := domain.Registro{ItemID: itemID, Em: em, Nota: in.Nota, Origem: origem}
		id, err := tx.RegistroInserir(ctx, r)
		if err != nil {
			return err
		}
		r.ID = id
		registro = r

		novoItem, err := a.aplicarRegistro(ctx, tx, item, r)
		if err != nil {
			return err
		}
		itemFinal = novoItem
		return nil
	})
	if err != nil {
		return ItemComMetricas{}, domain.Registro{}, err
	}

	comMetricas, err := a.itemComMetricas(ctx, itemFinal)
	return comMetricas, registro, err
}

// resolverEm decide o instante do registro a partir da entrada da API
// (decisão 3): "" é agora; "YYYY-MM-DD" é meio-dia local daquele dia
// (marcação retroativa pelo calendário); qualquer outra coisa tenta RFC3339.
// Nunca no futuro.
func (a *App) resolverEm(s string) (time.Time, error) {
	agora := a.agora()
	if s == "" {
		return agora, nil
	}

	if dia, err := domain.ParseDia(s); err == nil {
		t, errT := dia.Time()
		if errT != nil {
			return time.Time{}, erroValidacao("data inválida (%q)", s)
		}
		y, m, d := t.Date()
		em := time.Date(y, m, d, 12, 0, 0, 0, a.loc)
		if em.After(agora) {
			return time.Time{}, erroValidacao("não dá para registrar no futuro")
		}
		return em, nil
	}

	t, err := domain.ParseInstante(s)
	if err != nil {
		return time.Time{}, erroValidacao(`"em" inválido (%q): use RFC3339 ou YYYY-MM-DD`, s)
	}
	t = t.In(a.loc)
	if t.After(agora) {
		return time.Time{}, erroValidacao("não dá para registrar no futuro")
	}
	return t, nil
}

// aplicarRegistro recalcula ocorrência(s)/estado do item depois que r já
// foi gravado, dentro da mesma transação (decisão 1).
func (a *App) aplicarRegistro(ctx context.Context, tx *store.Tx, item domain.Item, r domain.Registro) (domain.Item, error) {
	if item.Tipo == domain.TipoCompromisso {
		return a.registrarCompromissoNaTx(ctx, tx, item)
	}

	hoje := a.hoje()
	dia := domain.DiaDeTime(r.Em.In(a.loc))

	ocorrencias, err := tx.OcorrenciasPorItem(ctx, item.ID)
	if err != nil {
		return domain.Item{}, err
	}

	if item.Regra != nil && item.Regra.Tipo == domain.RegraIntervalo {
		fechada, proxima, ok := domain.RegistrarIntervalo(ocorrencias, dia, item.Regra.Intervalo)
		if ok {
			if err := tx.OcorrenciaAtualizar(ctx, fechada); err != nil {
				return domain.Item{}, err
			}
			a.obs.OcorrenciaMudou(tx, fechada)

			novaID, inserida, err := tx.OcorrenciaInserir(ctx, proxima)
			if err != nil {
				return domain.Item{}, err
			}
			if inserida {
				proxima.ID = novaID
				a.obs.OcorrenciaMudou(tx, proxima)
			}
		}
		// !ok: registro extra (dia já cobria uma cumprida mais recente) — só
		// o registro fica, nada de ocorrência muda (decisão da função
		// RegistrarIntervalo, domain/ocorrencias.go).
		return item, nil
	}

	oc, ok := domain.OcorrenciaDoDia(ocorrencias, dia)
	if !ok {
		return item, nil // registro extra: dia sem ocorrência (§3.3 regra 6)
	}

	registros, err := tx.RegistrosPorItem(ctx, item.ID)
	if err != nil {
		return domain.Item{}, err
	}
	n := contarRegistrosNoIntervalo(registros, oc, a.loc)
	novoStatus := domain.StatusApos(item, oc, n, hoje)
	if novoStatus != oc.Status {
		oc.Status = novoStatus
		if err := tx.OcorrenciaAtualizar(ctx, oc); err != nil {
			return domain.Item{}, err
		}
		a.obs.OcorrenciaMudou(tx, oc)
	}
	return item, nil
}

// registrarCompromissoNaTx aplica "compromisso cumprido" (decisão 3): a
// ocorrência (a única do item, se existir) vira cumprida e o item vira
// concluido. Compartilhada com MudarEstado(id, "concluido") (decisão 5).
func (a *App) registrarCompromissoNaTx(ctx context.Context, tx *store.Tx, item domain.Item) (domain.Item, error) {
	ocorrencias, err := tx.OcorrenciasPorItem(ctx, item.ID)
	if err != nil {
		return domain.Item{}, err
	}
	if len(ocorrencias) > 0 {
		oc := ocorrencias[0]
		if oc.Status != domain.StatusCumprida {
			oc.Status = domain.StatusCumprida
			if err := tx.OcorrenciaAtualizar(ctx, oc); err != nil {
				return domain.Item{}, err
			}
			a.obs.OcorrenciaMudou(tx, oc)
		}
	}

	item.Estado = domain.EstadoConcluido
	item.AtualizadoEm = a.agora()
	if err := tx.ItemAtualizar(ctx, item); err != nil {
		return domain.Item{}, err
	}
	a.obs.ItemMudou(tx, item)
	return item, nil
}

// concluirCompromissoNaTx é o caminho de MudarEstado(id, "concluido") para
// compromisso (decisão 5: "o mesmo que Registrar agora, origem app") — cria
// o registro e delega o resto a registrarCompromissoNaTx.
func (a *App) concluirCompromissoNaTx(ctx context.Context, tx *store.Tx, item domain.Item) (domain.Item, error) {
	r := domain.Registro{ItemID: item.ID, Em: a.agora(), Origem: domain.OrigemApp}
	if _, err := tx.RegistroInserir(ctx, r); err != nil {
		return domain.Item{}, err
	}
	return a.registrarCompromissoNaTx(ctx, tx, item)
}

// DesfazerRegistro apaga (hard delete, decisão A6) o registro e recalcula a
// ocorrência que ele cobria (decisão 4).
func (a *App) DesfazerRegistro(ctx context.Context, registroID int64) (ItemComMetricas, error) {
	var itemFinal domain.Item
	err := a.st.WithTx(ctx, func(tx *store.Tx) error {
		r, err := tx.RegistroBuscar(ctx, registroID)
		if err != nil {
			return mapearErroStore(err, fmt.Sprintf("registro %d não encontrado", registroID))
		}
		item, err := tx.ItemBuscar(ctx, r.ItemID)
		if err != nil {
			return err
		}

		if err := tx.RegistroApagar(ctx, registroID); err != nil {
			return err
		}

		novoItem, err := a.desfazerNaTx(ctx, tx, item, r)
		if err != nil {
			return err
		}
		itemFinal = novoItem
		return nil
	})
	if err != nil {
		return ItemComMetricas{}, err
	}
	return a.itemComMetricas(ctx, itemFinal)
}

func (a *App) desfazerNaTx(ctx context.Context, tx *store.Tx, item domain.Item, r domain.Registro) (domain.Item, error) {
	if item.Tipo == domain.TipoCompromisso {
		ocorrencias, err := tx.OcorrenciasPorItem(ctx, item.ID)
		if err != nil {
			return domain.Item{}, err
		}
		if len(ocorrencias) > 0 {
			oc := ocorrencias[0]
			oc.Status = domain.StatusAberta
			if err := tx.OcorrenciaAtualizar(ctx, oc); err != nil {
				return domain.Item{}, err
			}
			a.obs.OcorrenciaMudou(tx, oc)
		}
		item.Estado = domain.EstadoAtivo
		item.AtualizadoEm = a.agora()
		if err := tx.ItemAtualizar(ctx, item); err != nil {
			return domain.Item{}, err
		}
		a.obs.ItemMudou(tx, item)
		return item, nil
	}

	hoje := a.hoje()
	dia := domain.DiaDeTime(r.Em.In(a.loc))
	ocorrencias, err := tx.OcorrenciasPorItem(ctx, item.ID)
	if err != nil {
		return domain.Item{}, err
	}

	if item.Regra != nil && item.Regra.Tipo == domain.RegraIntervalo {
		if err := a.desfazerIntervaloNaTx(ctx, tx, item, ocorrencias, dia); err != nil {
			return domain.Item{}, err
		}
		return item, nil
	}

	oc, ok := domain.OcorrenciaDoDia(ocorrencias, dia)
	if !ok {
		return item, nil // era um registro extra: nada a recalcular
	}
	registros, err := tx.RegistrosPorItem(ctx, item.ID)
	if err != nil {
		return domain.Item{}, err
	}
	n := contarRegistrosNoIntervalo(registros, oc, a.loc)
	novoStatus := domain.StatusApos(item, oc, n, hoje)
	if novoStatus != oc.Status {
		oc.Status = novoStatus
		if err := tx.OcorrenciaAtualizar(ctx, oc); err != nil {
			return domain.Item{}, err
		}
		a.obs.OcorrenciaMudou(tx, oc)
	}
	return item, nil
}

// desfazerIntervaloNaTx implementa a decisão 4 para hábito intervalo: se o
// registro apagado fechou a ÚLTIMA cumprida (fim == dia do registro) e não
// sobrou outro registro naquele dia, a cumprida volta a aberta e a próxima
// aberta (sem registros) é apagada; se era de uma cumprida mais antiga, só o
// registro sai — o histórico fica como estava.
func (a *App) desfazerIntervaloNaTx(ctx context.Context, tx *store.Tx, item domain.Item, ocorrencias []domain.Ocorrencia, dia domain.Dia) error {
	var ultima *domain.Ocorrencia
	for i := range ocorrencias {
		oc := ocorrencias[i]
		if oc.Status != domain.StatusCumprida || oc.Fim == nil {
			continue
		}
		if ultima == nil || *oc.Fim > *ultima.Fim {
			ultima = &ocorrencias[i]
		}
	}
	if ultima == nil || *ultima.Fim != dia {
		return nil // registro de uma cumprida mais antiga (ou nenhuma cumprida) — só o registro sai
	}

	registros, err := tx.RegistrosPorItem(ctx, item.ID)
	if err != nil {
		return err
	}
	for _, r := range registros {
		if domain.DiaDeTime(r.Em.In(a.loc)) == dia {
			return nil // sobrou outro registro nesse dia: a cumprida fica fechada
		}
	}

	for _, oc := range ocorrencias {
		if oc.Status == domain.StatusAberta && oc.Fim == nil {
			// ATENÇÃO (decisão 8): observador ANTES do DELETE.
			a.obs.OcorrenciaApagando(tx, oc)
			if err := tx.OcorrenciaApagar(ctx, oc.ID); err != nil {
				return err
			}
			break
		}
	}

	reaberta := domain.DesfazerRegistroIntervalo(*ultima)
	if err := tx.OcorrenciaAtualizar(ctx, reaberta); err != nil {
		return err
	}
	a.obs.OcorrenciaMudou(tx, reaberta)
	return nil
}

// notaMax limita a anotação de um feito (texto curto: "Salmo 23", "3x12 supino 40 kg").
const notaMax = 500

// EditarNotaRegistro grava a anotação de um feito (PATCH /api/registros/{id}).
// Nota vazia apaga. Não mexe em ocorrência nem métrica — é só texto.
func (a *App) EditarNotaRegistro(ctx context.Context, id int64, nota string) (domain.Registro, error) {
	nota = strings.TrimSpace(nota)
	if utf8.RuneCountInString(nota) > notaMax {
		return domain.Registro{}, erroValidacao("nota longa demais (máximo %d caracteres)", notaMax)
	}
	var out domain.Registro
	err := a.st.WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.RegistroAtualizarNota(ctx, id, nota); err != nil {
			return mapearErroStore(err, fmt.Sprintf("registro %d não encontrado", id))
		}
		r, err := tx.RegistroBuscar(ctx, id)
		if err != nil {
			return err
		}
		out = r
		return nil
	})
	return out, err
}
