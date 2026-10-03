package captura

// schemaProposta é o JSON Schema (strict, formato Groq/OpenAI) enviado em
// response_format — espelha PropostaModelo campo a campo (§4). Regras do
// modo strict do Groq (testadas na prática, não só documentadas):
//   - toda propriedade de todo objeto tem que aparecer em "required" (não
//     existe "opcional" de verdade — só nullable via type:[T,"null"]);
//   - todo objeto precisa de "additionalProperties": false;
//   - nada de "anyOf"/"oneOf" nem objeto nullable — por isso item/registro/
//     compras são sempre objetos presentes com campos escalares nullable
//     (ver a DECISÃO em modelo.go).
var schemaProposta = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"acao": map[string]any{
			"type": "string",
			"enum": []string{AcaoCriarItem, AcaoRegistrar, AcaoCompras, AcaoDesconhecido},
		},
		"item":     schemaItem,
		"registro": schemaRegistro,
		"compras":  schemaCompras,
		"resto":    map[string]any{"type": []string{"string", "null"}},
	},
	"required":             []string{"acao", "item", "registro", "compras", "resto"},
	"additionalProperties": false,
}

var schemaItem = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"tipo":                 map[string]any{"type": []string{"string", "null"}, "enum": []any{"habito", "compromisso", nil}},
		"titulo":               map[string]any{"type": []string{"string", "null"}},
		"notas":                map[string]any{"type": []string{"string", "null"}},
		"regra_tipo":           map[string]any{"type": []string{"string", "null"}, "enum": []any{"diario", "dias_semana", "vezes_periodo", "intervalo", nil}},
		"regra_dias":           map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
		"regra_vezes":          map[string]any{"type": []string{"integer", "null"}},
		"regra_periodo":        map[string]any{"type": []string{"string", "null"}, "enum": []any{"semana", "mes", nil}},
		"regra_intervalo_dias": map[string]any{"type": []string{"integer", "null"}},
		"dia":                  map[string]any{"type": []string{"string", "null"}, "description": "YYYY-MM-DD"},
		"hora":                 map[string]any{"type": []string{"string", "null"}, "description": "HH:MM"},
		"horarios":             map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	},
	"required": []string{
		"tipo", "titulo", "notas", "regra_tipo", "regra_dias", "regra_vezes",
		"regra_periodo", "regra_intervalo_dias", "dia", "hora", "horarios",
	},
	"additionalProperties": false,
}

var schemaRegistro = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"item_id": map[string]any{"type": []string{"integer", "null"}},
		"titulo":  map[string]any{"type": []string{"string", "null"}},
		"dia":     map[string]any{"type": []string{"string", "null"}, "description": "YYYY-MM-DD; null = hoje/agora"},
		"nota":    map[string]any{"type": []string{"string", "null"}},
	},
	"required":             []string{"item_id", "titulo", "dia", "nota"},
	"additionalProperties": false,
}

var schemaCompras = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"lista_nome": map[string]any{"type": []string{"string", "null"}},
		"itens": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"nome":       map[string]any{"type": "string"},
					"quantidade": map[string]any{"type": []string{"string", "null"}},
				},
				"required":             []string{"nome", "quantidade"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"lista_nome", "itens"},
	"additionalProperties": false,
}

// responseFormatJSONSchema monta o objeto response_format (§4: strict:true).
func responseFormatJSONSchema() map[string]any {
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "proposta",
			"strict": true,
			"schema": schemaProposta,
		},
	}
}
