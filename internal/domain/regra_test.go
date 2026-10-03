package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestParseRegraValidas(t *testing.T) {
	casos := []struct {
		nome string
		json string
		want Regra
	}{
		{"diario", `{"tipo":"diario"}`, Regra{Tipo: RegraDiario}},
		{"dias_semana", `{"tipo":"dias_semana","dias":[1,3,5]}`, Regra{Tipo: RegraDiasSemana, Dias: []int{1, 3, 5}}},
		{"dias_semana desordenado", `{"tipo":"dias_semana","dias":[5,1,3]}`, Regra{Tipo: RegraDiasSemana, Dias: []int{1, 3, 5}}},
		{"vezes_periodo semana", `{"tipo":"vezes_periodo","vezes":3,"periodo":"semana"}`, Regra{Tipo: RegraVezesPeriodo, Vezes: 3, Periodo: "semana"}},
		{"vezes_periodo mes", `{"tipo":"vezes_periodo","vezes":10,"periodo":"mes"}`, Regra{Tipo: RegraVezesPeriodo, Vezes: 10, Periodo: "mes"}},
		{"vezes_periodo semana no teto", `{"tipo":"vezes_periodo","vezes":7,"periodo":"semana"}`, Regra{Tipo: RegraVezesPeriodo, Vezes: 7, Periodo: "semana"}},
		{"vezes_periodo mes no teto", `{"tipo":"vezes_periodo","vezes":28,"periodo":"mes"}`, Regra{Tipo: RegraVezesPeriodo, Vezes: 28, Periodo: "mes"}},
		{"intervalo", `{"tipo":"intervalo","dias":3}`, Regra{Tipo: RegraIntervalo, Intervalo: 3}},
		{"intervalo no piso", `{"tipo":"intervalo","dias":2}`, Regra{Tipo: RegraIntervalo, Intervalo: 2}},
		{"intervalo no teto", `{"tipo":"intervalo","dias":90}`, Regra{Tipo: RegraIntervalo, Intervalo: 90}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got, err := ParseRegra([]byte(c.json))
			if err != nil {
				t.Fatalf("ParseRegra(%q): erro inesperado: %v", c.json, err)
			}
			if got.Tipo != c.want.Tipo || !intSliceIgual(got.Dias, c.want.Dias) ||
				got.Vezes != c.want.Vezes || got.Periodo != c.want.Periodo || got.Intervalo != c.want.Intervalo {
				t.Errorf("ParseRegra(%q) = %+v, esperava %+v", c.json, got, c.want)
			}
		})
	}
}

func intSliceIgual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestParseRegraInvalidas(t *testing.T) {
	casos := []struct {
		nome string
		json string
	}{
		{"tipo desconhecido", `{"tipo":"mensal"}`},
		{"json quebrado", `{"tipo":`},
		{"dias_semana vazio", `{"tipo":"dias_semana","dias":[]}`},
		{"dias_semana sem campo dias", `{"tipo":"dias_semana"}`},
		{"dias_semana dia 0", `{"tipo":"dias_semana","dias":[0,1]}`},
		{"dias_semana dia 8", `{"tipo":"dias_semana","dias":[8]}`},
		{"dias_semana duplicado", `{"tipo":"dias_semana","dias":[1,1]}`},
		{"vezes_periodo sem periodo", `{"tipo":"vezes_periodo","vezes":3}`},
		{"vezes_periodo periodo invalido", `{"tipo":"vezes_periodo","vezes":3,"periodo":"ano"}`},
		{"vezes_periodo vezes 0", `{"tipo":"vezes_periodo","vezes":0,"periodo":"semana"}`},
		{"vezes_periodo vezes 32", `{"tipo":"vezes_periodo","vezes":32,"periodo":"mes"}`},
		{"vezes_periodo 8 na semana", `{"tipo":"vezes_periodo","vezes":8,"periodo":"semana"}`},
		{"vezes_periodo 29 no mes", `{"tipo":"vezes_periodo","vezes":29,"periodo":"mes"}`},
		{"intervalo dias 1", `{"tipo":"intervalo","dias":1}`},
		{"intervalo dias 91", `{"tipo":"intervalo","dias":91}`},
		{"intervalo sem dias", `{"tipo":"intervalo"}`},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			_, err := ParseRegra([]byte(c.json))
			if err == nil {
				t.Fatalf("ParseRegra(%q): esperava erro", c.json)
			}
		})
	}
}

func TestRegraMarshalRoundTrip(t *testing.T) {
	casos := []string{
		`{"tipo":"diario"}`,
		`{"tipo":"dias_semana","dias":[1,3,5]}`,
		`{"tipo":"vezes_periodo","vezes":3,"periodo":"semana"}`,
		`{"tipo":"intervalo","dias":3}`,
	}
	for _, original := range casos {
		r, err := ParseRegra([]byte(original))
		if err != nil {
			t.Fatalf("ParseRegra(%q): %v", original, err)
		}
		out, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("Marshal(%+v): %v", r, err)
		}
		// Reparseia (em vez de comparar string) para não depender da ordem
		// de campos do encoding/json.
		r2, err := ParseRegra(out)
		if err != nil {
			t.Fatalf("ParseRegra(reserializado %q): %v", out, err)
		}
		if !reflect.DeepEqual(*r, *r2) {
			t.Errorf("round-trip mudou a regra: %+v -> %s -> %+v", r, out, r2)
		}
	}
}

func TestRegraUnmarshalJSONComoStruct(t *testing.T) {
	type wrapper struct {
		Regra *Regra `json:"regra"`
	}
	var w wrapper
	err := json.Unmarshal([]byte(`{"regra":{"tipo":"dias_semana","dias":[2,4]}}`), &w)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if w.Regra == nil || w.Regra.Tipo != RegraDiasSemana || !intSliceIgual(w.Regra.Dias, []int{2, 4}) {
		t.Errorf("w.Regra = %+v, esperava dias_semana [2 4]", w.Regra)
	}
}
