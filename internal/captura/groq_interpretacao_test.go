package captura

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func contextoTeste() Contexto {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	return Contexto{
		Agora:   time.Date(2026, 9, 28, 10, 0, 0, 0, loc),
		Loc:     loc,
		Habitos: []ContextoHabito{{ID: 1, Titulo: "Devocional"}},
		Listas:  []ContextoLista{{ID: 1, Nome: "Mercado"}},
	}
}

func TestGroqInterpretadorSucesso(t *testing.T) {
	var recebido chatRequisicao
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer gsk_teste" {
			t.Errorf("Authorization = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &recebido); err != nil {
			t.Fatalf("decodificar corpo: %v", err)
		}
		resp := chatRespostaTeste(`{"acao":"desconhecido","item":{"tipo":null,"titulo":null,"notas":null,"regra_tipo":null,"regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":[]},"registro":{"item_id":null,"titulo":null,"dia":null,"nota":null},"compras":{"lista_nome":null,"itens":[]},"resto":null}`, 100, 20)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	g := &GroqInterpretador{APIKey: "gsk_teste", Modelo: "openai/gpt-oss-120b", URL: srv.URL, HTTPClient: srv.Client()}
	conteudo, uso, err := g.Interpretar(context.Background(), contextoTeste(), "hmm deixa eu ver")
	if err != nil {
		t.Fatalf("Interpretar: %v", err)
	}
	if uso.PromptTokens != 100 || uso.CompletionTokens != 20 {
		t.Fatalf("uso = %+v", uso)
	}
	var proposta PropostaModelo
	if err := json.Unmarshal(conteudo, &proposta); err != nil {
		t.Fatalf("conteudo não é JSON válido: %v (%s)", err, conteudo)
	}
	if proposta.Acao != AcaoDesconhecido {
		t.Fatalf("Acao = %q", proposta.Acao)
	}

	if recebido.Temperature != 0 {
		t.Errorf("Temperature = %v, want 0", recebido.Temperature)
	}
	if recebido.ReasoningEffort != "low" {
		t.Errorf("ReasoningEffort = %q, want low", recebido.ReasoningEffort)
	}
	if recebido.Model != "openai/gpt-oss-120b" {
		t.Errorf("Model = %q", recebido.Model)
	}
	if recebido.ResponseFormat["type"] != "json_schema" {
		t.Errorf("ResponseFormat.type = %v, want json_schema", recebido.ResponseFormat["type"])
	}
	if len(recebido.Messages) != 2 || recebido.Messages[1].Content != "hmm deixa eu ver" {
		t.Fatalf("Messages = %+v", recebido.Messages)
	}
}

func TestGroqInterpretadorRetentaUmaVezEm503(t *testing.T) {
	var chamadas int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&chamadas, 1)
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		resp := chatRespostaTeste(`{"acao":"desconhecido","item":{"tipo":null,"titulo":null,"notas":null,"regra_tipo":null,"regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":[]},"registro":{"item_id":null,"titulo":null,"dia":null,"nota":null},"compras":{"lista_nome":null,"itens":[]},"resto":null}`, 1, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	g := &GroqInterpretador{APIKey: "k", Modelo: "m", URL: srv.URL, HTTPClient: srv.Client(), sleep: semEspera}
	_, _, err := g.Interpretar(context.Background(), contextoTeste(), "x")
	if err != nil {
		t.Fatalf("Interpretar: %v", err)
	}
	if atomic.LoadInt32(&chamadas) != 2 {
		t.Fatalf("chamadas = %d, want 2", chamadas)
	}
}

func TestGroqInterpretadorFalhaFinal429ViraErrLimite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	g := &GroqInterpretador{APIKey: "k", Modelo: "m", URL: srv.URL, HTTPClient: srv.Client(), sleep: semEspera}
	_, _, err := g.Interpretar(context.Background(), contextoTeste(), "x")
	var errLimite ErrLimite
	if !errors.As(err, &errLimite) {
		t.Fatalf("err = %v, want ErrLimite", err)
	}
}

func TestGroqInterpretadorFallbackJSONObjectEm400DeSchema(t *testing.T) {
	var chamadas int32
	var formatosVistos []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&chamadas, 1)
		body, _ := io.ReadAll(r.Body)
		var req chatRequisicao
		json.Unmarshal(body, &req)
		formatosVistos = append(formatosVistos, fmtType(req.ResponseFormat))

		if n == 1 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":{"message":"response_format.json_schema is not supported for this model"}}`))
			return
		}
		resp := chatRespostaTeste(`{"acao":"desconhecido","item":{"tipo":null,"titulo":null,"notas":null,"regra_tipo":null,"regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":[]},"registro":{"item_id":null,"titulo":null,"dia":null,"nota":null},"compras":{"lista_nome":null,"itens":[]},"resto":null}`, 1, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	g := &GroqInterpretador{APIKey: "k", Modelo: "m", URL: srv.URL, HTTPClient: srv.Client(), sleep: semEspera}
	_, _, err := g.Interpretar(context.Background(), contextoTeste(), "x")
	if err != nil {
		t.Fatalf("Interpretar: %v", err)
	}
	if atomic.LoadInt32(&chamadas) != 2 {
		t.Fatalf("chamadas = %d, want 2", chamadas)
	}
	if formatosVistos[0] != "json_schema" || formatosVistos[1] != "json_object" {
		t.Fatalf("formatos = %v, want [json_schema json_object]", formatosVistos)
	}
}

func TestGroqInterpretadorFalha400SemMencionarSchemaNaoRetenta(t *testing.T) {
	var chamadas int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&chamadas, 1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer srv.Close()

	g := &GroqInterpretador{APIKey: "k", Modelo: "m", URL: srv.URL, HTTPClient: srv.Client(), sleep: semEspera}
	_, _, err := g.Interpretar(context.Background(), contextoTeste(), "x")
	var errProvedor ErrProvedor
	if !errors.As(err, &errProvedor) {
		t.Fatalf("err = %v, want ErrProvedor", err)
	}
	if atomic.LoadInt32(&chamadas) != 1 {
		t.Fatalf("chamadas = %d, want 1", chamadas)
	}
}

// --- pequenos helpers de teste (evitam importar time/strings a mais no arquivo principal) ---

func fmtType(m map[string]any) string {
	if m == nil {
		return ""
	}
	v, _ := m["type"].(string)
	return v
}

func chatRespostaTeste(content string, promptTokens, completionTokens int) map[string]any {
	return map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"content": content}},
		},
		"usage": map[string]any{
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
		},
	}
}
