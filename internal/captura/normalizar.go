package captura

import (
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"pocket/internal/domain"
)

// notaMax é o mesmo limite de internal/app/registros.go (nota de um feito).
const notaMax = 500

// maxItensCompra é o teto de itens por captura de compras (§2 regra 6, §3).
const maxItensCompra = 20

// Normalizar é o único lugar que confia (ou não) no que o modelo devolveu
// (§4: "Todo o 'não confiar no LLM' fica aqui"). Recebe o JSON cru da
// proposta (ainda não decodificado — Interpretador não decodifica, ver
// captura.go) e devolve a Resposta pronta para virar §3 (captura_id/texto
// são acrescentados por quem chama, o app). hoje/agora já vêm no fuso loc
// (o App é quem sabe disso — este pacote não lê clock).
func Normalizar(ctxInfo Contexto, modelJSON []byte, hoje domain.Dia, agora time.Time, loc *time.Location) Resposta {
	var proposta PropostaModelo
	if err := json.Unmarshal(modelJSON, &proposta); err != nil {
		return Resposta{Acao: AcaoDesconhecido, Avisos: []string{"não consegui entender essa fala"}}
	}

	switch proposta.Acao {
	case AcaoCriarItem:
		return normalizarCriarItem(proposta.Item, proposta.Resto)
	case AcaoRegistrar:
		return normalizarRegistrar(ctxInfo, proposta.Registro, hoje, agora, loc, proposta.Resto)
	case AcaoCompras:
		return normalizarCompras(ctxInfo, proposta.Compras, proposta.Resto)
	default:
		return Resposta{Acao: AcaoDesconhecido, Resto: normalizarResto(proposta.Resto)}
	}
}

func normalizarResto(resto *string) *string {
	if resto == nil {
		return nil
	}
	r := strings.TrimSpace(*resto)
	if r == "" {
		return nil
	}
	return &r
}

// --- criar_item ---

func normalizarCriarItem(in propostaItem, restoIn *string) Resposta {
	var avisos []string
	titulo := ""
	if in.Titulo != nil {
		titulo = strings.TrimSpace(*in.Titulo)
	}
	if utf8.RuneCountInString(titulo) < 1 || utf8.RuneCountInString(titulo) > 120 {
		// Sem título utilizável não dá para montar um item — a captura
		// inteira vira desconhecido (o front cai no caminho "Salvar como
		// tarefa"/"Abrir formulário", V8).
		return Resposta{Acao: AcaoDesconhecido, Avisos: []string{"não entendi o que criar"}}
	}

	tipo := ""
	if in.Tipo != nil {
		tipo = *in.Tipo
	}
	if tipo != domain.TipoHabito && tipo != domain.TipoCompromisso {
		tipo = domain.TipoCompromisso
		avisos = append(avisos, "não entendi se era hábito ou compromisso — criei como compromisso")
	}

	payload := ItemPayload{Tipo: tipo, Titulo: titulo}
	if in.Notas != nil {
		payload.Notas = strings.TrimSpace(*in.Notas)
	}

	if tipo == domain.TipoHabito {
		normalizarItemHabito(&payload, in, &avisos)
	} else {
		normalizarItemCompromisso(&payload, in, &avisos)
	}

	return Resposta{Acao: AcaoCriarItem, Item: &payload, Resto: normalizarResto(restoIn), Avisos: avisos}
}

func normalizarItemHabito(payload *ItemPayload, in propostaItem, avisos *[]string) {
	regraTipo := ""
	if in.RegraTipo != nil {
		regraTipo = *in.RegraTipo
	}
	if regraTipo == "" {
		*avisos = append(*avisos, "não entendi a frequência")
	} else {
		regraBytes, err := montarRegraJSON(regraTipo, in)
		if err != nil {
			*avisos = append(*avisos, "não entendi a frequência")
		} else if _, perr := domain.ParseRegra(regraBytes); perr != nil {
			*avisos = append(*avisos, "não entendi a frequência")
		} else {
			payload.Regra = json.RawMessage(regraBytes)
		}
	}

	// hábito não tem data/hora (domain rule) — o modelo não deveria mandar,
	// mas se mandou, ignora silenciosamente (não é erro do usuário, é o
	// modelo confundindo tipos).
	payload.Horarios = filtrarHorariosValidos(in.Horarios, validaHoraMinuto, avisos)
}

func normalizarItemCompromisso(payload *ItemPayload, in propostaItem, avisos *[]string) {
	if in.RegraTipo != nil && *in.RegraTipo != "" {
		*avisos = append(*avisos, "compromisso não tem regra de frequência — ignorada")
	}

	var dia *domain.Dia
	if in.Dia != nil && strings.TrimSpace(*in.Dia) != "" {
		d, err := domain.ParseDia(strings.TrimSpace(*in.Dia))
		if err != nil {
			*avisos = append(*avisos, "não entendi a data")
		} else {
			dia = &d
			payload.Data = string(d)
		}
	}

	hora := ""
	if in.Hora != nil {
		hora = strings.TrimSpace(*in.Hora)
	}
	if hora != "" {
		if _, _, err := domain.ParseHoraMinuto(hora); err != nil {
			*avisos = append(*avisos, "não entendi o horário")
			hora = ""
		} else if dia == nil {
			*avisos = append(*avisos, "hora sem data — ignorada")
			hora = ""
		}
	}
	payload.Hora = hora

	// Horarios: antecedências ("-15m","-2h","-1d") se tem hora; senão
	// "HH:MM" de lembrete se tem data; sem data, nenhum lembrete cabe
	// (domain.ValidarHorarios) — reusa a mesma validação do resto do app.
	item := domain.Item{Tipo: domain.TipoCompromisso}
	if dia != nil {
		item.Data = dia
	}
	item.Hora = hora

	var candidatos []string
	if hora != "" {
		candidatos = filtrarHorariosValidos(in.Horarios, validaAntecedencia, avisos)
	} else if dia != nil {
		candidatos = filtrarHorariosValidos(in.Horarios, validaHoraMinuto, avisos)
	} else if len(in.Horarios) > 0 {
		*avisos = append(*avisos, "compromisso sem data não aceita lembrete — ignorado")
	}
	item.Horarios = candidatos

	if err := domain.ValidarHorarios(item); err != nil {
		*avisos = append(*avisos, "não entendi o lembrete")
		candidatos = nil
	}
	payload.Horarios = candidatos
}

// filtrarHorariosValidos mantém só os valores de lista que validam por
// parse(v) == nil, descartando o resto (com um aviso único, não um por
// item — a lista costuma ser curta).
func filtrarHorariosValidos(lista []string, valida func(string) error, avisos *[]string) []string {
	var out []string
	descartou := false
	for _, s := range lista {
		if err := valida(s); err != nil {
			descartou = true
			continue
		}
		out = append(out, s)
	}
	if descartou {
		*avisos = append(*avisos, "descartei um horário que não entendi")
	}
	return out
}

func validaHoraMinuto(s string) error {
	_, _, err := domain.ParseHoraMinuto(s)
	return err
}

func validaAntecedencia(s string) error {
	_, err := domain.ParseAntecedencia(s)
	return err
}

func montarRegraJSON(tipo string, in propostaItem) ([]byte, error) {
	switch domain.TipoRegra(tipo) {
	case domain.RegraDiario:
		return json.Marshal(map[string]any{"tipo": tipo})
	case domain.RegraDiasSemana:
		return json.Marshal(map[string]any{"tipo": tipo, "dias": in.RegraDias})
	case domain.RegraVezesPeriodo:
		periodo := ""
		if in.RegraPeriodo != nil {
			periodo = *in.RegraPeriodo
		}
		vezes := 0
		if in.RegraVezes != nil {
			vezes = *in.RegraVezes
		}
		return json.Marshal(map[string]any{"tipo": tipo, "vezes": vezes, "periodo": periodo})
	case domain.RegraIntervalo:
		dias := 0
		if in.RegraIntervaloDias != nil {
			dias = *in.RegraIntervaloDias
		}
		return json.Marshal(map[string]any{"tipo": tipo, "dias": dias})
	default:
		return nil, errRegraDesconhecida
	}
}

var errRegraDesconhecida = jsonErr("tipo de regra desconhecido")

type jsonErr string

func (e jsonErr) Error() string { return string(e) }

// --- registrar ---

func normalizarRegistrar(ctxInfo Contexto, in propostaRegistro, hoje domain.Dia, agora time.Time, loc *time.Location, restoIn *string) Resposta {
	var avisos []string

	tituloFalado := ""
	if in.Titulo != nil {
		tituloFalado = strings.TrimSpace(*in.Titulo)
	}

	// regra 3: item_id só é aceito se aparecer no contexto (hábitos ativos
	// + compromissos abertos); fora disso vira criar_item.
	var achadoID int64
	var achadoTitulo, achadoTipo string
	achou := false
	if in.ItemID != nil {
		for _, h := range ctxInfo.Habitos {
			if h.ID == *in.ItemID {
				achadoID, achadoTitulo, achadoTipo, achou = h.ID, h.Titulo, domain.TipoHabito, true
				break
			}
		}
		if !achou {
			for _, c := range ctxInfo.Compromissos {
				if c.ID == *in.ItemID {
					achadoID, achadoTitulo, achadoTipo, achou = c.ID, c.Titulo, domain.TipoCompromisso, true
					break
				}
			}
		}
	}

	if !achou {
		titulo := tituloFalado
		if utf8.RuneCountInString(titulo) < 1 || utf8.RuneCountInString(titulo) > 120 {
			return Resposta{Acao: AcaoDesconhecido, Avisos: []string{"não entendi o que você fez"}}
		}
		// DECISÃO (revisão Opus): "fiz X" de algo que não existe é, quase
		// sempre, um hábito que ainda não foi cadastrado — o formulário abre
		// como hábito (regra padrão do EditorSheet) e o usuário ajusta.
		avisos = append(avisos, "não achei esse item — criando um hábito novo")
		payload := ItemPayload{Tipo: domain.TipoHabito, Titulo: titulo}
		return Resposta{Acao: AcaoCriarItem, Item: &payload, Resto: normalizarResto(restoIn), Avisos: avisos}
	}

	reg := &RegistroPayload{ItemID: achadoID, Titulo: achadoTitulo, Tipo: achadoTipo}

	var em *string
	if in.Dia != nil && strings.TrimSpace(*in.Dia) != "" {
		d, err := domain.ParseDia(strings.TrimSpace(*in.Dia))
		if err != nil {
			avisos = append(avisos, "não entendi a data do registro — usei agora")
		} else {
			switch {
			case d < hoje:
				t, terr := d.Time()
				if terr == nil {
					y, m, dd := t.Date()
					instante := time.Date(y, m, dd, 12, 0, 0, 0, loc)
					s := domain.FormatInstante(instante)
					em = &s
				}
			case d > hoje:
				avisos = append(avisos, "não dá para registrar no futuro — usei agora")
			default:
				// hoje: em fica nil (agora), mesma regra de "omitido".
			}
		}
	}
	reg.Em = em

	if in.Nota != nil {
		nota := strings.TrimSpace(*in.Nota)
		if utf8.RuneCountInString(nota) > notaMax {
			r := []rune(nota)
			nota = string(r[:notaMax])
			avisos = append(avisos, "nota muito longa — cortei em 500 caracteres")
		}
		reg.Nota = nota
	}

	return Resposta{Acao: AcaoRegistrar, Registro: reg, Resto: normalizarResto(restoIn), Avisos: avisos}
}

// --- compras ---

func normalizarCompras(ctxInfo Contexto, in propostaCompras, restoIn *string) Resposta {
	var avisos []string

	var listaID int64
	var listaNome string
	achou := false
	if in.ListaNome != nil {
		nome := strings.TrimSpace(*in.ListaNome)
		if nome != "" {
			alvo := domain.NomeChave(nome)
			for _, l := range ctxInfo.Listas {
				if domain.NomeChave(l.Nome) == alvo {
					listaID, listaNome, achou = l.ID, l.Nome, true
					break
				}
			}
		}
	}
	if !achou {
		if len(ctxInfo.Listas) == 0 {
			return Resposta{Acao: AcaoDesconhecido, Avisos: []string{"nenhuma lista de compras encontrada"}}
		}
		listaID, listaNome = ctxInfo.Listas[0].ID, ctxInfo.Listas[0].Nome
		if in.ListaNome != nil && strings.TrimSpace(*in.ListaNome) != "" {
			avisos = append(avisos, "não achei essa lista — usei "+listaNome)
		}
	}

	itens := in.Itens
	if len(itens) > maxItensCompra {
		itens = itens[:maxItensCompra]
		avisos = append(avisos, "mostrei só os 20 primeiros itens")
	}

	var out []ItemCompraPayload
	for _, it := range itens {
		nome := strings.TrimSpace(it.Nome)
		if nome == "" {
			continue
		}
		qtd := ""
		if it.Quantidade != nil {
			qtd = strings.TrimSpace(*it.Quantidade)
		}
		out = append(out, ItemCompraPayload{Nome: nome, Quantidade: qtd})
	}

	if len(out) == 0 {
		return Resposta{Acao: AcaoDesconhecido, Avisos: []string{"não entendi os itens"}}
	}

	return Resposta{
		Acao:    AcaoCompras,
		Compras: &ComprasPayload{ListaID: listaID, ListaNome: listaNome, Itens: out},
		Resto:   normalizarResto(restoIn),
		Avisos:  avisos,
	}
}
