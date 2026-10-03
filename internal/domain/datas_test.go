package domain

import (
	"testing"
	"time"

	"pocket/internal/clock"
)

func TestParseDia(t *testing.T) {
	casos := []struct {
		entrada string
		erro    bool
	}{
		{"2026-09-26", false},
		{"2024-02-29", false}, // 2024 é bissexto
		{"2023-02-29", true},  // 2023 não é bissexto
		{"2026-04-31", true},  // abril tem 30 dias
		{"2026-13-01", true},
		{"26/09/2026", true},
		{"", true},
	}
	for _, c := range casos {
		d, err := ParseDia(c.entrada)
		if c.erro && err == nil {
			t.Errorf("ParseDia(%q): esperava erro, veio %q", c.entrada, d)
		}
		if !c.erro && err != nil {
			t.Errorf("ParseDia(%q): erro inesperado: %v", c.entrada, err)
		}
		if !c.erro && string(d) != c.entrada {
			t.Errorf("ParseDia(%q) = %q, esperava igual à entrada", c.entrada, d)
		}
	}
}

func TestHoje(t *testing.T) {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("carregar fuso: %v", err)
	}
	// 03:00 UTC de 2026-09-26 é 00:00 em São Paulo (UTC-3) — testa que Hoje
	// usa o fuso, não o dia UTC do relógio.
	c := clock.NewFake(time.Date(2026, 9, 26, 3, 0, 0, 0, time.UTC))
	if got := Hoje(c, loc); got != "2026-09-26" {
		t.Errorf("Hoje() = %q, esperava 2026-09-26", got)
	}

	c2 := clock.NewFake(time.Date(2026, 9, 26, 2, 0, 0, 0, time.UTC))
	if got := Hoje(c2, loc); got != "2026-09-25" {
		t.Errorf("Hoje() = %q, esperava 2026-09-25 (ainda 23h de ontem em SP)", got)
	}
}

func TestAddDias(t *testing.T) {
	casos := []struct {
		d    Dia
		n    int
		want Dia
	}{
		{"2026-09-26", 1, "2026-09-27"},
		{"2026-09-30", 1, "2026-10-01"}, // virada de mês (30 dias)
		{"2026-01-31", 1, "2026-02-01"}, // virada de mês (31 dias)
		{"2024-02-28", 1, "2024-02-29"}, // fev bissexto
		{"2024-02-29", 1, "2024-03-01"}, // fev bissexto -> março
		{"2023-02-28", 1, "2023-03-01"}, // fev não-bissexto
		{"2026-12-31", 1, "2027-01-01"}, // virada de ano
		{"2026-09-26", -30, "2026-08-27"},
	}
	for _, c := range casos {
		if got := c.d.AddDias(c.n); got != c.want {
			t.Errorf("%q.AddDias(%d) = %q, esperava %q", c.d, c.n, got, c.want)
		}
	}
}

func TestDiaSemana(t *testing.T) {
	casos := []struct {
		d    Dia
		want int
	}{
		{"2026-09-21", 1}, // segunda
		{"2026-09-22", 2},
		{"2026-09-23", 3},
		{"2026-09-24", 4},
		{"2026-09-25", 5},
		{"2026-09-26", 6}, // sábado
		{"2026-09-27", 7}, // domingo
	}
	for _, c := range casos {
		if got := DiaSemana(c.d); got != c.want {
			t.Errorf("DiaSemana(%q) = %d, esperava %d", c.d, got, c.want)
		}
	}
}

func TestInicioFimSemana(t *testing.T) {
	casos := []struct {
		d          Dia
		wantInicio Dia
		wantFim    Dia
	}{
		{"2026-09-21", "2026-09-21", "2026-09-27"}, // já é segunda
		{"2026-09-27", "2026-09-21", "2026-09-27"}, // domingo, mesma semana
		{"2026-09-23", "2026-09-21", "2026-09-27"}, // quarta
	}
	for _, c := range casos {
		if got := InicioSemana(c.d); got != c.wantInicio {
			t.Errorf("InicioSemana(%q) = %q, esperava %q", c.d, got, c.wantInicio)
		}
		if got := FimSemana(c.d); got != c.wantFim {
			t.Errorf("FimSemana(%q) = %q, esperava %q", c.d, got, c.wantFim)
		}
	}
}

func TestInicioFimMes(t *testing.T) {
	casos := []struct {
		d          Dia
		wantInicio Dia
		wantFim    Dia
	}{
		{"2026-09-15", "2026-09-01", "2026-09-30"}, // setembro, 30 dias
		{"2026-01-15", "2026-01-01", "2026-01-31"}, // janeiro, 31 dias
		{"2026-02-10", "2026-02-01", "2026-02-28"}, // fev não-bissexto
		{"2024-02-10", "2024-02-01", "2024-02-29"}, // fev bissexto
		{"2026-12-25", "2026-12-01", "2026-12-31"}, // dezembro
	}
	for _, c := range casos {
		if got := InicioMes(c.d); got != c.wantInicio {
			t.Errorf("InicioMes(%q) = %q, esperava %q", c.d, got, c.wantInicio)
		}
		if got := FimMes(c.d); got != c.wantFim {
			t.Errorf("FimMes(%q) = %q, esperava %q", c.d, got, c.wantFim)
		}
	}
}

func TestFormatParseInstante(t *testing.T) {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	orig := time.Date(2026, 9, 26, 14, 30, 0, 0, loc)
	s := FormatInstante(orig)
	parsed, err := ParseInstante(s)
	if err != nil {
		t.Fatalf("ParseInstante(%q): %v", s, err)
	}
	if !parsed.Equal(orig) {
		t.Errorf("round-trip mudou o instante: %v -> %q -> %v", orig, s, parsed)
	}
}
