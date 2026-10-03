// Package app junta domain (regras puras) e store (persistência) nos casos
// de uso do Pocket (CONTRATO.md §3, §4, §5 exceto push/notificações). Nenhum
// SQL aqui — só chamadas ao store — e nenhuma regra de negócio duplicada do
// domain: toda decisão de status/sequência/geração vem de lá.
package app

import (
	"errors"
	"fmt"
	"time"

	"pocket/internal/captura"
	"pocket/internal/clock"
	"pocket/internal/domain"
	"pocket/internal/push"
	"pocket/internal/store"
)

// App reúne as dependências dos casos de uso: o store, o relógio (nunca
// time.Now() direto — regra da casa) e o fuso local em que "hoje" e os
// timestamps são calculados (DECISÃO Opus, decisão 2).
type App struct {
	st    *store.Store
	clock clock.Clock
	loc   *time.Location
	obs   Observador

	// pusher/vapidPublic (etapa 4): nil/"" até SetPush ser chamado.
	pusher      push.Pusher
	vapidPublic string

	// captura (fase 1.5): nil/"" até SetCaptura ser chamado — CapturaAtiva()
	// é false nesse estado (GROQ_API_KEY vazia, main.go nunca chama
	// SetCaptura).
	capturaTranscritor   captura.Transcritor
	capturaInterpretador captura.Interpretador
	capturaModelo        string
}

// New cria o App. obs pode ser nil (usa o Observador nulo — a etapa 4
// injeta o de verdade).
func New(st *store.Store, c clock.Clock, loc *time.Location, obs Observador) *App {
	if obs == nil {
		obs = observadorNulo{}
	}
	return &App{st: st, clock: c, loc: loc, obs: obs}
}

// SetPush injeta o Pusher e a chave pública VAPID (etapa 4).
//
// DECISÃO (Opus, etapa 4): um setter em vez de mais parâmetros em New —
// New(st, clock, loc, obs) já é chamado assim em todos os testes das
// etapas 1–3 (app_test.go, httpapi_test.go, fluxo_test.go); mudar a
// assinatura quebraria todos eles por um recurso que a maioria não usa.
// main.go chama SetPush logo depois de New, antes de aceitar requisições.
func (a *App) SetPush(p push.Pusher, vapidPublic string) {
	a.pusher = p
	a.vapidPublic = vapidPublic
}

// agora devolve o instante atual já no fuso local do App.
func (a *App) agora() time.Time {
	return a.clock.Now().In(a.loc)
}

// hoje devolve o dia local de agora.
func (a *App) hoje() domain.Dia {
	return domain.DiaDeTime(a.agora())
}

// Agora expõe o instante atual (etapa 5: GET /health precisa comparar
// "agora" com tick_em/cert_expira_em sem importar time.Now() em httpapi —
// regra da casa: nenhum time.Now() fora de internal/clock e main.go).
func (a *App) Agora() time.Time { return a.agora() }

// Localizacao expõe o fuso local do App (etapa 5: formatar cert_expira_em
// em GET /health).
func (a *App) Localizacao() *time.Location { return a.loc }

// --- erros tipados (CONTRATO §5, DECISÃO Opus decisão 10) ---
//
// O httpapi mapeia estes três para 400/404/409; qualquer outro erro devolve
// 500 com {"erro":"erro interno"} e o erro real vai só para o log (slog) —
// nunca para o cliente.

// ErrValidacao é um erro de entrada do usuário (400): campo inválido, regra
// mal formada, valor fora do intervalo etc. Msg já vem em português, pronta
// para {"erro": Msg}.
type ErrValidacao struct{ Msg string }

func (e ErrValidacao) Error() string { return e.Msg }

// ErrNaoEncontrado é "essa linha não existe" (404).
type ErrNaoEncontrado struct{ Msg string }

func (e ErrNaoEncontrado) Error() string { return e.Msg }

// ErrConflito é uma operação que não pode acontecer nesse estado (409):
// transição de estado inválida, item arquivado, registrar no futuro, etc.
type ErrConflito struct{ Msg string }

func (e ErrConflito) Error() string { return e.Msg }

// erroValidacao/erroConflito são atalhos para construir os erros acima com
// fmt.Sprintf, usados o tempo todo pelos casos de uso.
func erroValidacao(formato string, args ...any) error {
	return ErrValidacao{Msg: fmt.Sprintf(formato, args...)}
}

func erroConflito(formato string, args ...any) error {
	return ErrConflito{Msg: fmt.Sprintf(formato, args...)}
}

// mapearErroStore traduz store.ErrNaoEncontrado (vocabulário do banco) para
// app.ErrNaoEncontrado (vocabulário da API) — os dois casos de uso que
// buscam por id antes de escrever passam o erro de leitura por aqui.
func mapearErroStore(err error, msg string) error {
	if err == nil {
		return nil
	}
	var naoEncontrado store.ErrNaoEncontrado
	if errors.As(err, &naoEncontrado) {
		return ErrNaoEncontrado{Msg: msg}
	}
	return err
}

// --- gancho para a etapa 4 (DECISÃO Opus, decisão 8) ---

// Observador é avisado, dentro da mesma transação, de toda mudança que o
// app faz em item/ocorrência — a etapa 4 implementa isto para gerar e
// cancelar notificações (§3.5). Default = observadorNulo (não faz nada).
//
// ATENÇÃO etapa 4: notificacao.ocorrencia_id não tem ON DELETE CASCADE —
// por isso OcorrenciaApagando é chamado ANTES do DELETE em toda apagada de
// ocorrência (EditarItem ao refazer abertas, DesfazerRegistro em intervalo),
// para o Observador poder apagar/desvincular as notificações daquela
// ocorrência enquanto a linha ainda existe.
type Observador interface {
	// ItemMudou é chamado depois de criar, editar ou mudar o estado de um
	// item (o item já gravado, dentro da tx).
	ItemMudou(tx *store.Tx, item domain.Item)
	// OcorrenciaMudou é chamado depois que uma ocorrência é criada ou tem
	// seu status/fim alterado (a ocorrência já gravada, com ID, dentro da tx).
	OcorrenciaMudou(tx *store.Tx, oc domain.Ocorrencia)
	// OcorrenciaApagando é chamado ANTES do DELETE de uma ocorrência (ver
	// ATENÇÃO acima) — oc ainda existe no banco neste momento.
	OcorrenciaApagando(tx *store.Tx, oc domain.Ocorrencia)
}

// observadorNulo é o Observador default (fase 1, antes da etapa 4 existir).
type observadorNulo struct{}

func (observadorNulo) ItemMudou(*store.Tx, domain.Item)                {}
func (observadorNulo) OcorrenciaMudou(*store.Tx, domain.Ocorrencia)    {}
func (observadorNulo) OcorrenciaApagando(*store.Tx, domain.Ocorrencia) {}
