package app

import (
	"reflect"
	"time"

	"pocket/internal/domain"
)

// contarRegistrosNoIntervalo conta, entre registros (de um único item), os
// que caem no dia local (loc) dentro de [oc.Inicio, oc.Fim] — Fim nulo
// (intervalo aberto) não tem limite superior. DECISÃO Opus (etapa 3/decisão
// 2): o filtro por dia é feito aqui em Go; o store só pré-filtra por
// item_id — volume pequeno, não vale a pena complicar o SQL.
func contarRegistrosNoIntervalo(registros []domain.Registro, oc domain.Ocorrencia, loc *time.Location) int {
	if oc.Inicio == nil {
		return 0
	}
	inicio := *oc.Inicio
	n := 0
	for _, r := range registros {
		dia := domain.DiaDeTime(r.Em.In(loc))
		if dia < inicio {
			continue
		}
		if oc.Fim != nil && dia > *oc.Fim {
			continue
		}
		n++
	}
	return n
}

// regraIgual compara duas regras já validadas (campos canônicos — Dias
// ordenado por ParseRegra) para decidir se uma edição realmente mudou a
// regra de frequência (EditarItem só refaz ocorrências quando muda).
func regraIgual(a, b domain.Regra) bool {
	return reflect.DeepEqual(a, b)
}

// diasEntreInclusive conta quantos dias existem entre de e ate, incluindo
// as duas pontas (usado pelo limite de 400 dias do calendário).
func diasEntreInclusive(de, ate domain.Dia) int {
	td, err1 := de.Time()
	ta, err2 := ate.Time()
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(ta.Sub(td).Hours()/24) + 1
}
