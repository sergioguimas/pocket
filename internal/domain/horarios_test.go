package domain

import (
	"testing"
	"time"
)

func TestParseHoraMinuto(t *testing.T) {
	casos := []struct {
		entrada   string
		erro      bool
		hora, min int
	}{
		{"07:30", false, 7, 30},
		{"00:00", false, 0, 0},
		{"23:59", false, 23, 59},
		{"24:00", true, 0, 0},
		{"12:60", true, 0, 0},
		{"7:30", true, 0, 0},
		{"-15m", true, 0, 0},
		{"", true, 0, 0},
	}
	for _, c := range casos {
		h, m, err := ParseHoraMinuto(c.entrada)
		if c.erro {
			if err == nil {
				t.Errorf("ParseHoraMinuto(%q): esperava erro", c.entrada)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseHoraMinuto(%q): erro inesperado: %v", c.entrada, err)
			continue
		}
		if h != c.hora || m != c.min {
			t.Errorf("ParseHoraMinuto(%q) = %d:%d, esperava %d:%d", c.entrada, h, m, c.hora, c.min)
		}
	}
}

func TestParseAntecedencia(t *testing.T) {
	casos := []struct {
		entrada string
		erro    bool
		want    time.Duration
	}{
		{"-15m", false, -15 * time.Minute},
		{"-2h", false, -2 * time.Hour},
		{"-1d", false, -24 * time.Hour},
		{"15m", true, 0},  // sem sinal
		{"+15m", true, 0}, // sinal errado
		{"-0m", true, 0},  // zero não é positivo
		{"-15s", true, 0}, // unidade inválida
		{"07:30", true, 0},
	}
	for _, c := range casos {
		got, err := ParseAntecedencia(c.entrada)
		if c.erro {
			if err == nil {
				t.Errorf("ParseAntecedencia(%q): esperava erro", c.entrada)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseAntecedencia(%q): erro inesperado: %v", c.entrada, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseAntecedencia(%q) = %v, esperava %v", c.entrada, got, c.want)
		}
	}
}

func TestValidarHorarios(t *testing.T) {
	data := Dia("2026-10-14")

	casos := []struct {
		nome string
		item Item
		erro bool
	}{
		{"hábito com HH:MM válido", Item{Tipo: TipoHabito, Horarios: []string{"07:30", "21:00"}}, false},
		{"hábito com antecedência é erro", Item{Tipo: TipoHabito, Horarios: []string{"-15m"}}, true},
		{"compromisso com hora e antecedências válidas", Item{Tipo: TipoCompromisso, Data: &data, Hora: "15:00", Horarios: []string{"-15m", "-1d"}}, false},
		{"compromisso com hora e HH:MM é erro", Item{Tipo: TipoCompromisso, Data: &data, Hora: "15:00", Horarios: []string{"07:30"}}, true},
		{"compromisso com data sem hora aceita HH:MM", Item{Tipo: TipoCompromisso, Data: &data, Horarios: []string{"09:00"}}, false},
		{"compromisso com data sem hora rejeita antecedência", Item{Tipo: TipoCompromisso, Data: &data, Horarios: []string{"-15m"}}, true},
		{"compromisso sem data sem horarios ok", Item{Tipo: TipoCompromisso}, false},
		{"compromisso sem data com horario é erro", Item{Tipo: TipoCompromisso, Horarios: []string{"09:00"}}, true},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			err := ValidarHorarios(c.item)
			if c.erro && err == nil {
				t.Errorf("esperava erro")
			}
			if !c.erro && err != nil {
				t.Errorf("erro inesperado: %v", err)
			}
		})
	}
}

func TestAgendasDoDiaHabito(t *testing.T) {
	loc := time.UTC
	dia := Dia("2026-09-26")
	item := Item{Tipo: TipoHabito, Horarios: []string{"07:30", "21:00"}}

	aberta := &Ocorrencia{Status: StatusAberta}
	got := AgendasDoDia(item, dia, aberta, loc)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, esperava 2", len(got))
	}
	want1 := time.Date(2026, 9, 26, 7, 30, 0, 0, loc)
	want2 := time.Date(2026, 9, 26, 21, 0, 0, 0, loc)
	if !got[0].Equal(want1) || !got[1].Equal(want2) {
		t.Errorf("got = %v, esperava [%v %v]", got, want1, want2)
	}

	// já cumprida: nenhum lembrete (decisão 7 — cumprir cancela o resto do dia).
	cumprida := &Ocorrencia{Status: StatusCumprida}
	if got := AgendasDoDia(item, dia, cumprida, loc); len(got) != 0 {
		t.Errorf("ocorrência cumprida não deveria gerar lembretes: %v", got)
	}

	// sem ocorrência cobrindo o dia: nenhum lembrete.
	if got := AgendasDoDia(item, dia, nil, loc); len(got) != 0 {
		t.Errorf("sem ocorrência não deveria gerar lembretes: %v", got)
	}

	// pulada: nenhum lembrete.
	pulada := &Ocorrencia{Status: StatusPulada}
	if got := AgendasDoDia(item, dia, pulada, loc); len(got) != 0 {
		t.Errorf("ocorrência pulada não deveria gerar lembretes: %v", got)
	}
}

func TestAgendasDoDiaCompromissoComHora(t *testing.T) {
	loc := time.UTC
	data := Dia("2026-10-14")
	item := Item{
		Tipo:     TipoCompromisso,
		Data:     &data,
		Hora:     "15:00",
		Horarios: []string{"-15m", "-1d"},
	}
	aberta := &Ocorrencia{Status: StatusAberta}

	// no próprio dia: só o lembrete da hora e o de -15min (ambos caem em 14/10).
	got14 := AgendasDoDia(item, data, aberta, loc)
	if len(got14) != 2 {
		t.Fatalf("14/10: len(got) = %d, esperava 2; got=%v", len(got14), got14)
	}
	wantHora := time.Date(2026, 10, 14, 15, 0, 0, 0, loc)
	want15min := time.Date(2026, 10, 14, 14, 45, 0, 0, loc)
	if !got14[0].Equal(wantHora) {
		t.Errorf("got14[0] = %v, esperava %v (lembrete na hora)", got14[0], wantHora)
	}
	if !got14[1].Equal(want15min) {
		t.Errorf("got14[1] = %v, esperava %v (-15m)", got14[1], want15min)
	}

	// um dia antes: o -1d aparece aqui, exatamente como o contrato descreve
	// (§3.5, decisão 7): "-1d de um compromisso em 14/10 15:00 aparece em
	// AgendasDoDia(13/10)".
	got13 := AgendasDoDia(item, Dia("2026-10-13"), aberta, loc)
	if len(got13) != 1 {
		t.Fatalf("13/10: len(got) = %d, esperava 1; got=%v", len(got13), got13)
	}
	want1d := time.Date(2026, 10, 13, 15, 0, 0, 0, loc)
	if !got13[0].Equal(want1d) {
		t.Errorf("got13[0] = %v, esperava %v (-1d)", got13[0], want1d)
	}

	// qualquer outro dia: nada.
	if got := AgendasDoDia(item, Dia("2026-10-12"), aberta, loc); len(got) != 0 {
		t.Errorf("12/10 não deveria ter lembrete: %v", got)
	}

	// concluído (ocorrência não aberta): nenhum lembrete, mesmo no dia certo.
	cumprida := &Ocorrencia{Status: StatusCumprida}
	if got := AgendasDoDia(item, data, cumprida, loc); len(got) != 0 {
		t.Errorf("compromisso concluído não deveria ter lembrete: %v", got)
	}
}

func TestAgendasDoDiaCompromissoSemHoraComData(t *testing.T) {
	loc := time.UTC
	data := Dia("2026-10-14")
	item := Item{Tipo: TipoCompromisso, Data: &data, Horarios: []string{"09:00"}}
	aberta := &Ocorrencia{Status: StatusAberta}

	got := AgendasDoDia(item, data, aberta, loc)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, esperava 1", len(got))
	}
	want := time.Date(2026, 10, 14, 9, 0, 0, 0, loc)
	if !got[0].Equal(want) {
		t.Errorf("got[0] = %v, esperava %v", got[0], want)
	}
}

func TestAgendasDoDiaCompromissoSemData(t *testing.T) {
	loc := time.UTC
	item := Item{Tipo: TipoCompromisso}
	// sem data, a ocorrência criada também não tem Inicio/Fim — mas mesmo
	// que alguém passe uma "aberta" artificial, sem Data não há lembrete.
	aberta := &Ocorrencia{Status: StatusAberta}
	if got := AgendasDoDia(item, Dia("2026-10-14"), aberta, loc); len(got) != 0 {
		t.Errorf("compromisso sem data não deveria ter lembrete: %v", got)
	}
}

func TestAgendasDoDiaFusoHorario(t *testing.T) {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("carregar fuso: %v", err)
	}
	item := Item{Tipo: TipoHabito, Horarios: []string{"07:30"}}
	aberta := &Ocorrencia{Status: StatusAberta}
	got := AgendasDoDia(item, Dia("2026-09-26"), aberta, loc)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, esperava 1", len(got))
	}
	if _, offset := got[0].Zone(); offset != -3*3600 {
		t.Errorf("offset = %d, esperava -3h (America/Sao_Paulo)", offset)
	}
	if got[0].Hour() != 7 || got[0].Minute() != 30 {
		t.Errorf("hora local = %02d:%02d, esperava 07:30", got[0].Hour(), got[0].Minute())
	}
}

func TestAgendasDoDiaIntervaloSoAPartirDoInicio(t *testing.T) {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	regra := Regra{Tipo: RegraIntervalo, Intervalo: 3}
	item := Item{Tipo: TipoHabito, Regra: &regra, Estado: EstadoAtivo, Horarios: []string{"09:00"}}
	inicio := Dia("2026-09-15")
	aberta := Ocorrencia{Inicio: &inicio, Meta: 1, Status: StatusAberta}
	casos := []struct {
		dia  Dia
		want int
	}{{"2026-09-14", 0}, {"2026-09-15", 1}, {"2026-09-17", 1}}
	for _, c := range casos {
		if got := AgendasDoDia(item, c.dia, &aberta, loc); len(got) != c.want {
			t.Errorf("dia %s: %d agendas, esperava %d", c.dia, len(got), c.want)
		}
	}
}
