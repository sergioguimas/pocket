package captura

import (
	"encoding/json"
	"fmt"
	"strings"

	"pocket/internal/domain"
)

// diasSemanaPt nomeia o dia da semana em português para o cabeçalho de
// contexto (time.Weekday: 0=domingo..6=sábado).
var diasSemanaPt = [...]string{"domingo", "segunda-feira", "terça-feira", "quarta-feira", "quinta-feira", "sexta-feira", "sábado"}

// promptSistema é o prompt de sistema (pt-BR) — regras 1–7 de CONTRATO-
// FASE15.md §2, fixas; o contexto (data/hora, hábitos, compromissos,
// listas) é injetado por montarPromptSistema a cada chamada.
const promptSistema = `Você é o assistente de captura rápida do Pocket, um app pessoal de hábitos, compromissos e listas
de compras. Sua única tarefa: ler uma frase falada ou digitada em português e devolver um JSON
descrevendo a ação que ela pede. Você NUNCA grava nada — só propõe; um humano confirma depois.

Devolva sempre um objeto com os campos "acao", "item", "registro", "compras" e "resto", no formato
exato do schema. "item", "registro" e "compras" existem sempre como objetos — preencha só os campos
do que interessa para a "acao" escolhida e deixe os demais campos (de "item"/"registro"/"compras"
que não usar) como null ou array vazio.

Ações possíveis:
- "criar_item": a frase descreve um hábito (frequência) ou compromisso (evento) novo.
- "registrar": a frase diz que algo já foi feito ("fiz X", "já fui em Y", "ontem eu Z").
- "compras": a frase pede para comprar/adicionar itens a uma lista.
- "desconhecido": a frase não é nenhuma das anteriores, está vazia, ou você não entendeu nada.

Regras:
1. Datas relativas ("sexta", "dia 14", "amanhã", "semana que vem", "ontem") — resolva você mesmo
   usando a data de hoje do contexto abaixo e devolva sempre "YYYY-MM-DD" em "item.dia" ou
   "registro.dia". "Dia 14" já passado neste mês vira o dia 14 do mês seguinte. Compromisso com
   hora e sem dia ("me lembra às 15h de ligar pro João") é para hoje — ou amanhã, se a hora já
   passou. Recado sem data nem hora ("me lembra de ligar pro João") é compromisso sem data.
2. Frequência ("todo dia", "3 vezes por semana", "a cada 3 dias", "segunda e quarta") = hábito
   (regra_tipo preenchido, item.dia e item.hora ficam null). Data única ou nenhuma = compromisso.
3. Em "registrar", escolha "registro.item_id" SOMENTE entre os hábitos ativos e compromissos
   abertos listados no contexto abaixo — nunca invente um id. Sempre preencha também
   "registro.titulo" com o título falado (é o texto que vira o título de um item novo, caso o
   item_id escolhido não exista de verdade). Se nada no contexto corresponder à frase, deixe
   "registro.item_id" null.
4. Dia do registro ("ontem", "sábado", data explícita) vai em "registro.dia" como "YYYY-MM-DD";
   se a frase não menciona quando, deixe null (significa agora).
5. O que vier depois de "fiz X" (ex.: "Romanos 8", "3x12 supino 40kg") vira "registro.nota".
6. Em "compras", "compras.lista_nome" é o nome da lista citada (ex.: "na farmácia" → "Farmácia");
   se nenhuma lista foi citada, deixe null. Quantidade no formato livre falado ("2 kg", "3",
   "1 caixa") vai em "quantidade" de cada item; sem quantidade, null. No máximo 20 itens.
7. Uma ação por frase. Se a frase tiver mais de uma coisa clara ("dentista amanhã e comprar
   leite"), resolva só a primeira e devolva o restante da frase, literal, em "resto" (ex.: "comprar
   leite"). Sem resto, "resto" é null.

Exemplos (entrada → campos relevantes da saída):
- "dentista dia 14 às 15h, me lembra um dia antes" → acao="criar_item", item.tipo="compromisso",
  item.titulo="Dentista", item.dia="<dia 14 resolvido>", item.hora="15:00", item.horarios=["-1d"].
- "ler 20 minutos todo dia às 21h" → acao="criar_item", item.tipo="habito", item.titulo="Ler",
  item.regra_tipo="diario", item.horarios=["21:00"].
- "academia 3 vezes por semana" → acao="criar_item", item.tipo="habito", item.titulo="Academia",
  item.regra_tipo="vezes_periodo", item.regra_vezes=3, item.regra_periodo="semana".
- "fiz o devocional, Romanos 8" → acao="registrar", registro.titulo="Devocional",
  registro.item_id=<id do hábito Devocional se estiver no contexto>, registro.nota="Romanos 8".
- "ontem fui na academia" → acao="registrar", registro.titulo="Academia", registro.dia="<ontem>".
- "comprar leite, pão e 2 kg de arroz" → acao="compras", compras.itens=[{nome:"leite"},
  {nome:"pão"},{nome:"arroz",quantidade:"2 kg"}].
- "dentista amanhã e comprar leite" → acao="criar_item" (o dentista), resto="comprar leite".
- "hmm deixa eu ver" → acao="desconhecido".`

// montarPromptSistema monta o prompt completo (regras fixas + contexto
// desta chamada) — §4: "data e hora locais com dia da semana, fuso, hábitos
// ativos, compromissos abertos (até 50 mais próximos), listas de compras
// não arquivadas".
func montarPromptSistema(c Contexto) string {
	var b strings.Builder
	b.WriteString(promptSistema)
	b.WriteString("\n\nContexto desta chamada:\n")

	fmt.Fprintf(&b, "- Agora: %s (%s), fuso %s\n",
		c.Agora.Format("2006-01-02 15:04"), diasSemanaPt[int(c.Agora.Weekday())], c.Loc.String())

	b.WriteString("- Hábitos ativos: ")
	b.WriteString(listaContextoHabitoJSON(c.Habitos))
	b.WriteString("\n")

	b.WriteString("- Compromissos abertos: ")
	b.WriteString(listaContextoCompromissoJSON(c.Compromissos))
	b.WriteString("\n")

	b.WriteString("- Listas de compras: ")
	b.WriteString(listaContextoListaJSON(c.Listas))
	b.WriteString("\n")

	return b.String()
}

func listaContextoHabitoJSON(habitos []ContextoHabito) string {
	if len(habitos) == 0 {
		return "[] (nenhum)"
	}
	type linha struct {
		ID     int64         `json:"id"`
		Titulo string        `json:"titulo"`
		Regra  *domain.Regra `json:"regra,omitempty"`
	}
	out := make([]linha, 0, len(habitos))
	for _, h := range habitos {
		out = append(out, linha{ID: h.ID, Titulo: h.Titulo, Regra: h.Regra})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func listaContextoCompromissoJSON(compromissos []ContextoCompromisso) string {
	if len(compromissos) == 0 {
		return "[] (nenhum)"
	}
	type linha struct {
		ID     int64  `json:"id"`
		Titulo string `json:"titulo"`
		Data   string `json:"data,omitempty"`
	}
	out := make([]linha, 0, len(compromissos))
	for _, c := range compromissos {
		out = append(out, linha{ID: c.ID, Titulo: c.Titulo, Data: c.Data})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func listaContextoListaJSON(listas []ContextoLista) string {
	if len(listas) == 0 {
		return "[] (nenhuma)"
	}
	type linha struct {
		ID   int64  `json:"id"`
		Nome string `json:"nome"`
	}
	out := make([]linha, 0, len(listas))
	for _, l := range listas {
		out = append(out, linha{ID: l.ID, Nome: l.Nome})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(b)
}
