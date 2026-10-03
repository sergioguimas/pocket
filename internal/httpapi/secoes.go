package httpapi

import (
	"net/http"

	"pocket/internal/app"
)

// secaoRequest é o corpo de POST /api/listas/{id}/secoes (CONTRATO-FASE3
// §2.2/§5).
type secaoRequest struct {
	Nome string `json:"nome"`
}

func (s *Server) handleCriarSecao(w http.ResponseWriter, r *http.Request) {
	listaID, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}
	var req secaoRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErro(w, http.StatusBadRequest, "corpo JSON inválido")
		return
	}
	sc, err := s.ap.CriarSecao(r.Context(), listaID, req.Nome)
	if err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, secaoParaResponse(sc))
}

type secaoPatchRequest struct {
	Nome  *string `json:"nome"`
	Ordem *int    `json:"ordem"`
}

func (s *Server) handleEditarSecao(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}
	var req secaoPatchRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErro(w, http.StatusBadRequest, "corpo JSON inválido")
		return
	}
	sc, err := s.ap.EditarSecao(r.Context(), id, app.SecaoPatch{Nome: req.Nome, Ordem: req.Ordem})
	if err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusOK, secaoParaResponse(sc))
}

func (s *Server) handleApagarSecao(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.ap.ApagarSecao(r.Context(), id); err != nil {
		writeErroApp(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
