// Captura rápida por voz e texto (CONTRATO-FASE15.md §3, §4). Como em
// push.go, a peça externa (aqui, dois: Transcritor e Interpretador) entra
// por um setter — SetCaptura — em vez de mais parâmetros em New (mesma
// DECISÃO Opus de sempre: não quebrar os testes das etapas anteriores).
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"pocket/internal/captura"
	"pocket/internal/domain"
	"pocket/internal/store"
)

// SetCaptura injeta o Transcritor/Interpretador e o nome do modelo de
// interpretação (gravado na coluna captura.modelo — o modelo de
// transcrição não entra no log/schema, só o de interpretação, que é quem
// decide a "acao"). main.go só chama isto quando GROQ_API_KEY está
// preenchida (§7); sem chamar, CapturaAtiva() fica false.
func (a *App) SetCaptura(t captura.Transcritor, i captura.Interpretador, modelo string) {
	a.capturaTranscritor = t
	a.capturaInterpretador = i
	a.capturaModelo = modelo
}

// CapturaAtiva atende GET /api/captura (§3): false esconde a barra no
// front. Basta o Interpretador — o caminho de texto (sem áudio) não precisa
// de Transcritor.
func (a *App) CapturaAtiva() bool {
	return a.capturaInterpretador != nil
}

// --- erros tipados (§3: 503/429/502 — mesmo vocabulário de ErrValidacao/
// ErrNaoEncontrado/ErrConflito, o httpapi mapeia por tipo). ---

// ErrCapturaDesligada é POST/GET /api/captura sem GROQ_API_KEY (503).
type ErrCapturaDesligada struct{ Msg string }

func (e ErrCapturaDesligada) Error() string { return e.Msg }

// ErrCapturaLimite é o 429 do Groq depois da tentativa extra (§3: "limite
// diário do Groq atingido — use o formulário").
//
// Texto (e em ErrCapturaProvedor) é a transcrição já feita quando a falha
// foi só na interpretação — o front a mostra e oferece "Salvar como
// tarefa", para o recado falado não se perder (V3).
type ErrCapturaLimite struct{ Msg, Texto string }

func (e ErrCapturaLimite) Error() string { return e.Msg }

// ErrCapturaProvedor é qualquer outra falha do Groq depois das tentativas
// (§3: "não consegui entender agora — tente de novo ou use o formulário").
type ErrCapturaProvedor struct{ Msg, Texto string }

func (e ErrCapturaProvedor) Error() string { return e.Msg }

const (
	msgCapturaDesligada = "captura desligada"
	msgCapturaLimite    = "limite do Groq atingido — espere um minuto ou use o formulário"
	msgCapturaProvedor  = "não consegui entender agora — tente de novo ou use o formulário"
)

// mapCapturaErr traduz um erro de internal/captura (ErrLimite/ErrProvedor,
// ou qualquer outro — rede, timeout) para o vocabulário do app, sempre com
// a mensagem exata do contrato (o erro interno real só vai para o slog, via
// falhaCaptura).
func mapCapturaErr(err error, texto string) error {
	var limite captura.ErrLimite
	if errors.As(err, &limite) {
		return ErrCapturaLimite{Msg: msgCapturaLimite, Texto: texto}
	}
	return ErrCapturaProvedor{Msg: msgCapturaProvedor, Texto: texto}
}

// CapturaEntrada é a entrada de POST /api/captura (§3): ou Audio+Mime (voz,
// o app transcreve e interpreta), ou Texto (só interpreta). httpapi decide
// qual dos dois veio a partir do Content-Type do corpo.
type CapturaEntrada struct {
	Audio []byte
	Mime  string
	Texto string
}

// CapturaResultado é a resposta de POST /api/captura já com o id da linha
// gravada — httpapi funde isto com captura.Resposta (captura_id/texto +
// acao/item/registro/compras/resto/avisos, §3).
type CapturaResultado struct {
	ID       int64
	Texto    string
	Resposta captura.Resposta
}

// Capturar é o caso de uso de POST /api/captura (§3, §4): transcreve (se
// veio áudio), monta o contexto do banco, interpreta, normaliza e persiste
// uma linha `captura` — inclusive quando o provider falha (erro preenchido,
// resposta vazia). CapturaAtiva() já deveria ter sido checada por quem
// chama (httpapi devolve 503 antes disso), mas Capturar confere de novo por
// segurança (chamado direto nos testes de app).
func (a *App) Capturar(ctx context.Context, in CapturaEntrada) (CapturaResultado, error) {
	if !a.CapturaAtiva() {
		return CapturaResultado{}, ErrCapturaDesligada{Msg: msgCapturaDesligada}
	}

	inicio := a.agora()
	origem := domain.OrigemCapturaTexto
	texto := in.Texto
	var latenciaTranscricaoMs int64

	if len(in.Audio) > 0 {
		origem = domain.OrigemCapturaVoz
		t0 := a.agora()
		transcrito, err := a.capturaTranscritor.Transcrever(ctx, in.Audio, in.Mime)
		latenciaTranscricaoMs = a.agora().Sub(t0).Milliseconds()
		if err != nil {
			return a.falhaCaptura(ctx, origem, "", inicio, latenciaTranscricaoMs, 0, err)
		}
		texto = strings.TrimSpace(transcrito)
		if texto == "" {
			resp := captura.Resposta{Acao: captura.AcaoDesconhecido, Avisos: []string{"não ouvi nada"}}
			slog.Info("captura", "origem", origem, "modelo", a.capturaModelo, "acao", resp.Acao,
				"latencia_transcricao_ms", latenciaTranscricaoMs, "latencia_interpretacao_ms", 0)
			return a.persistirCaptura(ctx, origem, "", resp, inicio)
		}
	}

	contexto, err := a.montarContextoCaptura(ctx)
	if err != nil {
		return CapturaResultado{}, err
	}

	t1 := a.agora()
	modelJSON, uso, err := a.capturaInterpretador.Interpretar(ctx, contexto, texto)
	latenciaInterpretacaoMs := a.agora().Sub(t1).Milliseconds()
	if err != nil {
		return a.falhaCaptura(ctx, origem, texto, inicio, latenciaTranscricaoMs, latenciaInterpretacaoMs, err)
	}

	hoje := a.hoje()
	resp := captura.Normalizar(contexto, modelJSON, hoje, a.agora(), a.loc)

	// DECISÃO (§4: "Log: latência de cada etapa, modelo, tokens, acao.
	// Nunca o texto nem o áudio no log") — texto/in.Audio nunca aparecem
	// nestes campos, só metadados.
	slog.Info("captura", "origem", origem, "modelo", a.capturaModelo, "acao", resp.Acao,
		"latencia_transcricao_ms", latenciaTranscricaoMs, "latencia_interpretacao_ms", latenciaInterpretacaoMs,
		"latencia_total_ms", a.agora().Sub(inicio).Milliseconds(),
		"prompt_tokens", uso.PromptTokens, "completion_tokens", uso.CompletionTokens)

	return a.persistirCaptura(ctx, origem, texto, resp, inicio)
}

// falhaCaptura persiste a linha de erro (§4: "app... persiste uma linha
// captura... também em erros do provider") e devolve o erro já traduzido
// para o vocabulário do app (mapCapturaErr) — o erro real (rede, status
// HTTP) vai só para o slog, nunca ao cliente.
func (a *App) falhaCaptura(ctx context.Context, origem, texto string, inicio time.Time, latenciaTranscricaoMs, latenciaInterpretacaoMs int64, err error) (CapturaResultado, error) {
	latenciaTotal := a.agora().Sub(inicio)
	slog.Error("captura falhou", "origem", origem, "modelo", a.capturaModelo,
		"latencia_transcricao_ms", latenciaTranscricaoMs, "latencia_interpretacao_ms", latenciaInterpretacaoMs,
		"latencia_total_ms", latenciaTotal.Milliseconds(), "erro", err)

	if _, insErr := a.st.CapturaInserir(ctx, domain.Captura{
		Em: inicio, Origem: origem, Texto: texto, Erro: err.Error(),
		Modelo: a.capturaModelo, LatenciaMs: int(latenciaTotal.Milliseconds()),
	}); insErr != nil {
		slog.Error("gravar captura com erro falhou", "erro", insErr)
	}

	return CapturaResultado{}, mapCapturaErr(err, texto)
}

// persistirCaptura grava a linha de sucesso (resposta preenchida, erro
// vazio) e devolve o CapturaResultado com o id gerado.
func (a *App) persistirCaptura(ctx context.Context, origem, texto string, resp captura.Resposta, inicio time.Time) (CapturaResultado, error) {
	latencia := a.agora().Sub(inicio)
	respJSON, err := json.Marshal(resp)
	if err != nil {
		return CapturaResultado{}, fmt.Errorf("serializar resposta da captura: %w", err)
	}

	id, err := a.st.CapturaInserir(ctx, domain.Captura{
		Em: inicio, Origem: origem, Texto: texto, Resposta: string(respJSON),
		Modelo: a.capturaModelo, LatenciaMs: int(latencia.Milliseconds()),
	})
	if err != nil {
		return CapturaResultado{}, err
	}
	return CapturaResultado{ID: id, Texto: texto, Resposta: resp}, nil
}

// montarContextoCaptura monta o contexto enviado ao LLM (§4): hábitos
// ativos, compromissos abertos (até 50 mais próximos por data; sem data
// vai por último), listas de compras não arquivadas. Nada de registros,
// notas ou histórico.
func (a *App) montarContextoCaptura(ctx context.Context) (captura.Contexto, error) {
	habitosAtivos, err := a.st.ItemListar(ctx, domain.TipoHabito, domain.EstadoAtivo)
	if err != nil {
		return captura.Contexto{}, err
	}
	compromissosAtivos, err := a.st.ItemListar(ctx, domain.TipoCompromisso, domain.EstadoAtivo)
	if err != nil {
		return captura.Contexto{}, err
	}
	listas, err := a.st.ListaListar(ctx, true)
	if err != nil {
		return captura.Contexto{}, err
	}

	habitos := make([]captura.ContextoHabito, 0, len(habitosAtivos))
	for _, h := range habitosAtivos {
		habitos = append(habitos, captura.ContextoHabito{ID: h.ID, Titulo: h.Titulo, Regra: h.Regra})
	}

	sort.SliceStable(compromissosAtivos, func(i, j int) bool {
		di, dj := compromissosAtivos[i].Data, compromissosAtivos[j].Data
		if di == nil {
			return false // sem data vai para o fim
		}
		if dj == nil {
			return true
		}
		return *di < *dj
	})
	if len(compromissosAtivos) > 50 {
		compromissosAtivos = compromissosAtivos[:50]
	}
	compromissos := make([]captura.ContextoCompromisso, 0, len(compromissosAtivos))
	for _, c := range compromissosAtivos {
		data := ""
		if c.Data != nil {
			data = string(*c.Data)
		}
		compromissos = append(compromissos, captura.ContextoCompromisso{ID: c.ID, Titulo: c.Titulo, Data: data})
	}

	listasCtx := make([]captura.ContextoLista, 0, len(listas))
	for _, l := range listas {
		listasCtx = append(listasCtx, captura.ContextoLista{ID: l.ID, Nome: l.Nome})
	}

	return captura.Contexto{
		Agora: a.agora(), Loc: a.loc, Habitos: habitos, Compromissos: compromissos, Listas: listasCtx,
	}, nil
}

// RegistrarDesfecho grava o que o usuário fez com a proposta (§3 POST
// /api/captura/{id}/desfecho, V5) — sem efeito colateral além da linha em
// `captura`.
func (a *App) RegistrarDesfecho(ctx context.Context, id int64, desfecho string) error {
	switch desfecho {
	case domain.DesfechoConfirmada, domain.DesfechoAjustada, domain.DesfechoDescartada:
	default:
		return erroValidacao("desfecho inválido (%q)", desfecho)
	}
	if err := a.st.CapturaDefinirDesfecho(ctx, id, desfecho); err != nil {
		return mapearErroStore(err, fmt.Sprintf("captura %d não encontrada", id))
	}
	return nil
}

// capturaPruneMax é o teto de linhas de captura mantidas pelo job diário
// (§5: "a poda mantém as 500 mais recentes").
const capturaPruneMax = 500

// podarCapturasNaTx apaga as linhas de captura além das capturaPruneMax
// mais recentes — chamado pelo job diário (jobs.go) dentro da mesma Tx dos
// outros passos do dia. DECISÃO: precisa ser dentro da Tx (não um
// a.st.CapturaPodar solto depois) porque o Store abre só uma conexão de
// escrita (SetMaxOpenConns(1) — store.go) e RodarJobsDiarios já segura essa
// conexão via WithTx; chamar um método de Store (não de Tx) aqui dentro
// bloquearia esperando uma segunda conexão que nunca libera.
func (a *App) podarCapturasNaTx(ctx context.Context, tx *store.Tx) error {
	_, err := tx.CapturaPodar(ctx, capturaPruneMax)
	return err
}
