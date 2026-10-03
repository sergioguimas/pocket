package httpapi

import (
	"net/http"

	"pocket/internal/app"
)

// itemCompraRequest é o corpo de POST /api/listas/{id}/itens (CONTRATO-
// FASE3 §5).
type itemCompraRequest struct {
	Nome       string `json:"nome"`
	Quantidade string `json:"quantidade"`
	SecaoID    *int64 `json:"secao_id"`
	Origem     string `json:"origem"`
}

func (s *Server) handleAdicionarItem(w http.ResponseWriter, r *http.Request) {
	listaID, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}
	var req itemCompraRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErro(w, http.StatusBadRequest, "corpo JSON inválido")
		return
	}

	item, criado, err := s.ap.AdicionarItem(r.Context(), listaID, app.ItemCompraEntrada{
		Nome: req.Nome, Quantidade: req.Quantidade, SecaoID: req.SecaoID, Origem: req.Origem,
	})
	if err != nil {
		writeErroApp(w, err)
		return
	}
	status := http.StatusOK
	if criado {
		status = http.StatusCreated
	}
	writeJSON(w, status, itemCompraParaResponse(item))
}

// itemCompraPatchRequest é o corpo de PATCH /api/compras/{id} (§5).
type itemCompraPatchRequest struct {
	Nome       *string `json:"nome"`
	Quantidade *string `json:"quantidade"`
	SecaoID    *int64  `json:"secao_id"`
	Nota       *string `json:"nota"`
}

func (s *Server) handleEditarItemCompra(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}
	var req itemCompraPatchRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErro(w, http.StatusBadRequest, "corpo JSON inválido")
		return
	}
	item, err := s.ap.EditarItemCompra(r.Context(), id, app.ItemCompraPatch{
		Nome: req.Nome, Quantidade: req.Quantidade, SecaoID: req.SecaoID, Nota: req.Nota,
	})
	if err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusOK, itemCompraParaResponse(item))
}

func (s *Server) handleApagarItemCompra(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.ap.ApagarItemCompra(r.Context(), id); err != nil {
		writeErroApp(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRiscar(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := s.ap.Riscar(r.Context(), id)
	if err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusOK, itemCompraParaResponse(item))
}

func (s *Server) handleDesriscar(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := s.ap.Desriscar(r.Context(), id)
	if err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusOK, itemCompraParaResponse(item))
}
