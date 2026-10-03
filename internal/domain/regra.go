package domain

import (
	"encoding/json"
	"fmt"
	"sort"
)

// TipoRegra discrimina os quatro formatos de regra de frequência (§3.2).
type TipoRegra string

const (
	RegraDiario       TipoRegra = "diario"
	RegraDiasSemana   TipoRegra = "dias_semana"
	RegraVezesPeriodo TipoRegra = "vezes_periodo"
	RegraIntervalo    TipoRegra = "intervalo"
)

// Períodos válidos de vezes_periodo.
const (
	PeriodoSemana = "semana"
	PeriodoMes    = "mes"
)

// DECISÃO (Opus, decisão 9): teto de "vezes" por período — 7 para semana
// (dias da semana) e 28 para mês (teto seguro: o mês mais curto tem 28
// dias). Um mês de 31 dias aceitaria mais, mas usar o pior caso evita que a
// mesma regra vire inválida só por cair num fevereiro.
const (
	maxVezesSemana = 7
	maxVezesMes    = 28
)

// Regra é a regra de frequência de um hábito (§3.2), guardada como JSON
// validado em Go. Só os campos do Tipo escolhido são usados; os demais ficam
// zerados.
type Regra struct {
	Tipo      TipoRegra
	Dias      []int  // dias_semana: 1(segunda)..7(domingo), ordenados, sem duplicados
	Vezes     int    // vezes_periodo
	Periodo   string // vezes_periodo: "semana" | "mes"
	Intervalo int    // intervalo: dias entre ocorrências, 2..90
}

// regraJSON é a forma crua do JSON de §3.2. "dias" é RawMessage porque o
// mesmo nome de campo significa coisas diferentes conforme o tipo:
// dias_semana usa um array ([1,3,5]); intervalo usa um número (3).
type regraJSON struct {
	Tipo    string          `json:"tipo"`
	Dias    json.RawMessage `json:"dias,omitempty"`
	Vezes   int             `json:"vezes,omitempty"`
	Periodo string          `json:"periodo,omitempty"`
}

// ParseRegra valida e decodifica o JSON de uma regra (§3.2), com as
// mensagens de erro em português exigidas pelo contrato.
func ParseRegra(data []byte) (*Regra, error) {
	var raw regraJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("regra inválida: %w", err)
	}

	switch TipoRegra(raw.Tipo) {
	case RegraDiario:
		return &Regra{Tipo: RegraDiario}, nil

	case RegraDiasSemana:
		var dias []int
		if len(raw.Dias) > 0 {
			if err := json.Unmarshal(raw.Dias, &dias); err != nil {
				return nil, fmt.Errorf(`regra dias_semana: campo "dias" inválido (esperado array de inteiros): %w`, err)
			}
		}
		if err := validarDiasSemana(dias); err != nil {
			return nil, err
		}
		ordenados := append([]int{}, dias...)
		sort.Ints(ordenados)
		return &Regra{Tipo: RegraDiasSemana, Dias: ordenados}, nil

	case RegraVezesPeriodo:
		if raw.Periodo != PeriodoSemana && raw.Periodo != PeriodoMes {
			return nil, fmt.Errorf(`regra vezes_periodo: "periodo" deve ser "semana" ou "mes" (veio %q)`, raw.Periodo)
		}
		if raw.Vezes < 1 || raw.Vezes > 31 {
			return nil, fmt.Errorf(`regra vezes_periodo: "vezes" deve estar entre 1 e 31 (veio %d)`, raw.Vezes)
		}
		max := maxVezesSemana
		if raw.Periodo == PeriodoMes {
			max = maxVezesMes
		}
		if raw.Vezes > max {
			return nil, fmt.Errorf("regra vezes_periodo: vezes (%d) maior que os dias do período %q", raw.Vezes, raw.Periodo)
		}
		return &Regra{Tipo: RegraVezesPeriodo, Vezes: raw.Vezes, Periodo: raw.Periodo}, nil

	case RegraIntervalo:
		var intervalo int
		if len(raw.Dias) > 0 {
			if err := json.Unmarshal(raw.Dias, &intervalo); err != nil {
				return nil, fmt.Errorf(`regra intervalo: campo "dias" inválido (esperado número): %w`, err)
			}
		}
		if intervalo < 2 || intervalo > 90 {
			return nil, fmt.Errorf(`regra intervalo: "dias" deve estar entre 2 e 90 (veio %d)`, intervalo)
		}
		return &Regra{Tipo: RegraIntervalo, Intervalo: intervalo}, nil

	default:
		return nil, fmt.Errorf("regra com tipo desconhecido (%q); válidos: diario, dias_semana, vezes_periodo, intervalo", raw.Tipo)
	}
}

// validarDiasSemana aplica §3.2: sem dias é erro; dia fora de 1..7 é erro;
// dia duplicado é erro.
func validarDiasSemana(dias []int) error {
	if len(dias) == 0 {
		return fmt.Errorf("regra dias_semana: informe ao menos um dia (1=segunda..7=domingo)")
	}
	visto := make(map[int]bool, len(dias))
	for _, d := range dias {
		if d < 1 || d > 7 {
			return fmt.Errorf("regra dias_semana: dia %d fora do intervalo 1..7", d)
		}
		if visto[d] {
			return fmt.Errorf("regra dias_semana: dia %d duplicado", d)
		}
		visto[d] = true
	}
	return nil
}

// MarshalJSON serializa a Regra de volta ao formato canônico de §3.2 —
// só os campos do Tipo escolhido aparecem no JSON.
func (r Regra) MarshalJSON() ([]byte, error) {
	switch r.Tipo {
	case RegraDiario:
		return json.Marshal(struct {
			Tipo string `json:"tipo"`
		}{string(RegraDiario)})
	case RegraDiasSemana:
		return json.Marshal(struct {
			Tipo string `json:"tipo"`
			Dias []int  `json:"dias"`
		}{string(RegraDiasSemana), r.Dias})
	case RegraVezesPeriodo:
		return json.Marshal(struct {
			Tipo    string `json:"tipo"`
			Vezes   int    `json:"vezes"`
			Periodo string `json:"periodo"`
		}{string(RegraVezesPeriodo), r.Vezes, r.Periodo})
	case RegraIntervalo:
		return json.Marshal(struct {
			Tipo string `json:"tipo"`
			Dias int    `json:"dias"`
		}{string(RegraIntervalo), r.Intervalo})
	default:
		return nil, fmt.Errorf("regra com tipo desconhecido (%q)", r.Tipo)
	}
}

// UnmarshalJSON delega a ParseRegra, para *Regra funcionar como campo comum
// de encoding/json (o store da etapa 3 decodifica item.regra direto nela).
func (r *Regra) UnmarshalJSON(data []byte) error {
	parsed, err := ParseRegra(data)
	if err != nil {
		return err
	}
	*r = *parsed
	return nil
}

// contemInt diz se v está em s — usado por GerarOcorrencias para checar o
// dia da semana contra dias_semana.
func contemInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
