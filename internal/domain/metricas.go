package domain

import "sort"

// ordenarPorInicio devolve uma cópia de ocorrencias ordenada por Inicio
// crescente (Decisão 8: "métricas recebem as ocorrências em ordem
// qualquer; ordene por inicio"). Ocorrências sem Inicio (compromisso sem
// data) ordenam antes de todas — não deveriam aparecer aqui, métricas são
// só de hábito, mas evita pânico se acontecer.
func ordenarPorInicio(ocorrencias []Ocorrencia) []Ocorrencia {
	sorted := make([]Ocorrencia, len(ocorrencias))
	copy(sorted, ocorrencias)
	sort.SliceStable(sorted, func(i, j int) bool {
		return diaOuVazio(sorted[i].Inicio) < diaOuVazio(sorted[j].Inicio)
	})
	return sorted
}

func diaOuVazio(d *Dia) Dia {
	if d == nil {
		return ""
	}
	return *d
}

// SequenciaAtual conta as ocorrências "cumprida" consecutivas a partir da
// mais recente, andando para trás (§3.6): "pulada" é transparente (nem
// conta, nem quebra); a ocorrência aberta mais recente (hoje/período
// corrente ainda não cumprido) também não quebra — só é ignorada; a
// primeira "perdida" encontrada para a sequência.
func SequenciaAtual(ocorrencias []Ocorrencia) int {
	sorted := ordenarPorInicio(ocorrencias)
	count := 0
	for i := len(sorted) - 1; i >= 0; i-- {
		switch sorted[i].Status {
		case StatusCumprida:
			count++
		case StatusPulada, StatusAberta:
			// transparente: nem soma, nem quebra — segue olhando pra trás.
		case StatusPerdida:
			return count
		}
	}
	return count
}

// MelhorSequencia é a mesma regra de SequenciaAtual, mas sobre o histórico
// inteiro (§3.6): a maior sequência de "cumprida" já vista, com "pulada"
// (e a aberta corrente, se sobrar no fim) transparentes.
func MelhorSequencia(ocorrencias []Ocorrencia) int {
	sorted := ordenarPorInicio(ocorrencias)
	melhor, atual := 0, 0
	for _, oc := range sorted {
		switch oc.Status {
		case StatusCumprida:
			atual++
			if atual > melhor {
				melhor = atual
			}
		case StatusPerdida:
			atual = 0
		case StatusPulada, StatusAberta:
			// transparente.
		}
	}
	return melhor
}

// Taxa30d calcula cumpridas / (cumpridas + perdidas) entre as ocorrências
// fechadas (Fim) nos últimos 30 dias — Fim entre hoje-30 e hoje-1 (Decisão
// 8) — mais a ocorrência corrente (Fim >= hoje) se ela já estiver cumprida
// (§3.6: "mais a corrente se já cumprida"). ok é false quando o
// denominador (cumpridas+perdidas) é zero — não há taxa para mostrar ainda.
func Taxa30d(ocorrencias []Ocorrencia, hoje Dia) (taxa float64, ok bool) {
	desde := hoje.AddDias(-30)
	ate := hoje.AddDias(-1)

	cumpridas, perdidas := 0, 0
	for _, oc := range ocorrencias {
		if oc.Fim == nil {
			continue
		}
		fim := *oc.Fim

		fechadaNaJanela := fim >= desde && fim <= ate
		correnteCumprida := fim >= hoje && oc.Status == StatusCumprida

		switch {
		case correnteCumprida:
			cumpridas++
		case fechadaNaJanela && oc.Status == StatusCumprida:
			cumpridas++
		case fechadaNaJanela && oc.Status == StatusPerdida:
			perdidas++
		}
	}

	total := cumpridas + perdidas
	if total == 0 {
		return 0, false
	}
	return float64(cumpridas) / float64(total), true
}

// EstadoIntervaloInfo descreve a situação de um hábito intervalo em relação
// a hoje (§3.3 regra 5, decisão A2/decisão 4): intervalo não tem sequência
// — só "em dia" (faltam Dias dias) ou "atrasado" (Dias dias, 0 = vence
// hoje).
type EstadoIntervaloInfo struct {
	EmDia bool
	Dias  int // faltam (EmDia) ou atraso em dias (!EmDia); nunca negativo
}

// EstadoIntervalo calcula o EstadoIntervaloInfo da ocorrência aberta de um
// hábito intervalo. aberta.Inicio nil (não deveria acontecer para
// intervalo) devolve o zero-value.
func EstadoIntervalo(aberta Ocorrencia, hoje Dia) EstadoIntervaloInfo {
	if aberta.Inicio == nil {
		return EstadoIntervaloInfo{}
	}
	inicio := *aberta.Inicio
	if hoje < inicio {
		return EstadoIntervaloInfo{EmDia: true, Dias: diasEntre(hoje, inicio)}
	}
	return EstadoIntervaloInfo{EmDia: false, Dias: diasEntre(inicio, hoje)}
}
