package captura

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// timeoutInterpretacao é o prazo por chamada de interpretação (§4).
const timeoutInterpretacao = 15 * time.Second

const urlChatGroq = "https://api.groq.com/openai/v1/chat/completions"

// GroqInterpretador implementa Interpretador com o chat completions
// OpenAI-compatível do Groq (§4): response_format json_schema strict,
// temperature 0, reasoning_effort "low".
type GroqInterpretador struct {
	APIKey     string
	Modelo     string
	URL        string // "" usa urlChatGroq — só testes trocam
	HTTPClient *http.Client
	sleep      sleepFunc // nil usa defaultSleep; injetável no teste
}

var _ Interpretador = (*GroqInterpretador)(nil)

func NovoGroqInterpretador(apiKey, modelo string) *GroqInterpretador {
	return &GroqInterpretador{APIKey: apiKey, Modelo: modelo, HTTPClient: &http.Client{}}
}

type chatMensagem struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequisicao struct {
	Model           string         `json:"model"`
	Messages        []chatMensagem `json:"messages"`
	Temperature     float64        `json:"temperature"`
	ReasoningEffort string         `json:"reasoning_effort"`
	ResponseFormat  map[string]any `json:"response_format"`
}

type chatResposta struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// Interpretar monta o corpo (system = regras fixas + contexto; user =
// texto), autentica com Bearer e tenta de novo 1x em 429/5xx/erro de rede
// (§4).
//
// DECISÃO: se o Groq devolver 400 mencionando "response_format" ou
// "schema" (Groq às vezes recusa strict json_schema para um modelo/config
// específico), tenta 1x com response_format {"type":"json_object"} e o
// schema colado no prompt de sistema como instrução — a segunda tentativa
// do contrato ("retry once") não fala desse caso especificamente, mas
// §4/etapa 3 do build pede esse plano B; ela não consome o mesmo orçamento
// de "1 nova tentativa em 429/5xx" (são causas diferentes de falha), mas o
// número total de tentativas HTTP fica limitado a 3 para não estourar a
// meta de latência (≤ 5s) esperando o usuário com o celular na mão.
func (g *GroqInterpretador) Interpretar(ctx context.Context, contexto Contexto, texto string) ([]byte, Uso, error) {
	sleep := g.sleep
	if sleep == nil {
		sleep = defaultSleep
	}
	url := g.URL
	if url == "" {
		url = urlChatGroq
	}
	client := g.HTTPClient
	if client == nil {
		client = &http.Client{}
	}

	ctx, cancel := context.WithTimeout(ctx, timeoutInterpretacao)
	defer cancel()

	promptSistema := montarPromptSistema(contexto)

	usarJSONObject := false
	var lastErr error
	lastStatus := 0
	const maxTentativas = 3
	for tentativa := 1; tentativa <= maxTentativas; tentativa++ {
		body := montarCorpoInterpretacao(g.Modelo, promptSistema, texto, usarJSONObject)
		reqBody, err := json.Marshal(body)
		if err != nil {
			return nil, Uso{}, fmt.Errorf("interpretação: montar corpo: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
		if err != nil {
			return nil, Uso{}, fmt.Errorf("interpretação: montar request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+g.APIKey)

		resp, doErr := client.Do(req)
		if doErr != nil {
			lastErr = doErr
			lastStatus = 0
		} else {
			respBody, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			status := resp.StatusCode
			if readErr != nil {
				lastErr = readErr
				lastStatus = status
			} else if status >= 200 && status < 300 {
				conteudo, uso, perr := extrairConteudo(respBody)
				if perr == nil {
					return conteudo, uso, nil
				}
				lastErr = perr
				lastStatus = status
			} else {
				lastErr = fmt.Errorf("groq status %d: %s", status, truncar(string(respBody), 300))
				lastStatus = status
				if status == http.StatusBadRequest && !usarJSONObject && mencionaSchema(respBody) {
					usarJSONObject = true
					continue // plano B (ver DECISÃO acima) — não conta backoff.
				}
				if !retryableStatus(status) {
					return nil, Uso{}, ErrProvedor{Msg: fmt.Sprintf("interpretação: %v", lastErr)}
				}
			}
		}

		if tentativa == maxTentativas {
			break
		}
		if serr := sleep(ctx, backoffRetry); serr != nil {
			return nil, Uso{}, ErrProvedor{Msg: fmt.Sprintf("interpretação: %v", serr)}
		}
	}

	if lastStatus == http.StatusTooManyRequests {
		return nil, Uso{}, ErrLimite{Msg: "limite do Groq atingido — espere um minuto ou use o formulário"}
	}
	return nil, Uso{}, ErrProvedor{Msg: fmt.Sprintf("interpretação: groq indisponível: %v", lastErr)}
}

func montarCorpoInterpretacao(modelo, promptSistema, texto string, usarJSONObject bool) chatRequisicao {
	sistema := promptSistema
	var responseFormat map[string]any
	if usarJSONObject {
		sistema = sistema + "\n\nResponda só com um JSON válido, sem texto fora dele, seguindo " +
			"exatamente este schema (todo campo listado é obrigatório; null é um valor válido " +
			"para os campos nullable):\n\n" + schemaComoTexto()
		responseFormat = map[string]any{"type": "json_object"}
	} else {
		responseFormat = responseFormatJSONSchema()
	}

	return chatRequisicao{
		Model: modelo,
		Messages: []chatMensagem{
			{Role: "system", Content: sistema},
			{Role: "user", Content: texto},
		},
		Temperature:     0,
		ReasoningEffort: "low",
		ResponseFormat:  responseFormat,
	}
}

func schemaComoTexto() string {
	b, err := json.MarshalIndent(schemaProposta, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}

// mencionaSchema checa se o corpo de erro do Groq fala de response_format/
// schema (heurística simples — o Groq não tem um código de erro estável
// documentado para "esse modelo não suporta json_schema strict").
func mencionaSchema(body []byte) bool {
	s := strings.ToLower(string(body))
	return strings.Contains(s, "response_format") || strings.Contains(s, "json_schema") || strings.Contains(s, "schema")
}

// extrairConteudo lê o content da primeira choice (o JSON — ainda cru,
// string escapada dentro do content, formato padrão OpenAI/Groq) e o
// usage. Não decodifica o content como PropostaModelo — isso é trabalho de
// Normalizar (captura.go/normalizar.go), inclusive o caso de vir malformado.
func extrairConteudo(respBody []byte) ([]byte, Uso, error) {
	var cr chatResposta
	if err := json.Unmarshal(respBody, &cr); err != nil {
		return nil, Uso{}, fmt.Errorf("decodificar resposta do groq: %w", err)
	}
	if len(cr.Choices) == 0 {
		return nil, Uso{}, fmt.Errorf("resposta do groq sem choices")
	}
	uso := Uso{PromptTokens: cr.Usage.PromptTokens, CompletionTokens: cr.Usage.CompletionTokens}
	return []byte(cr.Choices[0].Message.Content), uso, nil
}
