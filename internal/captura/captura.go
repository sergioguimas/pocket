// Package captura interpreta uma frase falada ou digitada num rascunho de
// ação pronto para um toque de confirmação (CONTRATO-FASE15.md, "Fronteiras"
// §1: o LLM só propõe — a confirmação sempre passa pelas rotas de escrita
// que já existem). Duas peças externas via interface (Transcritor,
// Interpretador — implementadas contra o Groq, ver groq_transcricao.go e
// groq_interpretacao.go) e uma peça 100% Go/determinística (Normalizar, em
// normalizar.go) que não confia em nada que o modelo devolveu.
package captura

import (
	"context"
	"time"

	"pocket/internal/domain"
)

// Transcritor converte áudio em texto (§4: whisper-large-v3-turbo via
// Groq). mime é o Content-Type original do arquivo (audio/webm, audio/ogg,
// audio/mp4).
type Transcritor interface {
	Transcrever(ctx context.Context, audio []byte, mime string) (string, error)
}

// Interpretador propõe uma ação a partir do texto + contexto (§4: chat
// completions do Groq, response_format json_schema). Devolve o JSON cru que
// o modelo respondeu (ainda não confiado nem decodificado — isso é trabalho
// de Normalizar, que faz o próprio json.Unmarshal e trata até o caso de vir
// malformado) e o uso de tokens, para o log de latência/tokens (§4, "nunca
// o texto no log").
type Interpretador interface {
	Interpretar(ctx context.Context, contexto Contexto, texto string) (modelJSON []byte, uso Uso, err error)
}

// Uso é o consumo de tokens de uma chamada de interpretação — só para log
// (§4: "log: latência de cada etapa, modelo, tokens, acao").
type Uso struct {
	PromptTokens     int
	CompletionTokens int
}

// Contexto é o que o app monta a cada chamada a partir do banco (§4) e
// manda ao modelo — nada de registros, notas ou histórico.
type Contexto struct {
	Agora        time.Time // instante local, já no fuso do App
	Loc          *time.Location
	Habitos      []ContextoHabito      // hábitos ativos
	Compromissos []ContextoCompromisso // compromissos abertos, até 50 mais próximos
	Listas       []ContextoLista
}

// ContextoHabito é um hábito ativo citável em "registrar" (§3 regra 3) —
// Regra ajuda o modelo a não confundir "3 vezes por semana" com um hábito
// já existente diferente.
type ContextoHabito struct {
	ID     int64
	Titulo string
	Regra  *domain.Regra
}

// ContextoCompromisso é um compromisso aberto citável em "registrar".
type ContextoCompromisso struct {
	ID     int64
	Titulo string
	Data   string // "YYYY-MM-DD"; "" quando o compromisso não tem data
}

// ContextoLista é uma lista de compras não arquivada citável em "compras".
type ContextoLista struct {
	ID   int64
	Nome string
}

// --- erros tipados (§4: "1 nova tentativa em 429/5xx/rede, depois erro" —
// distinguir 429 final de qualquer outra falha, como app.ErrValidacao/
// ErrNaoEncontrado/ErrConflito já fazem para o resto da API). ---

// ErrLimite é um 429 do Groq depois da tentativa extra — "limite do
// Groq atingido" (§3, mapeado para HTTP 429 pelo app/httpapi).
type ErrLimite struct{ Msg string }

func (e ErrLimite) Error() string { return e.Msg }

// ErrProvedor é qualquer outra falha do Groq (rede, 5xx, corpo
// inesperado) depois das tentativas — "não consegui entender agora" (§3,
// mapeado para HTTP 502).
type ErrProvedor struct{ Msg string }

func (e ErrProvedor) Error() string { return e.Msg }
