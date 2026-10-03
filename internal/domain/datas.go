package domain

import (
	"fmt"
	"time"

	"pocket/internal/clock"
)

const layoutDia = "2006-01-02"

// Dia é uma data local sem hora, sempre 'YYYY-MM-DD' (CONTRATO §4). Por ser
// largura fixa e zero-padded, comparação lexicográfica de string (<, >, ==)
// já é comparação cronológica — por isso Dia não precisa de métodos
// Antes/Depois: o próprio operador de string resolve.
type Dia string

// ParseDia valida s como 'YYYY-MM-DD' (data de calendário real: rejeita
// 31/04, 29/02 em ano não bissexto etc., como domain.ParseDate do Smaug).
func ParseDia(s string) (Dia, error) {
	t, err := time.Parse(layoutDia, s)
	if err != nil || t.Format(layoutDia) != s {
		return "", fmt.Errorf("data inválida (%q): use YYYY-MM-DD", s)
	}
	return Dia(s), nil
}

// DiaDeTime formata o dia local de t (no fuso que t já carrega) como Dia.
func DiaDeTime(t time.Time) Dia {
	return Dia(t.Format(layoutDia))
}

// Hoje devolve o dia local de agora (c.Now()) no fuso loc.
func Hoje(c clock.Clock, loc *time.Location) Dia {
	return DiaDeTime(c.Now().In(loc))
}

// Time converte d para time.Time à meia-noite (sem fuso — serve só para
// aritmética de calendário: extrair ano/mês/dia, comparar durações). Chamado
// só com Dia já validado (por ParseDia ou produzido por este pacote), então
// o erro é só defensivo.
func (d Dia) Time() (time.Time, error) {
	return time.Parse(layoutDia, string(d))
}

// AddDias soma n dias (n pode ser negativo) a d, atravessando mês/ano.
func (d Dia) AddDias(n int) Dia {
	t, err := d.Time()
	if err != nil {
		return d
	}
	return DiaDeTime(t.AddDate(0, 0, n))
}

// DiaSemana devolve o dia da semana de d no formato do contrato §3.2:
// 1=segunda .. 7=domingo (time.Weekday usa 0=domingo..6=sábado).
func DiaSemana(d Dia) int {
	t, err := d.Time()
	if err != nil {
		return 0
	}
	wd := int(t.Weekday())
	if wd == 0 {
		return 7
	}
	return wd
}

// InicioSemana devolve a segunda-feira da semana de d (decisão A1: semana
// começa na segunda).
func InicioSemana(d Dia) Dia {
	t, err := d.Time()
	if err != nil {
		return d
	}
	wd := int(t.Weekday()) // domingo=0..sábado=6
	offset := (wd + 6) % 7 // dias a voltar até a segunda anterior ou igual
	return DiaDeTime(t.AddDate(0, 0, -offset))
}

// FimSemana devolve o domingo da semana de d (o outro extremo de
// InicioSemana — precisa existir para os limites do período de
// vezes_periodo "semana", como InicioMes/FimMes existem para "mes").
func FimSemana(d Dia) Dia {
	return InicioSemana(d).AddDias(6)
}

// InicioMes devolve o 1º dia do mês de d.
func InicioMes(d Dia) Dia {
	t, err := d.Time()
	if err != nil {
		return d
	}
	return DiaDeTime(time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC))
}

// FimMes devolve o último dia do mês de d (truque do "dia 0" do mês
// seguinte — funciona também para dezembro, como em servicos/smaug).
func FimMes(d Dia) Dia {
	t, err := d.Time()
	if err != nil {
		return d
	}
	return DiaDeTime(time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC))
}

// diasEntre devolve quantos dias separam a de b (b - a; negativo se b < a).
func diasEntre(a, b Dia) int {
	ta, erra := a.Time()
	tb, errb := b.Time()
	if erra != nil || errb != nil {
		return 0
	}
	return int(tb.Sub(ta).Hours() / 24)
}

// DECISÃO (Opus, contrato §11/decisão 1): FormatInstante/ParseInstante são a
// única função de formato RFC3339 local de timestamps — o store (etapa 3)
// as usa para criado_em/atualizado_em/em, em vez de formatar time.Time à
// mão. Ficam aqui (mesmo arquivo dos formatadores de Dia) por serem a
// contraparte "com hora" das funções de data acima.

// FormatInstante formata t (já no fuso desejado pelo chamador) em RFC3339.
func FormatInstante(t time.Time) string {
	return t.Format(time.RFC3339)
}

// ParseInstante lê de volta um RFC3339 gravado por FormatInstante.
func ParseInstante(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}
