package domain

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// ParseHoraMinuto valida um horário "HH:MM" (24h, com zero à esquerda) e
// devolve hora e minuto.
func ParseHoraMinuto(s string) (hora, minuto int, err error) {
	if len(s) != 5 || s[2] != ':' {
		return 0, 0, fmt.Errorf(`horário inválido (%q): use "HH:MM"`, s)
	}
	h, errH := strconv.Atoi(s[0:2])
	m, errM := strconv.Atoi(s[3:5])
	if errH != nil || errM != nil {
		return 0, 0, fmt.Errorf(`horário inválido (%q): use "HH:MM"`, s)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("horário inválido (%q): hora 00-23, minuto 00-59", s)
	}
	return h, m, nil
}

var reAntecedencia = regexp.MustCompile(`^-([0-9]+)(m|h|d)$`)

// ParseAntecedencia valida uma antecedência de lembrete ("-15m", "-2h",
// "-1d" — sempre negativa, §3.5) e devolve a duração (negativa) a somar ao
// horário do compromisso.
func ParseAntecedencia(s string) (time.Duration, error) {
	m := reAntecedencia.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf(`antecedência inválida (%q): use "-15m", "-2h" ou "-1d"`, s)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("antecedência inválida (%q): valor deve ser positivo", s)
	}
	var unidade time.Duration
	switch m[2] {
	case "m":
		unidade = time.Minute
	case "h":
		unidade = time.Hour
	case "d":
		unidade = 24 * time.Hour
	}
	return -time.Duration(n) * unidade, nil
}

// ValidarHorarios valida item.Horarios conforme o tipo do item (§3.5,
// decisão 7): hábito e compromisso com data-mas-sem-hora aceitam só
// "HH:MM"; compromisso com hora aceita só antecedências ("-15m","-2h",
// "-1d" — sempre tem, além delas, um lembrete implícito na própria hora,
// gerado por AgendasDoDia, não guardado em Horarios); compromisso sem data
// não aceita horário nenhum (não há dia para ancorar o lembrete).
func ValidarHorarios(item Item) error {
	switch {
	case item.Tipo == TipoHabito:
		return validarListaHoraMinuto(item.Horarios)

	case item.Tipo == TipoCompromisso && item.Data == nil:
		if len(item.Horarios) > 0 {
			return fmt.Errorf("compromisso sem data não aceita horários de lembrete")
		}
		return nil

	case item.Tipo == TipoCompromisso && item.Hora != "":
		return validarListaAntecedencia(item.Horarios)

	default: // compromisso com data, sem hora
		return validarListaHoraMinuto(item.Horarios)
	}
}

func validarListaHoraMinuto(lista []string) error {
	for _, s := range lista {
		if _, _, err := ParseHoraMinuto(s); err != nil {
			return err
		}
	}
	return nil
}

func validarListaAntecedencia(lista []string) error {
	for _, s := range lista {
		if _, err := ParseAntecedencia(s); err != nil {
			return err
		}
	}
	return nil
}

// AgendasDoDia devolve os instantes (na location loc) em que item deveria
// notificar no dia dado (§3.5). O gate é sempre ocorrenciaAberta: a
// ocorrência que cobre dia precisa existir e estar "aberta" — cumprida,
// pulada, perdida ou nil (sem ocorrência ainda, ou compromisso sem data)
// significa nenhum lembrete (decisão 7: cumprir antes do horário cancela).
// Isso cobre uniformemente hábito (dispara em cada dia com ocorrência
// aberta não cumprida — em vezes_periodo, todo dia do período até bater a
// meta) e compromisso (para de disparar assim que concluído).
func AgendasDoDia(item Item, dia Dia, ocorrenciaAberta *Ocorrencia, loc *time.Location) []time.Time {
	if ocorrenciaAberta == nil || ocorrenciaAberta.Status != StatusAberta {
		return nil
	}
	// Hábito só lembra em dia coberto pela ocorrência — em intervalo, a
	// aberta tem fim nulo e só vence no inicio (antes disso está "em dia").
	// Compromisso não entra aqui: antecedência "-1d" cai antes do inicio.
	if item.Tipo == TipoHabito && ocorrenciaAberta.Inicio != nil && dia < *ocorrenciaAberta.Inicio {
		return nil
	}

	t, err := dia.Time()
	if err != nil {
		return nil
	}
	ano, mes, diaDoMes := t.Date()

	if item.Tipo == TipoCompromisso && item.Hora != "" {
		return agendasCompromissoComHora(item, ano, mes, diaDoMes, loc)
	}

	// Hábito, ou compromisso sem hora mas com data: cada Horarios["HH:MM"]
	// dispara no próprio dia.
	var out []time.Time
	for _, s := range item.Horarios {
		h, m, err := ParseHoraMinuto(s)
		if err != nil {
			continue // já validado na criação/edição — defensivo aqui
		}
		out = append(out, time.Date(ano, mes, diaDoMes, h, m, 0, 0, loc))
	}
	return out
}

// agendasCompromissoComHora monta o lembrete na hora do compromisso mais um
// por antecedência em item.Horarios, e devolve só os que caem no dia
// (ano, mes, diaDoMes) pedido — por isso "-1d" de um compromisso em 14/10
// 15:00 aparece só quando dia = 13/10.
func agendasCompromissoComHora(item Item, ano int, mes time.Month, diaDoMes int, loc *time.Location) []time.Time {
	if item.Data == nil {
		return nil
	}
	h, m, err := ParseHoraMinuto(item.Hora)
	if err != nil {
		return nil
	}
	dt, err := item.Data.Time()
	if err != nil {
		return nil
	}
	ay, am, ad := dt.Date()
	momento := time.Date(ay, am, ad, h, m, 0, 0, loc)

	var out []time.Time
	if noMesmoDia(momento, ano, mes, diaDoMes, loc) {
		out = append(out, momento)
	}
	for _, s := range item.Horarios {
		ant, err := ParseAntecedencia(s)
		if err != nil {
			continue
		}
		inst := momento.Add(ant)
		if noMesmoDia(inst, ano, mes, diaDoMes, loc) {
			out = append(out, inst)
		}
	}
	return out
}

func noMesmoDia(t time.Time, ano int, mes time.Month, dia int, loc *time.Location) bool {
	tl := t.In(loc)
	y, mo, d := tl.Date()
	return y == ano && mo == mes && d == dia
}
