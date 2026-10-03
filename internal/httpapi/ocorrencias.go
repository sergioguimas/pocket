package httpapi

import "net/http"

// handlePular e handleDespular respondem com o item atualizado (com
// métricas) — DECISÃO Opus (decisão 9 não fixa a resposta destas duas
// rotas; mantém o mesmo formato de POST /registros e DELETE /registros/{id}
// por consistência).
func (s *Server) handlePular(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}
	cm, err := s.ap.Pular(r.Context(), id)
	if err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusOK, itemComMetricasParaResponse(cm))
}

func (s *Server) handleDespular(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}
	cm, err := s.ap.Despular(r.Context(), id)
	if err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusOK, itemComMetricasParaResponse(cm))
}
