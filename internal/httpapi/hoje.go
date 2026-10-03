package httpapi

import "net/http"

// hojeResponse é o corpo de GET /api/hoje (decisão 9; + Compras, §2.7 da
// fase 3).
type hojeResponse struct {
	Data         string                    `json:"data"`
	Habitos      []hojeHabitoResponse      `json:"habitos"`
	Compromissos []hojeCompromissoResponse `json:"compromissos"`
	SemData      int                       `json:"sem_data"`
	Compras      []hojeListaResponse       `json:"compras"`
}

// hojeListaResponse é uma linha de "compras" em Hoje (§2.7).
type hojeListaResponse struct {
	ListaID   int64  `json:"lista_id"`
	Nome      string `json:"nome"`
	Pendentes int    `json:"pendentes"`
}

type hojeHabitoResponse struct {
	Item       itemResponse       `json:"item"`
	Ocorrencia ocorrenciaResponse `json:"ocorrencia"`
	Intervalo  *intervaloResponse `json:"intervalo,omitempty"`
}

type hojeCompromissoResponse struct {
	Item       itemResponse       `json:"item"`
	Ocorrencia ocorrenciaResponse `json:"ocorrencia"`
	Atrasado   bool               `json:"atrasado"`
}

func (s *Server) handleHoje(w http.ResponseWriter, r *http.Request) {
	resp, err := s.ap.Hoje(r.Context())
	if err != nil {
		writeErroApp(w, err)
		return
	}

	out := hojeResponse{
		Data:         string(resp.Dia),
		SemData:      resp.SemData,
		Habitos:      make([]hojeHabitoResponse, 0, len(resp.Habitos)),
		Compromissos: make([]hojeCompromissoResponse, 0, len(resp.Compromissos)),
		Compras:      make([]hojeListaResponse, 0, len(resp.Compras)),
	}
	for _, c := range resp.Compras {
		out.Compras = append(out.Compras, hojeListaResponse{ListaID: c.ListaID, Nome: c.Nome, Pendentes: c.Pendentes})
	}
	for _, h := range resp.Habitos {
		ocResp := ocorrenciaParaResponse(h.Ocorrencia)
		feitos := h.Feitos
		ocResp.Feitos = &feitos

		var interv *intervaloResponse
		if h.Intervalo != nil {
			interv = &intervaloResponse{EmDia: h.Intervalo.EmDia, Dias: h.Intervalo.Dias}
		}
		out.Habitos = append(out.Habitos, hojeHabitoResponse{
			Item: itemParaResponse(h.Item, nil), Ocorrencia: ocResp, Intervalo: interv,
		})
	}
	for _, c := range resp.Compromissos {
		out.Compromissos = append(out.Compromissos, hojeCompromissoResponse{
			Item: itemParaResponse(c.Item, nil), Ocorrencia: ocorrenciaParaResponse(c.Ocorrencia), Atrasado: c.Atrasado,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
