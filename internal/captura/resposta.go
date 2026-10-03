package captura

import "encoding/json"

// Resposta é o formato de §3 do contrato, menos captura_id/texto (que o
// app acrescenta — ele é quem sabe o id da linha gravada e já tem o texto
// em mãos antes de chamar Normalizar). Os quatro ponteiros/objeto (Item,
// Registro, Compras, Resto) são mutuamente exclusivos na prática (só um
// não-nil por Acao), mas o tipo não força isso — Normalizar garante.
type Resposta struct {
	Acao     string           `json:"acao"`
	Item     *ItemPayload     `json:"item"`
	Registro *RegistroPayload `json:"registro"`
	Compras  *ComprasPayload  `json:"compras"`
	Resto    *string          `json:"resto"`
	Avisos   []string         `json:"avisos"`
}

// ItemPayload tem o formato exato de NovoItemPayload (POST /api/itens,
// CONTRATO.md §5) — já passou por domain.Regra/ParseHoraMinuto/ParseDia e
// domain.ValidarHorarios. Regra crua em JSON (mesmo formato que o handler
// de POST /api/itens espera no corpo), para o front reenviar sem tradução.
type ItemPayload struct {
	Tipo     string          `json:"tipo"`
	Titulo   string          `json:"titulo"`
	Notas    string          `json:"notas,omitempty"`
	Regra    json.RawMessage `json:"regra,omitempty"`
	Data     string          `json:"data,omitempty"`
	Hora     string          `json:"hora,omitempty"`
	Horarios []string        `json:"horarios,omitempty"`
}

// RegistroPayload é o cartão "Registrar {Titulo} {quando}?" (§2/§3).
type RegistroPayload struct {
	ItemID int64   `json:"item_id"`
	Titulo string  `json:"titulo"`
	Tipo   string  `json:"tipo"` // "habito" | "compromisso" — para o front escolher o texto do cartão
	Em     *string `json:"em"`   // RFC3339 local; null = agora
	Nota   string  `json:"nota,omitempty"`
}

// ComprasPayload é o cartão de compras (§2/§3): lista de destino + itens
// (desmarcáveis, nome/quantidade editáveis no front).
type ComprasPayload struct {
	ListaID   int64               `json:"lista_id"`
	ListaNome string              `json:"lista_nome"`
	Itens     []ItemCompraPayload `json:"itens"`
}

// ItemCompraPayload é um item dentro do cartão de compras.
type ItemCompraPayload struct {
	Nome       string `json:"nome"`
	Quantidade string `json:"quantidade,omitempty"`
}
