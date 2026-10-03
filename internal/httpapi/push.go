package httpapi

import (
	"net/http"

	"pocket/internal/app"
)

// handleVapid responde GET /api/push/vapid (§5): a chave pública que o
// front usa em PushManager.subscribe().
func (s *Server) handleVapid(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"chave_publica": s.ap.VapidPublica()})
}

// pushSubRequest é o corpo de POST /api/push/subscribe — o formato de
// PushSubscription.toJSON() do browser (endpoint + keys.p256dh/auth).
type pushSubRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func (s *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	var req pushSubRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErro(w, http.StatusBadRequest, "corpo JSON inválido")
		return
	}

	err := s.ap.InscreverPush(r.Context(), app.PushSubEntrada{
		Endpoint: req.Endpoint, P256dh: req.Keys.P256dh, Auth: req.Keys.Auth, UserAgent: r.UserAgent(),
	})
	if err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handlePushTeste responde POST /api/push/teste (§5): push imediato
// "teste" para todas as inscrições.
func (s *Server) handlePushTeste(w http.ResponseWriter, r *http.Request) {
	enviados, err := s.ap.TestarPush(r.Context())
	if err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"enviados": enviados})
}

// handleAcaoNotificacao responde POST /api/notificacoes/{id}/{acao} (§5) —
// chamado pelo service worker: recebida, feito, adiar, abrir.
func (s *Server) handleAcaoNotificacao(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}
	acao := r.PathValue("acao")

	if err := s.ap.AcaoNotificacao(r.Context(), id, acao); err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
