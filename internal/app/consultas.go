package app

import (
	"context"
	"fmt"

	"pocket/internal/domain"
)

// Metricas são as métricas de hábito de §3.6, calculadas em Go a partir das
// ocorrências (sem tabela de cache). Taxa30d nil = "sem taxa" (decisão do
// build da etapa 2: hábito novo sem ocorrência fechada não mostra 0%).
// Intervalo só é preenchido para regra intervalo.
type Metricas struct {
	SequenciaAtual  int
	MelhorSequencia int
	Taxa30d         *float64
	Intervalo       *domain.EstadoIntervaloInfo
}

// ItemComMetricas é item + métricas (nil para compromisso — métricas são só
// de hábito, §3.6).
type ItemComMetricas struct {
	Item     domain.Item
	Metricas *Metricas
}

func (a *App) itemComMetricas(ctx context.Context, item domain.Item) (ItemComMetricas, error) {
	if item.Tipo != domain.TipoHabito {
		return ItemComMetricas{Item: item}, nil
	}
	ocorrencias, err := a.st.OcorrenciasPorItem(ctx, item.ID)
	if err != nil {
		return ItemComMetricas{}, err
	}
	m := calcularMetricas(item, ocorrencias, a.hoje())
	return ItemComMetricas{Item: item, Metricas: &m}, nil
}

func calcularMetricas(item domain.Item, ocorrencias []domain.Ocorrencia, hoje domain.Dia) Metricas {
	m := Metricas{
		SequenciaAtual:  domain.SequenciaAtual(ocorrencias),
		MelhorSequencia: domain.MelhorSequencia(ocorrencias),
	}
	if taxa, ok := domain.Taxa30d(ocorrencias, hoje); ok {
		m.Taxa30d = &taxa
	}
	if item.Regra != nil && item.Regra.Tipo == domain.RegraIntervalo {
		for _, oc := range ocorrencias {
			if oc.Status == domain.StatusAberta && oc.Fim == nil {
				info := domain.EstadoIntervalo(oc, hoje)
				m.Intervalo = &info
				break
			}
		}
	}
	return m
}

// ListarComMetricas atende GET /api/itens?tipo=&estado= (§5): itens
// filtrados, com métricas nos hábitos.
func (a *App) ListarComMetricas(ctx context.Context, tipo, estado string) ([]ItemComMetricas, error) {
	itens, err := a.st.ItemListar(ctx, tipo, estado)
	if err != nil {
		return nil, err
	}
	out := make([]ItemComMetricas, 0, len(itens))
	for _, it := range itens {
		cm, err := a.itemComMetricas(ctx, it)
		if err != nil {
			return nil, err
		}
		out = append(out, cm)
	}
	return out, nil
}

// ItemDetalhe é a resposta de GET /api/itens/{id} (decisão 9): item +
// métricas + últimos 30 registros (mais recentes primeiro) + a ocorrência
// aberta "corrente" (a que decide o que mostrar agora — nil se não houver).
type ItemDetalhe struct {
	ItemComMetricas
	Registros        []domain.Registro
	OcorrenciaAberta *domain.Ocorrencia
}

func (a *App) DetalheItem(ctx context.Context, id int64) (ItemDetalhe, error) {
	item, err := a.st.ItemBuscar(ctx, id)
	if err != nil {
		return ItemDetalhe{}, mapearErroStore(err, fmt.Sprintf("item %d não encontrado", id))
	}

	comMetricas, err := a.itemComMetricas(ctx, item)
	if err != nil {
		return ItemDetalhe{}, err
	}

	registros, err := a.st.RegistrosUltimos(ctx, id, 30)
	if err != nil {
		return ItemDetalhe{}, err
	}

	ocorrencias, err := a.st.OcorrenciasPorItem(ctx, id)
	if err != nil {
		return ItemDetalhe{}, err
	}
	aberta := ocorrenciaAbertaCorrente(item, ocorrencias, a.hoje())

	return ItemDetalhe{ItemComMetricas: comMetricas, Registros: registros, OcorrenciaAberta: aberta}, nil
}

// ocorrenciaAbertaCorrente acha a ocorrência "aberta" que representa o
// estado atual do item: para intervalo, a aberta de fim nulo; para os
// demais hábitos, a que cobre hoje; para compromisso, a única (se ainda
// aberta).
func ocorrenciaAbertaCorrente(item domain.Item, ocorrencias []domain.Ocorrencia, hoje domain.Dia) *domain.Ocorrencia {
	if item.Regra != nil && item.Regra.Tipo == domain.RegraIntervalo {
		for i := range ocorrencias {
			if ocorrencias[i].Status == domain.StatusAberta && ocorrencias[i].Fim == nil {
				return &ocorrencias[i]
			}
		}
		return nil
	}
	if oc, ok := domain.OcorrenciaDoDia(ocorrencias, hoje); ok && oc.Status == domain.StatusAberta {
		return &oc
	}
	if item.Tipo == domain.TipoCompromisso && len(ocorrencias) > 0 && ocorrencias[0].Status == domain.StatusAberta {
		return &ocorrencias[0]
	}
	return nil
}

// CalendarioDia é um dia do calendário de um item (decisão 9): status "" =
// nenhuma ocorrência cobria esse dia.
type CalendarioDia struct {
	Dia       domain.Dia
	Status    string
	Registros int
}

const calendarioMaxDias = 400

// Calendario atende GET /api/itens/{id}/calendario?de=&ate= (§5): default
// últimas 12 semanas até hoje; intervalo > 400 dias é 400.
func (a *App) Calendario(ctx context.Context, itemID int64, de, ate string) ([]CalendarioDia, error) {
	if _, err := a.st.ItemBuscar(ctx, itemID); err != nil {
		return nil, mapearErroStore(err, fmt.Sprintf("item %d não encontrado", itemID))
	}

	hoje := a.hoje()
	diaAte := hoje
	if ate != "" {
		d, err := domain.ParseDia(ate)
		if err != nil {
			return nil, ErrValidacao{Msg: err.Error()}
		}
		diaAte = d
	}
	diaDe := diaAte.AddDias(-12 * 7)
	if de != "" {
		d, err := domain.ParseDia(de)
		if err != nil {
			return nil, ErrValidacao{Msg: err.Error()}
		}
		diaDe = d
	}
	if diaDe > diaAte {
		return nil, erroValidacao("'de' não pode ser depois de 'ate'")
	}
	if diasEntreInclusive(diaDe, diaAte) > calendarioMaxDias {
		return nil, erroValidacao("intervalo maior que %d dias", calendarioMaxDias)
	}

	ocorrencias, err := a.st.OcorrenciasPorItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	registros, err := a.st.RegistrosPorItem(ctx, itemID)
	if err != nil {
		return nil, err
	}

	contagem := map[domain.Dia]int{}
	for _, r := range registros {
		contagem[domain.DiaDeTime(r.Em.In(a.loc))]++
	}

	var out []CalendarioDia
	for d := diaDe; d <= diaAte; d = d.AddDias(1) {
		status := ""
		if oc, ok := domain.OcorrenciaDoDia(ocorrencias, d); ok {
			status = oc.Status
		}
		out = append(out, CalendarioDia{Dia: d, Status: status, Registros: contagem[d]})
	}
	return out, nil
}
