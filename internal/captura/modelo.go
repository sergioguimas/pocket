package captura

// PropostaModelo é a forma crua que o Groq devolve (§4: response_format
// json_schema, strict — ver schemaProposta em schema.go). Todo campo listado
// aqui está em "required" do schema; nullable vira ponteiro (ou, para
// listas, um array vazio — arrays nunca são null no schema). Nada aqui é
// confiado: Normalizar (normalizar.go) é quem decide o que sobrevive.
//
// DECISÃO: "item", "registro" e "compras" são sempre objetos presentes
// (nunca null) com campos escalares nullable — Groq em strict mode não
// aceita bem anyOf/oneOf nem objeto nullable (a starta rejeita ou o schema
// fica frágil); "acao" é quem diz qual dos três importa.
type PropostaModelo struct {
	Acao     string           `json:"acao"`
	Item     propostaItem     `json:"item"`
	Registro propostaRegistro `json:"registro"`
	Compras  propostaCompras  `json:"compras"`
	Resto    *string          `json:"resto"`
}

// Valores possíveis de PropostaModelo.Acao (§2 da tabela do contrato).
const (
	AcaoCriarItem    = "criar_item"
	AcaoRegistrar    = "registrar"
	AcaoCompras      = "compras"
	AcaoDesconhecido = "desconhecido"
)

type propostaItem struct {
	Tipo               *string  `json:"tipo"` // "habito" | "compromisso" | null
	Titulo             *string  `json:"titulo"`
	Notas              *string  `json:"notas"`
	RegraTipo          *string  `json:"regra_tipo"`           // "diario"|"dias_semana"|"vezes_periodo"|"intervalo"|null
	RegraDias          []int    `json:"regra_dias"`           // dias_semana: 1..7
	RegraVezes         *int     `json:"regra_vezes"`          // vezes_periodo
	RegraPeriodo       *string  `json:"regra_periodo"`        // "semana"|"mes"|null
	RegraIntervaloDias *int     `json:"regra_intervalo_dias"` // intervalo
	Dia                *string  `json:"dia"`                  // "YYYY-MM-DD"; compromisso
	Hora               *string  `json:"hora"`                 // "HH:MM"; compromisso
	Horarios           []string `json:"horarios"`             // "HH:MM" (lembrete) ou "-15m"/"-2h"/"-1d" (antecedência)
}

type propostaRegistro struct {
	ItemID *int64  `json:"item_id"`
	Titulo *string `json:"titulo"` // título falado — fallback se item_id não bater no contexto
	Dia    *string `json:"dia"`    // "YYYY-MM-DD"; null = hoje/agora
	Nota   *string `json:"nota"`
}

type propostaCompras struct {
	ListaNome *string              `json:"lista_nome"`
	Itens     []propostaItemCompra `json:"itens"`
}

type propostaItemCompra struct {
	Nome       string  `json:"nome"`
	Quantidade *string `json:"quantidade"`
}
