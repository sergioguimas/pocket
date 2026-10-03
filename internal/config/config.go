// Package config lê a configuração do pocket do ambiente (.env ou variáveis
// reais), no mesmo padrão de servicos/smaug/internal/config: uma função de
// lookup injetável, para os testes não tocarem em variáveis de ambiente
// reais. Variáveis em CONTRATO.md §8.
package config

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"time"

	_ "time/tzdata" // Android não tem /etc/localtime; sem isso POCKET_TZ falha
)

// Config reúne tudo que o pocket lê do ambiente.
type Config struct {
	Listen      string
	DB          string
	Secrets     string
	TZ          string
	Location    *time.Location
	ACME        string // caminho do acme.sh; vazio (definido assim) desliga a renovação
	PushSubject string

	// Captura (fase 1.5, CONTRATO-FASE15.md §7): GroqAPIKey vazia desliga a
	// captura por voz/texto inteira (GET /api/captura → ativa:false) — a
	// mesma chave do Smaug (V4: a cota do Groq é por organização, não por
	// chave). CapturaModelo/TranscricaoModelo têm default, mas ficam
	// configuráveis para trocar de modelo (ex. gpt-oss-20b) sem rebuild.
	GroqAPIKey        string
	CapturaModelo     string
	TranscricaoModelo string
	// GroqURL é a base da API OpenAI-compatível do Groq — só muda no dev,
	// para apontar para um Groq falso local e testar a tela sem gastar cota.
	GroqURL string
}

// lookupFunc é o formato de os.LookupEnv; existe para permitir testar
// LoadConfigFrom sem tocar em variáveis de ambiente reais.
type lookupFunc func(key string) (string, bool)

// getenvOr devolve o valor de key se definido e não-vazio, senão def. Usada
// nas variáveis em que "definida vazia" e "não definida" significam a mesma
// coisa (todas exceto POCKET_ACME, que trata isso à parte — ver §8 e a nota
// do build).
func getenvOr(lookup lookupFunc, key, def string) string {
	if v, ok := lookup(key); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return def
}

// LoadConfigFrom monta a Config a partir de uma fonte arbitrária de
// variáveis (produzida em cima de os.LookupEnv em produção, ou de um map
// fixo nos testes).
func LoadConfigFrom(lookup lookupFunc) (*Config, error) {
	cfg := &Config{
		Listen:      getenvOr(lookup, "POCKET_LISTEN", "100.64.0.10:9104"),
		DB:          getenvOr(lookup, "POCKET_DB", "data/pocket.db"),
		Secrets:     getenvOr(lookup, "POCKET_SECRETS", "secrets"),
		TZ:          getenvOr(lookup, "POCKET_TZ", "America/Sao_Paulo"),
		PushSubject: getenvOr(lookup, "POCKET_PUSH_SUBJECT", "https://pocket.example.com"),

		// GROQ_API_KEY (sem prefixo POCKET_, igual ao Smaug — V4 do
		// contrato) vazia = captura desligada; getenvOr trata "ausente" e
		// "definida vazia" como a mesma coisa, que é exatamente o
		// comportamento pedido (§7).
		GroqAPIKey:        getenvOr(lookup, "GROQ_API_KEY", ""),
		CapturaModelo:     getenvOr(lookup, "POCKET_CAPTURA_MODELO", "openai/gpt-oss-120b"),
		TranscricaoModelo: getenvOr(lookup, "POCKET_TRANSCRICAO_MODELO", "whisper-large-v3-turbo"),
		GroqURL:           strings.TrimRight(getenvOr(lookup, "POCKET_GROQ_URL", "https://api.groq.com/openai/v1"), "/"),
	}

	if err := ValidateListen(cfg.Listen); err != nil {
		return nil, err
	}

	loc, err := time.LoadLocation(cfg.TZ)
	if err != nil {
		return nil, fmt.Errorf("POCKET_TZ inválido (%q): %w", cfg.TZ, err)
	}
	cfg.Location = loc

	// POCKET_ACME: "não definida" (default ~/.acme.sh/acme.sh) e "definida
	// vazia" (renovação desligada, ex.: dev no PC) são coisas diferentes —
	// getenvOr não serve aqui, ela trataria as duas como default.
	if raw, ok := lookup("POCKET_ACME"); ok {
		cfg.ACME = expandHome(strings.TrimSpace(raw), lookup)
	} else {
		cfg.ACME = expandHome("~/.acme.sh/acme.sh", lookup)
	}

	return cfg, nil
}

// expandHome troca um "~/" inicial pelo valor de HOME (lido pelo mesmo
// lookup, para os testes controlarem sem tocar no ambiente real). Sem HOME
// disponível, devolve o caminho como veio — melhor um erro claro depois ao
// tentar abrir o arquivo do que adivinhar.
func expandHome(path string, lookup lookupFunc) string {
	if path == "" || !strings.HasPrefix(path, "~/") {
		return path
	}
	if home, ok := lookup("HOME"); ok && home != "" {
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}

// ValidateListen recusa endereços que exporiam o Pocket fora da tailnet
// (CONTRATO §1: "não escuta em 0.0.0.0"): vazio, sem host ou 0.0.0.0/::.
func ValidateListen(addr string) error {
	if strings.TrimSpace(addr) == "" {
		return fmt.Errorf("POCKET_LISTEN vazio: recusando subir sem saber onde escutar")
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("POCKET_LISTEN inválido (%q): %w", addr, err)
	}
	if host == "" {
		return fmt.Errorf("POCKET_LISTEN sem host (%q): recusando — seria acessível de qualquer interface", addr)
	}
	if host == "0.0.0.0" || host == "::" {
		return fmt.Errorf("POCKET_LISTEN=%q: nunca 0.0.0.0 — escute só em loopback ou na tailnet", addr)
	}
	return nil
}
