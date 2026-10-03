package httpapi

import (
	"encoding/json"
	"net/http"

	"pocket/internal/app"
)

// itemRequest é o corpo de POST /api/itens (CONTRATO §5). Regra chega crua
// (json.RawMessage) — o app é quem valida o formato de §3.2.
//
// DECISÃO: Regra é *json.RawMessage (ponteiro), não json.RawMessage puro —
// só assim "regra" ausente e "regra": null (explícito) colapsam no mesmo
// nil (regra do encoding/json para ponteiro: JSON null sempre zera o
// ponteiro antes de chamar qualquer UnmarshalJSON). Um json.RawMessage sem
// ponteiro guarda os bytes literais "null" nesse caso — item.Regra viraria
// não-vazio e um compromisso sem "regra" no corpo seria rejeitado por
// engano ("compromisso não tem regra de frequência").
type itemRequest struct {
	Tipo     string           `json:"tipo"`
	Titulo   string           `json:"titulo"`
	Notas    string           `json:"notas"`
	Regra    *json.RawMessage `json:"regra"`
	Data     string           `json:"data"`
	Hora     string           `json:"hora"`
	Horarios []string         `json:"horarios"`
	Cor      string           `json:"cor"`
	Ordem    int              `json:"ordem"`
}

func (s *Server) handleCriarItem(w http.ResponseWriter, r *http.Request) {
	var req itemRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErro(w, http.StatusBadRequest, "corpo JSON inválido")
		return
	}

	var regra []byte
	if req.Regra != nil {
		regra = []byte(*req.Regra)
	}
	item, err := s.ap.CriarItem(r.Context(), app.ItemEntrada{
		Tipo: req.Tipo, Titulo: req.Titulo, Notas: req.Notas, Regra: regra,
		Data: req.Data, Hora: req.Hora, Horarios: req.Horarios, Cor: req.Cor, Ordem: req.Ordem,
	})
	if err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, itemParaResponse(item, nil))
}

func (s *Server) handleListarItens(w http.ResponseWriter, r *http.Request) {
	tipo := r.URL.Query().Get("tipo")
	estado := r.URL.Query().Get("estado")

	itens, err := s.ap.ListarComMetricas(r.Context(), tipo, estado)
	if err != nil {
		writeErroApp(w, err)
		return
	}
	out := make([]itemResponse, 0, len(itens))
	for _, it := range itens {
		out = append(out, itemComMetricasParaResponse(it))
	}
	writeJSON(w, http.StatusOK, out)
}

// itemDetalheResponse é o corpo de GET /api/itens/{id} (decisão 9): item
// (com métricas) + últimos 30 registros (mais recentes primeiro) +
// ocorrência aberta corrente (null se não houver nenhuma ainda "aberta").
type itemDetalheResponse struct {
	Item             itemResponse        `json:"item"`
	Registros        []registroResponse  `json:"registros"`
	OcorrenciaAberta *ocorrenciaResponse `json:"ocorrencia_aberta"`
}

func (s *Server) handleDetalheItem(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}

	det, err := s.ap.DetalheItem(r.Context(), id)
	if err != nil {
		writeErroApp(w, err)
		return
	}

	registros := make([]registroResponse, 0, len(det.Registros))
	for _, reg := range det.Registros {
		registros = append(registros, registroParaResponse(reg))
	}
	resp := itemDetalheResponse{
		Item:      itemComMetricasParaResponse(det.ItemComMetricas),
		Registros: registros,
	}
	if det.OcorrenciaAberta != nil {
		oc := ocorrenciaParaResponse(*det.OcorrenciaAberta)
		resp.OcorrenciaAberta = &oc
	}
	writeJSON(w, http.StatusOK, resp)
}

// itemPatchRequest é o corpo de PATCH /api/itens/{id} (decisão 6): todo
// campo é opcional (ponteiro nil = não veio); Tipo só existe para detectar
// a tentativa de mudar o tipo (sempre 400, decisão 6).
type itemPatchRequest struct {
	Tipo     *string          `json:"tipo"`
	Titulo   *string          `json:"titulo"`
	Notas    *string          `json:"notas"`
	Regra    *json.RawMessage `json:"regra"`
	Data     *string          `json:"data"`
	Hora     *string          `json:"hora"`
	Horarios *[]string        `json:"horarios"`
	Cor      *string          `json:"cor"`
	Ordem    *int             `json:"ordem"`
}

func (s *Server) handleEditarItem(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}

	var req itemPatchRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErro(w, http.StatusBadRequest, "corpo JSON inválido")
		return
	}

	patch := app.ItemPatch{
		TipoPresente: req.Tipo != nil,
		Titulo:       req.Titulo,
		Notas:        req.Notas,
		Data:         req.Data,
		Hora:         req.Hora,
		Horarios:     req.Horarios,
		Cor:          req.Cor,
		Ordem:        req.Ordem,
	}
	if req.Regra != nil {
		b := []byte(*req.Regra)
		patch.Regra = &b
	}

	item, err := s.ap.EditarItem(r.Context(), id, patch)
	if err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusOK, itemParaResponse(item, nil))
}

type estadoRequest struct {
	Estado string `json:"estado"`
}

func (s *Server) handleMudarEstado(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}

	var req estadoRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErro(w, http.StatusBadRequest, "corpo JSON inválido")
		return
	}

	item, err := s.ap.MudarEstado(r.Context(), id, req.Estado)
	if err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusOK, itemParaResponse(item, nil))
}

type calendarioDiaResponse struct {
	Dia       string `json:"dia"`
	Status    string `json:"status"`
	Registros int    `json:"registros"`
}

func (s *Server) handleCalendario(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}

	de := r.URL.Query().Get("de")
	ate := r.URL.Query().Get("ate")

	dias, err := s.ap.Calendario(r.Context(), id, de, ate)
	if err != nil {
		writeErroApp(w, err)
		return
	}
	out := make([]calendarioDiaResponse, 0, len(dias))
	for _, d := range dias {
		out = append(out, calendarioDiaResponse{Dia: string(d.Dia), Status: d.Status, Registros: d.Registros})
	}
	writeJSON(w, http.StatusOK, out)
}
