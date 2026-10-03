// Package domain — fase 3 (listas de compras, CONTRATO-FASE3.md §2). Mesmo
// espírito do resto do pacote: só cálculo puro a partir dos dados que o
// store passa, nenhum I/O.
package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Origens de item de compra (schema §4, coluna item_compra.origem).
const (
	OrigemCompraManual    = "manual"
	OrigemCompraFrequente = "frequente"
	OrigemCompraReceita   = "receita" // fase 4; nunca gravado antes disso
)

// Lista é uma lista de compras (§2.1). Lembrete fica em app/httpapi (regra
// diario/dias_semana + horarios, como a de hábito) — F3-1 só grava/valida o
// campo bruto; geração/envio são F3-2.
type Lista struct {
	ID        int64
	Nome      string
	Ordem     int
	Arquivada bool
	Lembrete  *ListaLembrete
	CriadoEm  string // RFC3339 local
}

// ListaLembrete é o lembrete de uma lista (§2.6): regra diario/dias_semana
// (vezes_periodo e intervalo não fazem sentido aqui — recusados na
// validação do app) + horários HH:MM.
type ListaLembrete struct {
	Regra    *Regra
	Horarios []string
}

// Secao é uma seção de uma lista (§2.2): nome + ordem, editável.
type Secao struct {
	ID      int64
	ListaID int64
	Nome    string
	Ordem   int
}

// ItemCompra é um item de uma lista (§2.3).
type ItemCompra struct {
	ID         int64
	ListaID    int64
	SecaoID    int64
	Nome       string
	NomeChave  string
	Quantidade string // texto livre ("2 kg", "3", "1 cx"); "" = sem quantidade
	Nota       string
	Comprado   bool
	CompradoEm string // "" quando não comprado
	Origem     string
	ReceitaID  *int64 // NULL até a fase 4
	CriadoEm   string // RFC3339 local
}

// CompraHistorico é a linha de histórico/frequentes de uma lista (§2.4),
// chave (lista_id, nome_chave).
type CompraHistorico struct {
	ListaID          int64
	NomeChave        string
	Nome             string // última grafia usada
	SecaoID          *int64
	UltimaQuantidade string
	Vezes            int
	UltimaEm         string // RFC3339 local
}

// NomeChave normaliza nome para comparação/dedupe (§2.5): minúsculas, sem
// acento, espaços colapsados, sem plural simples no fim ("s" depois de
// vogal — "tomates" -> "tomate"; não mexer em "arroz"; "pães" -> "paes").
// Não precisa ser perfeita (§2.5: erro vira no máximo um item duplicado ou
// um frequente a mais), então o algoritmo é heurística simples, sem
// dicionário de exceções.
//
// DECISÃO: a ordem importa para bater com o exemplo "pães" -> "paes" do
// contrato (plural irregular de "pão": -ão -> -ães/-ões/-ãos, não é "s
// depois de vogal simples" — mas termina em "s" precedido de "e", que a
// regra ingênua cortaria, dando "pae"). removerPluralSimples roda ANTES de
// remover acento e trata essas três terminações nasais como exceção
// explícita (fica como está — "não precisa ser perfeita", só não pode errar
// o exemplo do contrato); "tomates" não bate nenhuma delas e perde o "s"
// normalmente. Só depois disso o acento é removido.
func NomeChave(nome string) string {
	minusculo := strings.ToLower(nome)
	colapsado := colapsarEspacos(minusculo)
	semPlural := removerPluralSimples(colapsado)
	return removerAcentos(semPlural)
}

// pluraisIrregularesNasais são as terminações de plural irregular de
// palavras em "-ão" (§2.5, exemplo "pães"): tirar o "s" não daria o
// singular mesmo, então a regra simples de baixo (s após vogal) erraria.
var pluraisIrregularesNasais = []string{"ães", "ões", "ãos"}

// colapsarEspacos troca qualquer sequência de espaços (e apara as pontas)
// por um espaço só.
func colapsarEspacos(s string) string {
	campos := strings.FieldsFunc(s, unicode.IsSpace)
	return strings.Join(campos, " ")
}

// removerPluralSimples tira um "s" final quando o caractere anterior é uma
// das 5 vogais simples a/e/i/o/u (sem acento) — ver a DECISÃO em NomeChave
// sobre por que isto roda antes de remover acento, e sobre a exceção dos
// plurais nasais (-ães/-ões/-ãos).
func removerPluralSimples(s string) string {
	for _, sufixo := range pluraisIrregularesNasais {
		if strings.HasSuffix(s, sufixo) {
			return s
		}
	}
	r := []rune(s)
	n := len(r)
	if n < 2 || r[n-1] != 's' {
		return s
	}
	if ehVogalSimples(r[n-2]) {
		return string(r[:n-1])
	}
	return s
}

func ehVogalSimples(r rune) bool {
	switch r {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	}
	return false
}

// removerAcentos troca as vogais acentuadas comuns em português (e cedilha/
// til de "ñ") pelo equivalente sem acento — troca simples por tabela em vez
// de puxar unicode/norm como dependência nova (o alfabeto do dia a dia de
// lista de compras é pequeno o bastante).
func removerAcentos(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if troca, ok := tabelaAcentos[r]; ok {
			b.WriteRune(troca)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

var tabelaAcentos = map[rune]rune{
	'á': 'a', 'à': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ó': 'o', 'ò': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
	'ç': 'c', 'ñ': 'n',
}

// ScoreFrequente calcula o peso de recência-frequência de §2.4:
// vezes / (1 + dias desde ultima_em / 30). diasDesde é sempre >= 0 (quem
// chama já calculou a diferença de dias até "hoje" — este pacote não lê
// clock, ver o comentário do pacote).
func ScoreFrequente(vezes int, diasDesde int) float64 {
	if diasDesde < 0 {
		diasDesde = 0
	}
	return float64(vezes) / (1 + float64(diasDesde)/30)
}

// AgendasListaDoDia devolve os instantes (na location loc) em que o
// lembrete de uma lista deveria disparar no dia dado (§2.6): regra
// "diario" sempre bate; "dias_semana" só nos dias marcados. lembrete nil,
// sem regra, ou com um tipo que não devia existir aqui (o app já recusa
// vezes_periodo/intervalo na validação — isto é só defensivo) devolve nil.
func AgendasListaDoDia(lembrete *ListaLembrete, dia Dia, loc *time.Location) []time.Time {
	if lembrete == nil || lembrete.Regra == nil {
		return nil
	}
	switch lembrete.Regra.Tipo {
	case RegraDiario:
		// sempre bate.
	case RegraDiasSemana:
		if !contemInt(lembrete.Regra.Dias, DiaSemana(dia)) {
			return nil
		}
	default:
		return nil
	}

	t, err := dia.Time()
	if err != nil {
		return nil
	}
	ano, mes, diaDoMes := t.Date()

	var out []time.Time
	for _, s := range lembrete.Horarios {
		h, m, err := ParseHoraMinuto(s)
		if err != nil {
			continue // já validado na criação/edição do lembrete — defensivo aqui.
		}
		out = append(out, time.Date(ano, mes, diaDoMes, h, m, 0, 0, loc))
	}
	return out
}

// SecaoPendencia é a contagem de itens pendentes de uma seção — entrada de
// MontarResumoCompras.
type SecaoPendencia struct {
	Nome      string
	Pendentes int
}

// MontarResumoCompras monta o corpo do lembrete de lista (§2.6): "N itens:
// seção1 X, seção2 Y, seção3 Z, +W" — até 3 seções com mais pendentes
// (nome em minúsculas, ordenadas por pendentes desc, empate por nome para
// dar um resultado determinístico), o resto (se houver mais de 3 seções
// com pendentes) somado num "+N" final. "1 item" no singular quando o
// total é exatamente 1. Seções com 0 pendentes são ignoradas.
//
// DECISÃO (F3-2): a leitura literal do texto do contrato ("até 3 seções
// com mais itens, o resto somado") é "no máximo 3 nomes de seção, o
// restante — se houver uma 4ª seção ou mais — some num '+N'". O exemplo do
// contrato ("hortifrúti 3, laticínios 2, +2") tem só 2 nomes antes do
// "+2", mas o próprio pedido da etapa F3-2 pede testes para "3 seções" (o
// caso limite, sem "+N") e "mais de 3 seções" (com "+N") — o que só faz
// sentido se o limite for 3 nomes, não 2; foi isso que se implementou.
func MontarResumoCompras(secoes []SecaoPendencia) string {
	comItens := make([]SecaoPendencia, 0, len(secoes))
	total := 0
	for _, s := range secoes {
		if s.Pendentes <= 0 {
			continue
		}
		comItens = append(comItens, s)
		total += s.Pendentes
	}
	sort.SliceStable(comItens, func(i, j int) bool {
		if comItens[i].Pendentes != comItens[j].Pendentes {
			return comItens[i].Pendentes > comItens[j].Pendentes
		}
		return comItens[i].Nome < comItens[j].Nome
	})

	const maxSecoesNomeadas = 3
	top := comItens
	resto := 0
	if len(comItens) > maxSecoesNomeadas {
		top = comItens[:maxSecoesNomeadas]
		for _, s := range comItens[maxSecoesNomeadas:] {
			resto += s.Pendentes
		}
	}

	partes := make([]string, 0, len(top)+1)
	for _, s := range top {
		partes = append(partes, fmt.Sprintf("%s %d", strings.ToLower(s.Nome), s.Pendentes))
	}
	if resto > 0 {
		partes = append(partes, fmt.Sprintf("+%d", resto))
	}

	palavra := "itens"
	if total == 1 {
		palavra = "item"
	}
	return fmt.Sprintf("%d %s: %s", total, palavra, strings.Join(partes, ", "))
}
