package httpapi

import (
	"encoding/json"
	"net/http"

	"pocket/internal/app"
)

// listaRequest é o corpo de POST /api/listas (CONTRATO-FASE3 §5).
type listaRequest struct {
	Nome string `json:"nome"`
}

func (s *Server) handleCriarLista(w http.ResponseWriter, r *http.Request) {
	var req listaRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErro(w, http.StatusBadRequest, "corpo JSON inválido")
		return
	}
	l, err := s.ap.CriarLista(r.Context(), app.ListaEntrada{Nome: req.Nome})
	if err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, listaParaResponse(l, nil))
}

func (s *Server) handleListarListas(w http.ResponseWriter, r *http.Request) {
	listas, err := s.ap.ListarListas(r.Context())
	if err != nil {
		writeErroApp(w, err)
		return
	}
	out := make([]listaResponse, 0, len(listas))
	for _, lp := range listas {
		pendentes := lp.Pendentes
		out = append(out, listaParaResponse(lp.Lista, &pendentes))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDetalheLista(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}
	det, err := s.ap.DetalheLista(r.Context(), id)
	if err != nil {
		writeErroApp(w, err)
		return
	}

	secoes := make([]secaoResponse, 0, len(det.Secoes))
	for _, sc := range det.Secoes {
		secoes = append(secoes, secaoParaResponse(sc))
	}
	itens := make([]itemCompraResponse, 0, len(det.Itens))
	for _, it := range det.Itens {
		itens = append(itens, itemCompraParaResponse(it))
	}
	writeJSON(w, http.StatusOK, listaDetalheResponse{
		Lista: listaParaResponse(det.Lista, nil), Secoes: secoes, Itens: itens,
	})
}

// listaPatchRequest é o corpo de PATCH /api/listas/{id} (§5). Lembrete usa
// json.RawMessage não-ponteiro (não *json.RawMessage) de propósito: chave
// ausente deixa o slice no zero-value nil; "lembrete": null grava os 4
// bytes literais "null" (não-nil, distinguível de ausente) — é assim que o
// handler diferencia "não mude" de "remova" (ver DECISÃO em
// app.ListaPatch).
type listaPatchRequest struct {
	Nome     *string         `json:"nome"`
	Ordem    *int            `json:"ordem"`
	Lembrete json.RawMessage `json:"lembrete"`
}

// lembreteRequest é o formato de "lembrete" quando presente e não-null
// (§2.6): {"regra":{...},"horarios":["09:00"]}.
type lembreteRequest struct {
	Regra    json.RawMessage `json:"regra"`
	Horarios []string        `json:"horarios"`
}

func (s *Server) handleEditarLista(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}

	var req listaPatchRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErro(w, http.StatusBadRequest, "corpo JSON inválido")
		return
	}

	patch := app.ListaPatch{Nome: req.Nome, Ordem: req.Ordem}
	if len(req.Lembrete) > 0 {
		patch.LembretePresente = true
		if string(req.Lembrete) == "null" {
			patch.LembreteRemover = true
		} else {
			var lr lembreteRequest
			if err := json.Unmarshal(req.Lembrete, &lr); err != nil {
				writeErro(w, http.StatusBadRequest, "lembrete inválido")
				return
			}
			patch.Lembrete = &app.ListaLembreteEntrada{Regra: []byte(lr.Regra), Horarios: lr.Horarios}
		}
	}

	l, err := s.ap.EditarLista(r.Context(), id, patch)
	if err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listaParaResponse(l, nil))
}

type listaEstadoRequest struct {
	Arquivada bool `json:"arquivada"`
}

func (s *Server) handleMudarEstadoLista(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}
	var req listaEstadoRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErro(w, http.StatusBadRequest, "corpo JSON inválido")
		return
	}
	l, err := s.ap.MudarEstadoLista(r.Context(), id, req.Arquivada)
	if err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listaParaResponse(l, nil))
}

func (s *Server) handleLimparComprados(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}
	n, err := s.ap.LimparComprados(r.Context(), id)
	if err != nil {
		writeErroApp(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"removidos": n})
}

func (s *Server) handleFrequentes(w http.ResponseWriter, r *http.Request) {
	id, err := idDoPath(r, "id")
	if err != nil {
		writeErro(w, http.StatusBadRequest, err.Error())
		return
	}
	q := r.URL.Query().Get("q")
	hist, err := s.ap.Frequentes(r.Context(), id, q)
	if err != nil {
		writeErroApp(w, err)
		return
	}
	out := make([]historicoResponse, 0, len(hist))
	for _, h := range hist {
		out = append(out, historicoParaResponse(h))
	}
	writeJSON(w, http.StatusOK, out)
}
