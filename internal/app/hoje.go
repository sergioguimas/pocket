package app

import (
	"context"
	"sort"

	"pocket/internal/domain"
)

// HojeResposta é o corpo de GET /api/hoje (§5, decisão 9; + Compras, §2.7
// da fase 3).
type HojeResposta struct {
	Dia          domain.Dia
	Habitos      []HojeHabito
	Compromissos []HojeCompromisso
	SemData      int
	Compras      []HojeLista
}

// HojeLista é uma linha de "compras" em Hoje (§2.7): só listas não
// arquivadas com pendentes > 0 ("Mercado · 7 itens").
type HojeLista struct {
	ListaID   int64
	Nome      string
	Pendentes int
}

// HojeHabito é um hábito com ocorrência cobrindo hoje (qualquer status — o
// front decide o visual, decisão 9). Feitos é quantos registros já caíram
// no intervalo da ocorrência (para o chip "2/3"). Intervalo só é
// preenchido para regra intervalo.
type HojeHabito struct {
	Item       domain.Item
	Ocorrencia domain.Ocorrencia
	Feitos     int
	Intervalo  *domain.EstadoIntervaloInfo
}

// HojeCompromisso é um compromisso ativo com data <= hoje (atrasado se <),
// ou um concluído hoje.
type HojeCompromisso struct {
	Item       domain.Item
	Ocorrencia domain.Ocorrencia
	Atrasado   bool
}

// Hoje monta a resposta de GET /api/hoje (decisão 9):
//   - hábitos ativos com ocorrência cobrindo hoje (qualquer status);
//   - intervalo só entra se a aberta já venceu (inicio <= hoje) — "em dia"
//     não aparece em Hoje, só quando fica devido;
//   - compromissos ativos com data == hoje ou < hoje (atrasado), mais os
//     concluídos hoje;
//   - sem_data = compromissos ativos sem data.
func (a *App) Hoje(ctx context.Context) (HojeResposta, error) {
	hoje := a.hoje()
	itens, err := a.st.ItemListar(ctx, "", "")
	if err != nil {
		return HojeResposta{}, err
	}

	resp := HojeResposta{Dia: hoje}
	for _, item := range itens {
		switch item.Tipo {
		case domain.TipoHabito:
			if item.Estado != domain.EstadoAtivo {
				continue
			}
			hh, ok, err := a.hojeHabito(ctx, item, hoje)
			if err != nil {
				return HojeResposta{}, err
			}
			if ok {
				resp.Habitos = append(resp.Habitos, hh)
			}

		case domain.TipoCompromisso:
			hc, semData, mostrar, err := a.hojeCompromisso(ctx, item, hoje)
			if err != nil {
				return HojeResposta{}, err
			}
			if semData {
				resp.SemData++
			}
			if mostrar {
				resp.Compromissos = append(resp.Compromissos, hc)
			}
		}
	}

	compras, err := a.hojeCompras(ctx)
	if err != nil {
		return HojeResposta{}, err
	}
	resp.Compras = compras

	ordenarHoje(resp.Habitos, resp.Compromissos)
	return resp, nil
}

// hojeCompras monta a lista de HojeLista (§2.7): só listas não arquivadas
// com pendentes > 0.
func (a *App) hojeCompras(ctx context.Context) ([]HojeLista, error) {
	listas, err := a.st.ListaListar(ctx, true)
	if err != nil {
		return nil, err
	}
	var out []HojeLista
	for _, l := range listas {
		n, err := a.st.PendentesPorLista(ctx, l.ID)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			out = append(out, HojeLista{ListaID: l.ID, Nome: l.Nome, Pendentes: n})
		}
	}
	return out, nil
}

func (a *App) hojeHabito(ctx context.Context, item domain.Item, hoje domain.Dia) (HojeHabito, bool, error) {
	ocorrencias, err := a.st.OcorrenciasPorItem(ctx, item.ID)
	if err != nil {
		return HojeHabito{}, false, err
	}

	if item.Regra != nil && item.Regra.Tipo == domain.RegraIntervalo {
		for _, oc := range ocorrencias {
			if oc.Status == domain.StatusAberta && oc.Fim == nil && oc.Inicio != nil && *oc.Inicio <= hoje {
				registros, err := a.st.RegistrosPorItem(ctx, item.ID)
				if err != nil {
					return HojeHabito{}, false, err
				}
				info := domain.EstadoIntervalo(oc, hoje)
				feitos := contarRegistrosNoIntervalo(registros, oc, a.loc)
				return HojeHabito{Item: item, Ocorrencia: oc, Feitos: feitos, Intervalo: &info}, true, nil
			}
		}
		return HojeHabito{}, false, nil
	}

	oc, ok := domain.OcorrenciaDoDia(ocorrencias, hoje)
	if !ok {
		return HojeHabito{}, false, nil
	}
	registros, err := a.st.RegistrosPorItem(ctx, item.ID)
	if err != nil {
		return HojeHabito{}, false, err
	}
	feitos := contarRegistrosNoIntervalo(registros, oc, a.loc)
	return HojeHabito{Item: item, Ocorrencia: oc, Feitos: feitos}, true, nil
}

// hojeCompromisso devolve (hc, semData, mostrar, err): semData incrementa o
// contador de "sem data"; mostrar diz se hc deve entrar na lista de
// compromissos de hoje.
func (a *App) hojeCompromisso(ctx context.Context, item domain.Item, hoje domain.Dia) (HojeCompromisso, bool, bool, error) {
	if item.Estado != domain.EstadoAtivo && item.Estado != domain.EstadoConcluido {
		return HojeCompromisso{}, false, false, nil
	}

	ocorrencias, err := a.st.OcorrenciasPorItem(ctx, item.ID)
	if err != nil {
		return HojeCompromisso{}, false, false, err
	}
	var oc domain.Ocorrencia
	if len(ocorrencias) > 0 {
		oc = ocorrencias[0]
	}

	if item.Estado == domain.EstadoConcluido {
		registros, err := a.st.RegistrosPorItem(ctx, item.ID)
		if err != nil {
			return HojeCompromisso{}, false, false, err
		}
		for _, r := range registros {
			if domain.DiaDeTime(r.Em.In(a.loc)) == hoje {
				return HojeCompromisso{Item: item, Ocorrencia: oc, Atrasado: false}, false, true, nil
			}
		}
		return HojeCompromisso{}, false, false, nil
	}

	// ativo
	if item.Data == nil {
		return HojeCompromisso{}, true, false, nil
	}
	if *item.Data > hoje {
		return HojeCompromisso{}, false, false, nil // no futuro: não aparece em Hoje ainda
	}
	atrasado := *item.Data < hoje
	return HojeCompromisso{Item: item, Ocorrencia: oc, Atrasado: atrasado}, false, true, nil
}

// ordenarHoje dá uma ordem estável e determinística (ordem do item, depois
// id) — o contrato não fixa ordem, mas uma lista instável é ruim para UI e
// para teste.
func ordenarHoje(habitos []HojeHabito, compromissos []HojeCompromisso) {
	sort.SliceStable(habitos, func(i, j int) bool {
		if habitos[i].Item.Ordem != habitos[j].Item.Ordem {
			return habitos[i].Item.Ordem < habitos[j].Item.Ordem
		}
		return habitos[i].Item.ID < habitos[j].Item.ID
	})
	sort.SliceStable(compromissos, func(i, j int) bool {
		if compromissos[i].Item.Ordem != compromissos[j].Item.Ordem {
			return compromissos[i].Item.Ordem < compromissos[j].Item.Ordem
		}
		return compromissos[i].Item.ID < compromissos[j].Item.ID
	})
}
