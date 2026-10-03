package httpapi

import (
	"net/http"

	"pocket/internal/app"
	"pocket/internal/domain"
)

// registroRequest é o corpo de POST /api/itens/{id}/registros (CONTRATO
// §5): em e nota são opcionais; em omitido = agora (decisão 3).
type registroRequest struct {
	Em   string `json:"em"`
	Nota string `json:"nota"`
}

// registroResultResponse é a resposta de POST /registros (decisão 9): item
// atualizado (com métricas) + o registro criado.
type registroResultResponse struct {
	Item     itemResponse     `json:"item"`
	Registro registroResponse `json:"registro"`
}

func (s *Server) handleCriarRegistro(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}

	var req registroRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErro(w, http.StatusBadRequest, "corpo JSON inválido")
		return
	}

	cm, registro, err := s.ap.Registrar(r.Context(), id, app.RegistroEntrada{
		Em: req.Em, Nota: req.Nota, Origem: domain.OrigemApp,
	})
	if err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, registroResultResponse{
		Item: itemComMetricasParaResponse(cm), Registro: registroParaResponse(registro),
	})
}

func (s *Server) handleDesfazerRegistro(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}

	cm, err := s.ap.DesfazerRegistro(r.Context(), id)
	if err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusOK, itemComMetricasParaResponse(cm))
}

// handleEditarNotaRegistro: PATCH /api/registros/{id} {nota} → registro.
func (s *Server) handleEditarNotaRegistro(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		Nota *string `json:"nota"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Nota == nil {
		writeErro(w, http.StatusBadRequest, "corpo precisa de {\"nota\": \"...\"}")
		return
	}
	reg, err := s.ap.EditarNotaRegistro(r.Context(), id, *req.Nota)
	if err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusOK, registroParaResponse(reg))
}
