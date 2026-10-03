package domain

import "time"

// Desfechos possíveis de uma captura (schema §5, coluna captura.desfecho —
// CONTRATO-FASE15.md §3 POST /api/captura/{id}/desfecho).
const (
	DesfechoConfirmada = "confirmada"
	DesfechoAjustada   = "ajustada"
	DesfechoDescartada = "descartada"
)

// Origens de uma captura (schema §5, coluna captura.origem).
const (
	OrigemCapturaVoz   = "voz"
	OrigemCapturaTexto = "texto"
)

// Captura é uma chamada de POST /api/captura já registrada (CONTRATO-
// FASE15.md §5): o áudio nunca é guardado, só o texto (transcrito ou
// digitado) e a resposta que o front recebeu (ou o erro, se o Groq falhou).
// Serve como matéria-prima para ajustar o prompt com erros reais (V5).
type Captura struct {
	ID         int64
	Em         time.Time
	Origem     string
	Texto      string
	Resposta   string // JSON devolvido ao front; "" se Erro != ""
	Erro       string // "" quando a captura funcionou
	Modelo     string
	LatenciaMs int
	Desfecho   string // "" até o front avisar (confirmada/ajustada/descartada)
}
