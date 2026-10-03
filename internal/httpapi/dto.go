package httpapi

import (
	"net/http"
	"strconv"

	"pocket/internal/app"
	"pocket/internal/domain"
)

// Este arquivo reúne as structs JSON de resposta e as conversões a partir
// dos tipos de app/domain (DECISÃO Opus, decisão 9). Os formatos completos,
// com exemplo, estão documentados em servicos/pocket/API.md — a etapa 6
// (front) lê só aquele arquivo e o contrato, não este código.

// itemResponse é o item como aparece em toda resposta da API.
type itemResponse struct {
	ID           int64             `json:"id"`
	Tipo         string            `json:"tipo"`
	Titulo       string            `json:"titulo"`
	Notas        string            `json:"notas"`
	Regra        *domain.Regra     `json:"regra,omitempty"`
	Data         *string           `json:"data,omitempty"`
	Hora         string            `json:"hora,omitempty"`
	Horarios     []string          `json:"horarios"`
	Estado       string            `json:"estado"`
	Cor          string            `json:"cor,omitempty"`
	Ordem        int               `json:"ordem"`
	CriadoEm     string            `json:"criado_em"`
	AtualizadoEm string            `json:"atualizado_em"`
	Metricas     *metricasResponse `json:"metricas,omitempty"`
}

type metricasResponse struct {
	SequenciaAtual  int                `json:"sequencia_atual"`
	MelhorSequencia int                `json:"melhor_sequencia"`
	Taxa30d         *float64           `json:"taxa_30d"`
	Intervalo       *intervaloResponse `json:"intervalo,omitempty"`
}

type intervaloResponse struct {
	EmDia bool `json:"em_dia"`
	Dias  int  `json:"dias"`
}

func itemParaResponse(item domain.Item, m *app.Metricas) itemResponse {
	resp := itemResponse{
		ID: item.ID, Tipo: item.Tipo, Titulo: item.Titulo, Notas: item.Notas,
		Hora: item.Hora, Horarios: item.Horarios, Estado: item.Estado, Cor: item.Cor,
		Ordem: item.Ordem, CriadoEm: domain.FormatInstante(item.CriadoEm),
		AtualizadoEm: domain.FormatInstante(item.AtualizadoEm),
	}
	if resp.Horarios == nil {
		resp.Horarios = []string{}
	}
	if item.Regra != nil {
		resp.Regra = item.Regra
	}
	if item.Data != nil {
		d := string(*item.Data)
		resp.Data = &d
	}
	if m != nil {
		resp.Metricas = metricasParaResponse(m)
	}
	return resp
}

func metricasParaResponse(m *app.Metricas) *metricasResponse {
	mr := &metricasResponse{SequenciaAtual: m.SequenciaAtual, MelhorSequencia: m.MelhorSequencia, Taxa30d: m.Taxa30d}
	if m.Intervalo != nil {
		mr.Intervalo = &intervaloResponse{EmDia: m.Intervalo.EmDia, Dias: m.Intervalo.Dias}
	}
	return mr
}

func itemComMetricasParaResponse(cm app.ItemComMetricas) itemResponse {
	return itemParaResponse(cm.Item, cm.Metricas)
}

// ocorrenciaResponse é a ocorrência como aparece em Hoje, no detalhe do
// item e no registro. Feitos só é preenchido em Hoje (chip "2/3").
type ocorrenciaResponse struct {
	ID     int64   `json:"id"`
	Inicio *string `json:"inicio,omitempty"`
	Fim    *string `json:"fim,omitempty"`
	Meta   int     `json:"meta"`
	Status string  `json:"status"`
	Feitos *int    `json:"feitos,omitempty"`
}

func ocorrenciaParaResponse(oc domain.Ocorrencia) ocorrenciaResponse {
	r := ocorrenciaResponse{ID: oc.ID, Meta: oc.Meta, Status: oc.Status}
	if oc.Inicio != nil {
		s := string(*oc.Inicio)
		r.Inicio = &s
	}
	if oc.Fim != nil {
		s := string(*oc.Fim)
		r.Fim = &s
	}
	return r
}

// registroResponse é o registro em POST /registros e no detalhe do item.
type registroResponse struct {
	ID     int64  `json:"id"`
	Em     string `json:"em"`
	Nota   string `json:"nota,omitempty"`
	Origem string `json:"origem"`
}

func registroParaResponse(r domain.Registro) registroResponse {
	return registroResponse{ID: r.ID, Em: domain.FormatInstante(r.Em), Nota: r.Nota, Origem: r.Origem}
}

// --- fase 3 (compras, CONTRATO-FASE3 §5) ---

// listaResponse é o objeto "lista" de toda resposta que envolve uma lista.
// Pendentes só é preenchido em GET /api/listas (decisão 9 da fase 1:
// campos que só existem em certas rotas ficam omitempty).
type listaResponse struct {
	ID        int64             `json:"id"`
	Nome      string            `json:"nome"`
	Ordem     int               `json:"ordem"`
	Arquivada bool              `json:"arquivada"`
	Lembrete  *lembreteResponse `json:"lembrete"`
	CriadoEm  string            `json:"criado_em"`
	Pendentes *int              `json:"pendentes,omitempty"`
}

type lembreteResponse struct {
	Regra    *domain.Regra `json:"regra"`
	Horarios []string      `json:"horarios"`
}

func listaParaResponse(l domain.Lista, pendentes *int) listaResponse {
	resp := listaResponse{
		ID: l.ID, Nome: l.Nome, Ordem: l.Ordem, Arquivada: l.Arquivada,
		CriadoEm: l.CriadoEm, Pendentes: pendentes,
	}
	if l.Lembrete != nil {
		resp.Lembrete = &lembreteResponse{Regra: l.Lembrete.Regra, Horarios: l.Lembrete.Horarios}
		if resp.Lembrete.Horarios == nil {
			resp.Lembrete.Horarios = []string{}
		}
	}
	return resp
}

// secaoResponse é o objeto "secao" (§2.2).
type secaoResponse struct {
	ID      int64  `json:"id"`
	ListaID int64  `json:"lista_id"`
	Nome    string `json:"nome"`
	Ordem   int    `json:"ordem"`
}

func secaoParaResponse(sc domain.Secao) secaoResponse {
	return secaoResponse{ID: sc.ID, ListaID: sc.ListaID, Nome: sc.Nome, Ordem: sc.Ordem}
}

// itemCompraResponse é o objeto "item" de uma lista de compras (§2.3).
type itemCompraResponse struct {
	ID         int64  `json:"id"`
	ListaID    int64  `json:"lista_id"`
	SecaoID    int64  `json:"secao_id"`
	Nome       string `json:"nome"`
	NomeChave  string `json:"nome_chave"`
	Quantidade string `json:"quantidade,omitempty"`
	Nota       string `json:"nota,omitempty"`
	Comprado   bool   `json:"comprado"`
	CompradoEm string `json:"comprado_em,omitempty"`
	Origem     string `json:"origem"`
	ReceitaID  *int64 `json:"receita_id,omitempty"`
	CriadoEm   string `json:"criado_em"`
}

func itemCompraParaResponse(ic domain.ItemCompra) itemCompraResponse {
	return itemCompraResponse{
		ID: ic.ID, ListaID: ic.ListaID, SecaoID: ic.SecaoID, Nome: ic.Nome, NomeChave: ic.NomeChave,
		Quantidade: ic.Quantidade, Nota: ic.Nota, Comprado: ic.Comprado, CompradoEm: ic.CompradoEm,
		Origem: ic.Origem, ReceitaID: ic.ReceitaID, CriadoEm: ic.CriadoEm,
	}
}

// listaDetalheResponse é o corpo de GET /api/listas/{id} (§5): lista +
// seções (ordenadas) + itens (ordem de §2.3 regra 4).
type listaDetalheResponse struct {
	Lista  listaResponse        `json:"lista"`
	Secoes []secaoResponse      `json:"secoes"`
	Itens  []itemCompraResponse `json:"itens"`
}

// historicoResponse é a linha de GET /api/listas/{id}/frequentes (§2.4).
type historicoResponse struct {
	Nome             string `json:"nome"`
	NomeChave        string `json:"nome_chave"`
	SecaoID          *int64 `json:"secao_id,omitempty"`
	UltimaQuantidade string `json:"ultima_quantidade,omitempty"`
	Vezes            int    `json:"vezes"`
	UltimaEm         string `json:"ultima_em"`
}

func historicoParaResponse(h domain.CompraHistorico) historicoResponse {
	return historicoResponse{
		Nome: h.Nome, NomeChave: h.NomeChave, SecaoID: h.SecaoID,
		UltimaQuantidade: h.UltimaQuantidade, Vezes: h.Vezes, UltimaEm: h.UltimaEm,
	}
}

// idDoPath lê e valida um inteiro do path (ex.: {id}) — 400 se não for um
// número.
func idDoPath(r *http.Request, campo string) (int64, error) {
	v := r.PathValue(campo)
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, errIDInvalido{campo: campo, valor: v}
	}
	return id, nil
}

type errIDInvalido struct {
	campo, valor string
}

func (e errIDInvalido) Error() string {
	return e.campo + " inválido (" + strconv.Quote(e.valor) + ")"
}
