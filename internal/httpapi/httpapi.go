// Package httpapi expõe a API HTTP do Pocket (CONTRATO §5, exceto
// /api/push/* e /api/notificacoes/*, que chegam na etapa 4). Os handlers só
// decodificam/codificam JSON e traduzem path/query — toda validação e regra
// de negócio fica em internal/app (DECISÃO Opus, decisão 10/12).
package httpapi

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"pocket/internal/app"
	"pocket/internal/domain"
	"pocket/internal/store"
)

// CertInfo devolve a validade do certificado TLS carregado (internal/ops.
// CertLoader implementa isto) — GET /health/etapa 5.
type CertInfo interface {
	Validade() (time.Time, error)
}

// TickInfo devolve o instante do último tick do scheduler (internal/
// scheduler.Scheduler implementa isto) — GET /health/etapa 5.
type TickInfo interface {
	UltimoTick() time.Time
}

// limiarSaudeCert e limiarSaudeTick são os limiares de §5/etapa 5 que
// derrubam "ok" em GET /health.
const (
	limiarSaudeCert = 7 * 24 * time.Hour
	limiarSaudeTick = 5 * time.Minute
)

// Server monta o mux e guarda as dependências dos handlers.
type Server struct {
	mux    *http.ServeMux
	st     *store.Store
	ap     *app.App
	versao string

	// cert/tick (etapa 5): nil até SetSaude ser chamado — GET /health então
	// só devolve {ok, versao} com tick_em/cert_expira_em nulos (mesmo
	// comportamento das etapas 1–4, usado pelos testes que não os injetam).
	cert CertInfo
	tick TickInfo

	// estatico (etapa 6): nil até SetEstatico ser chamado — sem o dist/ do
	// front embutido, qualquer rota fora de /api e /health devolve 404
	// (comportamento das etapas 1–5, usado pelos testes que não constroem o
	// front). O //go:embed all:dist fica em main.go (dist/ é vizinho dele,
	// não de internal/httpapi — go:embed não aceita "..").
	estatico fs.FS
}

// New cria o Server já com as rotas registradas. ap é o núcleo de casos de
// uso (etapa 3) — a partir desta etapa, httpapi.New sempre recebe um App
// (DECISÃO Opus, decisão 11).
func New(st *store.Store, ap *app.App, versao string) *Server {
	s := &Server{mux: http.NewServeMux(), st: st, ap: ap, versao: versao}
	s.routes()
	return s
}

// ServeHTTP torna Server um http.Handler — pode ir direto em http.Server.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)

	s.mux.HandleFunc("GET /api/hoje", s.handleHoje)

	s.mux.HandleFunc("GET /api/itens", s.handleListarItens)
	s.mux.HandleFunc("POST /api/itens", s.handleCriarItem)
	s.mux.HandleFunc("GET /api/itens/{id}", s.handleDetalheItem)
	s.mux.HandleFunc("PATCH /api/itens/{id}", s.handleEditarItem)
	s.mux.HandleFunc("POST /api/itens/{id}/estado", s.handleMudarEstado)
	s.mux.HandleFunc("GET /api/itens/{id}/calendario", s.handleCalendario)
	s.mux.HandleFunc("POST /api/itens/{id}/registros", s.handleCriarRegistro)

	s.mux.HandleFunc("DELETE /api/registros/{id}", s.handleDesfazerRegistro)
	s.mux.HandleFunc("PATCH /api/registros/{id}", s.handleEditarNotaRegistro)

	s.mux.HandleFunc("POST /api/ocorrencias/{id}/pular", s.handlePular)
	s.mux.HandleFunc("POST /api/ocorrencias/{id}/despular", s.handleDespular)

	// fase 3 (compras, CONTRATO-FASE3 §5 — exceto lembrete, F3-2).
	s.mux.HandleFunc("GET /api/listas", s.handleListarListas)
	s.mux.HandleFunc("POST /api/listas", s.handleCriarLista)
	s.mux.HandleFunc("PATCH /api/listas/{id}", s.handleEditarLista)
	s.mux.HandleFunc("POST /api/listas/{id}/estado", s.handleMudarEstadoLista)
	s.mux.HandleFunc("GET /api/listas/{id}", s.handleDetalheLista)
	s.mux.HandleFunc("POST /api/listas/{id}/itens", s.handleAdicionarItem)
	s.mux.HandleFunc("PATCH /api/compras/{id}", s.handleEditarItemCompra)
	s.mux.HandleFunc("POST /api/compras/{id}/comprado", s.handleRiscar)
	s.mux.HandleFunc("POST /api/compras/{id}/desmarcar", s.handleDesriscar)
	s.mux.HandleFunc("DELETE /api/compras/{id}", s.handleApagarItemCompra)
	s.mux.HandleFunc("POST /api/listas/{id}/limpar-comprados", s.handleLimparComprados)
	s.mux.HandleFunc("GET /api/listas/{id}/frequentes", s.handleFrequentes)
	s.mux.HandleFunc("POST /api/listas/{id}/secoes", s.handleCriarSecao)
	s.mux.HandleFunc("PATCH /api/secoes/{id}", s.handleEditarSecao)
	s.mux.HandleFunc("DELETE /api/secoes/{id}", s.handleApagarSecao)

	// fase 1.5 (captura por voz/texto, CONTRATO-FASE15.md §3).
	s.mux.HandleFunc("GET /api/captura", s.handleCapturaAtiva)
	s.mux.HandleFunc("POST /api/captura", s.handleCriarCaptura)
	s.mux.HandleFunc("POST /api/captura/{id}/desfecho", s.handleCapturaDesfecho)

	s.mux.HandleFunc("GET /api/push/vapid", s.handleVapid)
	s.mux.HandleFunc("POST /api/push/subscribe", s.handlePushSubscribe)
	s.mux.HandleFunc("POST /api/push/teste", s.handlePushTeste)
	s.mux.HandleFunc("POST /api/notificacoes/{id}/{acao}", s.handleAcaoNotificacao)

	// SPA estática (etapa 6): "/" é o padrão de menor prioridade do
	// ServeMux — nunca sombreia as rotas de /api e /health acima. Sem
	// SetEstatico (testes das etapas 1–5), handleEstatico devolve 404.
	s.mux.HandleFunc("GET /", s.handleEstatico)
}

// SetSaude injeta as fontes de cert_expira_em e tick_em (etapa 5) — setter
// em vez de mais parâmetros em New: New(st, ap, versao) já é chamado assim
// em todos os testes das etapas 1–4 (httpapi_test.go, fluxo_test.go,
// erros_test.go, push_test.go); mudar a assinatura quebraria todos eles por
// um recurso que a maioria não usa. main.go chama SetSaude depois de criar
// o certLoader e o scheduler.
func (s *Server) SetSaude(cert CertInfo, tick TickInfo) {
	s.cert = cert
	s.tick = tick
}

// handleHealth responde {ok, versao, tick_em, cert_expira_em, subs}
// (CONTRATO §5/§7, etapa 5). ok é false se o banco não responde, se o
// certificado vence em menos de 7 dias, ou se o último tick do scheduler
// foi há mais de 5 min (sem tick/cert configurados — SetSaude nunca
// chamado — essas duas condições são ignoradas, tick_em/cert_expira_em
// saem null).
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ok := s.st.Ping() == nil
	agora := s.ap.Agora()

	var tickEm any
	if s.tick != nil {
		if ultimo := s.tick.UltimoTick(); !ultimo.IsZero() {
			tickEm = domain.FormatInstante(ultimo)
			if agora.Sub(ultimo) > limiarSaudeTick {
				ok = false
			}
		} else {
			// nunca rodou ainda (corrida rara: requisição antes do 1º tick).
			ok = false
		}
	}

	var certEm any
	if s.cert != nil {
		if validade, err := s.cert.Validade(); err == nil {
			certEm = domain.FormatInstante(validade.In(s.ap.Localizacao()))
			if validade.Sub(agora) < limiarSaudeCert {
				ok = false
			}
		}
	}

	subs := 0
	if lista, err := s.st.PushSubListar(r.Context()); err == nil {
		subs = len(lista)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             ok,
		"versao":         s.versao,
		"tick_em":        tickEm,
		"cert_expira_em": certEm,
		"subs":           subs,
	})
}

// writeJSON escreve v como JSON com o status dado.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErro escreve o formato de erro da API (CONTRATO §5):
// {"erro": "mensagem em português"} + status HTTP.
func writeErro(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"erro": msg})
}

// writeErroCaptura é o writeErro da captura: quando a transcrição já tinha
// saído e só a interpretação falhou, devolve também {"texto"} (V3: o
// recado falado não se perde).
func writeErroCaptura(w http.ResponseWriter, status int, msg, texto string) {
	if texto == "" {
		writeErro(w, status, msg)
		return
	}
	writeJSON(w, status, map[string]string{"erro": msg, "texto": texto})
}

// writeErroApp traduz um erro devolvido pelo app (DECISÃO Opus, decisão 10):
// app.ErrValidacao -> 400, app.ErrNaoEncontrado -> 404, app.ErrConflito ->
// 409; qualquer outro erro é 500 com {"erro":"erro interno"} — o erro real
// só vai para o log (slog), nunca para o cliente.
func writeErroApp(w http.ResponseWriter, err error) {
	var validacao app.ErrValidacao
	var naoEncontrado app.ErrNaoEncontrado
	var conflito app.ErrConflito
	var capturaDesligada app.ErrCapturaDesligada
	var capturaLimite app.ErrCapturaLimite
	var capturaProvedor app.ErrCapturaProvedor
	switch {
	case errors.As(err, &validacao):
		writeErro(w, http.StatusBadRequest, validacao.Msg)
	case errors.As(err, &naoEncontrado):
		writeErro(w, http.StatusNotFound, naoEncontrado.Msg)
	case errors.As(err, &conflito):
		writeErro(w, http.StatusConflict, conflito.Msg)
	case errors.As(err, &capturaDesligada):
		writeErro(w, http.StatusServiceUnavailable, capturaDesligada.Msg)
	case errors.As(err, &capturaLimite):
		writeErroCaptura(w, http.StatusTooManyRequests, capturaLimite.Msg, capturaLimite.Texto)
	case errors.As(err, &capturaProvedor):
		writeErroCaptura(w, http.StatusBadGateway, capturaProvedor.Msg, capturaProvedor.Texto)
	default:
		slog.Error("erro interno no httpapi", "erro", err)
		writeErro(w, http.StatusInternalServerError, "erro interno")
	}
}

// decodeJSON decodifica o corpo de r em v; corpo ausente ou malformado é
// erro (o chamador devolve 400 — "Body JSON inválido → 400", decisão 10).
func decodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return errors.New("corpo vazio")
	}
	return json.NewDecoder(r.Body).Decode(v)
}
