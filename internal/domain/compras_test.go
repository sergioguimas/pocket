package domain

import (
	"testing"
	"time"
)

func TestNomeChave(t *testing.T) {
	casos := []struct {
		in, want string
	}{
		{"Tomates", "tomate"},
		{"tomate", "tomate"},
		{"arroz", "arroz"},
		{"Arroz", "arroz"},
		{"pães", "paes"},
		{"Leite", "leite"},
		{"  Leite   Integral  ", "leite integral"},
		{"AÇÚCAR", "acucar"},
		{"maçã", "maca"},
		{"café", "cafe"},
		{"Ovos", "ovo"},
		{"batatas", "batata"},
		{"", ""},
		{"s", "s"},
	}
	for _, c := range casos {
		if got := NomeChave(c.in); got != c.want {
			t.Errorf("NomeChave(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestScoreFrequente(t *testing.T) {
	casos := []struct {
		nome      string
		vezes     int
		diasDesde int
		want      float64
	}{
		{"hoje, 1 vez", 1, 0, 1.0},
		{"hoje, 3 vezes", 3, 0, 3.0},
		{"30 dias, 1 vez", 1, 30, 0.5},
		{"60 dias, 2 vezes", 2, 60, 2.0 / 3.0},
		{"dias negativos tratados como 0", 2, -5, 2.0},
	}
	for _, c := range casos {
		got := ScoreFrequente(c.vezes, c.diasDesde)
		if diff := got - c.want; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("%s: ScoreFrequente(%d,%d) = %v, want %v", c.nome, c.vezes, c.diasDesde, got, c.want)
		}
	}
}

func TestScoreFrequenteOrdenaPorRecencia(t *testing.T) {
	// Mesmo número de vezes, mais recente pontua mais alto (§2.4: "peso de
	// recência").
	recente := ScoreFrequente(5, 1)
	antigo := ScoreFrequente(5, 90)
	if recente <= antigo {
		t.Fatalf("recente (%v) devia pontuar mais que antigo (%v)", recente, antigo)
	}
}

func TestScoreFrequenteMaisVezesPontuaMais(t *testing.T) {
	mais := ScoreFrequente(10, 30)
	menos := ScoreFrequente(2, 30)
	if mais <= menos {
		t.Fatalf("mais vezes (%v) devia pontuar mais que menos vezes (%v)", mais, menos)
	}
}

func TestAgendasListaDoDiaDiario(t *testing.T) {
	loc := timeLocalTeste(t)
	lem := &ListaLembrete{Regra: &Regra{Tipo: RegraDiario}, Horarios: []string{"09:00", "18:30"}}
	dia, _ := ParseDia("2026-09-28")

	agendas := AgendasListaDoDia(lem, dia, loc)
	if len(agendas) != 2 {
		t.Fatalf("agendas = %v, want 2", agendas)
	}
	if agendas[0].Hour() != 9 || agendas[0].Minute() != 0 {
		t.Fatalf("agendas[0] = %v, want 09:00", agendas[0])
	}
	if agendas[1].Hour() != 18 || agendas[1].Minute() != 30 {
		t.Fatalf("agendas[1] = %v, want 18:30", agendas[1])
	}
}

func TestAgendasListaDoDiaDiasSemana(t *testing.T) {
	loc := timeLocalTeste(t)
	// 2026-09-28 é segunda (1); regra só sábado (6).
	lem := &ListaLembrete{Regra: &Regra{Tipo: RegraDiasSemana, Dias: []int{6}}, Horarios: []string{"09:00"}}

	segunda, _ := ParseDia("2026-09-28")
	if agendas := AgendasListaDoDia(lem, segunda, loc); len(agendas) != 0 {
		t.Fatalf("agendas na segunda = %v, want nenhuma (regra só sábado)", agendas)
	}

	sabado, _ := ParseDia("2026-10-03")
	if agendas := AgendasListaDoDia(lem, sabado, loc); len(agendas) != 1 {
		t.Fatalf("agendas no sábado = %v, want 1", agendas)
	}
}

func TestAgendasListaDoDiaSemLembrete(t *testing.T) {
	loc := timeLocalTeste(t)
	dia, _ := ParseDia("2026-09-28")
	if agendas := AgendasListaDoDia(nil, dia, loc); agendas != nil {
		t.Fatalf("agendas com lembrete nil = %v, want nil", agendas)
	}
}

func timeLocalTeste(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	return loc
}

func TestMontarResumoComprasUmItem(t *testing.T) {
	got := MontarResumoCompras([]SecaoPendencia{{Nome: "Hortifrúti", Pendentes: 1}})
	want := "1 item: hortifrúti 1"
	if got != want {
		t.Fatalf("MontarResumoCompras = %q, want %q", got, want)
	}
}

func TestMontarResumoComprasTresSecoesSemAgregado(t *testing.T) {
	got := MontarResumoCompras([]SecaoPendencia{
		{Nome: "Hortifrúti", Pendentes: 3},
		{Nome: "Laticínios", Pendentes: 2},
		{Nome: "Bebidas", Pendentes: 1},
	})
	want := "6 itens: hortifrúti 3, laticínios 2, bebidas 1"
	if got != want {
		t.Fatalf("MontarResumoCompras = %q, want %q", got, want)
	}
}

func TestMontarResumoComprasMaisDeTresSecoes(t *testing.T) {
	got := MontarResumoCompras([]SecaoPendencia{
		{Nome: "Hortifrúti", Pendentes: 3},
		{Nome: "Laticínios", Pendentes: 2},
		{Nome: "Bebidas", Pendentes: 1},
		{Nome: "Higiene", Pendentes: 1},
	})
	want := "7 itens: hortifrúti 3, laticínios 2, bebidas 1, +1"
	if got != want {
		t.Fatalf("MontarResumoCompras = %q, want %q", got, want)
	}
}

func TestMontarResumoComprasIgnoraSecoesVazias(t *testing.T) {
	got := MontarResumoCompras([]SecaoPendencia{
		{Nome: "Hortifrúti", Pendentes: 2},
		{Nome: "Outros", Pendentes: 0},
	})
	want := "2 itens: hortifrúti 2"
	if got != want {
		t.Fatalf("MontarResumoCompras = %q, want %q", got, want)
	}
}
