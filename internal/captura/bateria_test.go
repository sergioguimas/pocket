//go:build groq

// Bateria de 30 frases pt-BR contra o Groq real (CONTRATO-FASE15.md §10
// etapa 3). Fora do `go test` normal (build tag "groq") — só roda no
// redmilab, onde GROQ_API_KEY mora:
//
//	go test -tags groq ./internal/captura/ -run TestBateria -v
//
// Sem GROQ_API_KEY no ambiente, o teste pula (skip), nunca falha — este PC
// de desenvolvimento não tem a chave. Contexto fixo: agora = 2026-09-28
// 10:00 America/Sao_Paulo (uma segunda-feira); hábitos Devocional diário,
// Academia 3×/semana, Ler, Regar plantas a cada 3 dias; compromisso
// Dentista 2026-10-14; listas Mercado (id 1), Farmácia (id 2). Ajuste o
// prompt (prompt.go) até a taxa de acerto ficar ≥ 90% e registre o
// resultado no README, como o §10 pede.
package captura

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"pocket/internal/domain"
)

// casoBateria é uma frase com o que se espera da Resposta normalizada.
type casoBateria struct {
	frase        string
	acaoEsperada string
	// checar é opcional: valida campos além da ação (título, item_id, etc).
	// r já é a Resposta normalizada; devolve "" se ok, ou a razão da falha.
	checar func(r Resposta) string
}

func contextoBateria(loc *time.Location) Contexto {
	return Contexto{
		Agora: time.Date(2026, 9, 28, 10, 0, 0, 0, loc),
		Loc:   loc,
		Habitos: []ContextoHabito{
			{ID: 1, Titulo: "Devocional"},
			{ID: 2, Titulo: "Academia"},
			{ID: 3, Titulo: "Ler"},
			{ID: 4, Titulo: "Regar plantas"},
		},
		Compromissos: []ContextoCompromisso{
			{ID: 10, Titulo: "Dentista", Data: "2026-10-14"},
		},
		Listas: []ContextoLista{
			{ID: 1, Nome: "Mercado"},
			{ID: 2, Nome: "Farmácia"},
		},
	}
}

func tituloIgual(esperado string) func(Resposta) string {
	return func(r Resposta) string {
		var titulo string
		switch {
		case r.Item != nil:
			titulo = r.Item.Titulo
		case r.Registro != nil:
			titulo = r.Registro.Titulo
		}
		if !strings.EqualFold(titulo, esperado) {
			return "título = " + titulo + ", want " + esperado
		}
		return ""
	}
}

func itemIDEsperado(id int64) func(Resposta) string {
	return func(r Resposta) string {
		if r.Registro == nil {
			return "registro nil"
		}
		if r.Registro.ItemID != id {
			return "item_id incorreto"
		}
		return ""
	}
}

func casosBateria() []casoBateria {
	return []casoBateria{
		{"dentista dia 14 às 15h, me lembra um dia antes", AcaoCriarItem, tituloIgual("Dentista")},
		{"ler 20 minutos todo dia às 21h", AcaoCriarItem, tituloIgual("Ler")},
		{"academia 3 vezes por semana", AcaoCriarItem, tituloIgual("Academia")},
		{"fiz o devocional, Romanos 8", AcaoRegistrar, itemIDEsperado(1)},
		{"ontem fui na academia", AcaoRegistrar, itemIDEsperado(2)},
		{"comprar leite, pão e 2 kg de arroz", AcaoCompras, nil},
		{"na farmácia: dipirona", AcaoCompras, nil},
		{"dentista amanhã e comprar leite", AcaoCriarItem, nil},
		{"hmm deixa eu ver", AcaoDesconhecido, nil},
		{"já li hoje", AcaoRegistrar, itemIDEsperado(3)},
		{"reguei as plantas", AcaoRegistrar, itemIDEsperado(4)},
		{"médico sexta-feira às 10h", AcaoCriarItem, nil},
		{"beber água todo dia", AcaoCriarItem, tituloIgual("Beber água")},
		{"pagar conta de luz dia 5", AcaoCriarItem, nil},
		{"reunião com o time amanhã às 9h", AcaoCriarItem, nil},
		{"já fui no dentista", AcaoRegistrar, itemIDEsperado(10)},
		{"comprar dipirona, band-aid e álcool em gel na farmácia", AcaoCompras, nil},
		{"estudar inglês 5 vezes por semana", AcaoCriarItem, nil},
		{"tomar remédio a cada 8 horas", AcaoDesconhecido, nil}, // frequência por hora, fora do escopo
		{"caminhar todo dia às 6h", AcaoCriarItem, nil},
		{"fiz academia hoje, treino de perna", AcaoRegistrar, itemIDEsperado(2)},
		{"regar as plantas semana que vem", AcaoCriarItem, nil},
		{"comprar 3 bananas e uma dúzia de ovos", AcaoCompras, nil},
		{"limpar a casa toda segunda e quinta", AcaoCriarItem, nil},
		{"terminei de ler o capítulo 5", AcaoRegistrar, itemIDEsperado(3)},
		{"dentista mês que vem", AcaoCriarItem, tituloIgual("Dentista")},
		{"41", AcaoDesconhecido, nil},
		{"comprar", AcaoDesconhecido, nil},
		{"academia hoje e comprar leite", AcaoRegistrar, nil},
		{"fazer a barba a cada 2 dias", AcaoCriarItem, nil},
	}
}

func TestBateriaFrases(t *testing.T) {
	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		t.Skip("GROQ_API_KEY vazia — pulando a bateria (só roda no redmilab)")
	}

	modelo := os.Getenv("POCKET_CAPTURA_MODELO")
	if modelo == "" {
		modelo = "openai/gpt-oss-120b"
	}
	interpretador := NovoGroqInterpretador(apiKey, modelo)

	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	ctxInfo := contextoBateria(loc)
	hoje := domain.Hoje(clockFixo(ctxInfo.Agora), loc)

	// O tier gratuito do gpt-oss-120b tem 8.000 tokens/minuto (medido em
	// 28/09) e cada chamada leva o prompt inteiro — sem pausa, a bateria
	// esbarra no 429 a partir da 4ª frase. BATERIA_PAUSA_S ajusta.
	pausa := 20 * time.Second
	if v, err := strconv.Atoi(os.Getenv("BATERIA_PAUSA_S")); err == nil {
		pausa = time.Duration(v) * time.Second
	}

	casos := casosBateria()
	acertos := 0
	for i, c := range casos {
		if i > 0 {
			time.Sleep(pausa)
		}
		modelJSON, uso, err := interpretador.Interpretar(context.Background(), ctxInfo, c.frase)
		if i == 0 && err == nil {
			t.Logf("tokens da 1ª chamada: prompt=%d, resposta=%d", uso.PromptTokens, uso.CompletionTokens)
		}
		if err != nil {
			t.Logf("FALHOU %-60s erro: %v", c.frase, err)
			continue
		}
		resp := Normalizar(ctxInfo, modelJSON, hoje, ctxInfo.Agora, loc)

		ok := resp.Acao == c.acaoEsperada
		motivo := ""
		if ok && c.checar != nil {
			if m := c.checar(resp); m != "" {
				ok = false
				motivo = m
			}
		}
		if !ok {
			if motivo == "" {
				motivo = "acao = " + resp.Acao + ", want " + c.acaoEsperada
			}
			t.Logf("FALHOU %-60s %s", c.frase, motivo)
			continue
		}
		acertos++
		t.Logf("ok     %-60s acao=%s", c.frase, resp.Acao)
	}

	taxa := float64(acertos) / float64(len(casos))
	t.Logf("Taxa de acerto: %d/%d = %.1f%%", acertos, len(casos), taxa*100)
	if taxa < 0.9 {
		t.Fatalf("taxa de acerto %.1f%% abaixo da meta de 90%%", taxa*100)
	}
}

// clockFixo satisfaz clock.Clock com um instante fixo, sem importar
// internal/clock aqui (mesmo truque de normalizar_test.go).
type clockFixo time.Time

func (c clockFixo) Now() time.Time { return time.Time(c) }
