package config

import (
	"path/filepath"
	"testing"
)

func lookupFrom(env map[string]string) lookupFunc {
	return func(k string) (string, bool) { v, ok := env[k]; return v, ok }
}

func TestLoadConfigFromDefaults(t *testing.T) {
	cfg, err := LoadConfigFrom(lookupFrom(map[string]string{}))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	cases := []struct {
		nome string
		got  string
		want string
	}{
		{"Listen", cfg.Listen, "100.64.0.10:9104"},
		{"DB", cfg.DB, "data/pocket.db"},
		{"Secrets", cfg.Secrets, "secrets"},
		{"TZ", cfg.TZ, "America/Sao_Paulo"},
		{"PushSubject", cfg.PushSubject, "https://pocket.example.com"},
		{"GroqAPIKey", cfg.GroqAPIKey, ""},
		{"CapturaModelo", cfg.CapturaModelo, "openai/gpt-oss-120b"},
		{"TranscricaoModelo", cfg.TranscricaoModelo, "whisper-large-v3-turbo"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s default = %q, want %q", c.nome, c.got, c.want)
		}
	}
	if cfg.Location == nil {
		t.Fatal("Location não deveria ser nil")
	}
	// ACME não definida: default expandido com HOME (ausente no lookup aqui,
	// então fica com "~/" literal mesmo — só testamos o caso com HOME abaixo).
	if cfg.ACME != "~/.acme.sh/acme.sh" {
		t.Errorf("ACME default sem HOME = %q, want %q", cfg.ACME, "~/.acme.sh/acme.sh")
	}
}

func TestLoadConfigFromListenProibido(t *testing.T) {
	cases := []struct {
		nome   string
		listen string
		valid  bool
	}{
		{"loopback", "127.0.0.1:9104", true},
		{"tailnet", "100.64.0.10:9104", true},
		{"vazio explicito usa default", "", true}, // getenvOr cai no default
		{"0.0.0.0", "0.0.0.0:9104", false},
		{"ipv6 wildcard", "[::]:9104", false},
		{"sem host", ":9104", false},
		{"sem porta", "127.0.0.1", false},
	}
	for _, c := range cases {
		t.Run(c.nome, func(t *testing.T) {
			env := map[string]string{}
			if c.listen != "" {
				env["POCKET_LISTEN"] = c.listen
			}
			_, err := LoadConfigFrom(lookupFrom(env))
			if c.valid && err != nil {
				t.Errorf("POCKET_LISTEN=%q devia ser válido, deu erro: %v", c.listen, err)
			}
			if !c.valid && err == nil {
				t.Errorf("POCKET_LISTEN=%q devia ser inválido e não deu erro", c.listen)
			}
		})
	}
}

func TestValidateListenDireto(t *testing.T) {
	valid := []string{"127.0.0.1:9104", "100.64.0.10:9104", "localhost:9104"}
	for _, addr := range valid {
		if err := ValidateListen(addr); err != nil {
			t.Errorf("ValidateListen(%q) devia passar, deu erro: %v", addr, err)
		}
	}
	invalid := []string{"", "0.0.0.0:9104", "[::]:9104", ":9104", "sem-porta"}
	for _, addr := range invalid {
		if err := ValidateListen(addr); err == nil {
			t.Errorf("ValidateListen(%q) devia falhar e não falhou", addr)
		}
	}
}

func TestLoadConfigFromTZInvalida(t *testing.T) {
	_, err := LoadConfigFrom(lookupFrom(map[string]string{"POCKET_TZ": "Marte/Nao_Existe"}))
	if err == nil {
		t.Fatal("esperava erro por POCKET_TZ inválido")
	}
}

func TestLoadConfigFromTZValida(t *testing.T) {
	cfg, err := LoadConfigFrom(lookupFrom(map[string]string{"POCKET_TZ": "UTC"}))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if cfg.Location.String() != "UTC" {
		t.Errorf("Location = %v, want UTC", cfg.Location)
	}
}

func TestLoadConfigFromCaptura(t *testing.T) {
	t.Run("defaults com GROQ_API_KEY ausente = captura desligada", func(t *testing.T) {
		cfg, err := LoadConfigFrom(lookupFrom(map[string]string{}))
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if cfg.GroqAPIKey != "" {
			t.Errorf("GroqAPIKey = %q, want vazio", cfg.GroqAPIKey)
		}
	})

	t.Run("GROQ_API_KEY definida liga a captura", func(t *testing.T) {
		cfg, err := LoadConfigFrom(lookupFrom(map[string]string{"GROQ_API_KEY": "gsk_teste"}))
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if cfg.GroqAPIKey != "gsk_teste" {
			t.Errorf("GroqAPIKey = %q, want gsk_teste", cfg.GroqAPIKey)
		}
	})

	t.Run("modelos customizados", func(t *testing.T) {
		cfg, err := LoadConfigFrom(lookupFrom(map[string]string{
			"POCKET_CAPTURA_MODELO":     "openai/gpt-oss-20b",
			"POCKET_TRANSCRICAO_MODELO": "whisper-large-v3",
		}))
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if cfg.CapturaModelo != "openai/gpt-oss-20b" {
			t.Errorf("CapturaModelo = %q", cfg.CapturaModelo)
		}
		if cfg.TranscricaoModelo != "whisper-large-v3" {
			t.Errorf("TranscricaoModelo = %q", cfg.TranscricaoModelo)
		}
	})
}

func TestLoadConfigFromACME(t *testing.T) {
	t.Run("nao definida usa default expandido com HOME", func(t *testing.T) {
		cfg, err := LoadConfigFrom(lookupFrom(map[string]string{"HOME": "/home/u0"}))
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		// filepath.Join: no Windows (dev no PC) o separador é "\\"
		if want := filepath.Join("/home/u0", ".acme.sh", "acme.sh"); cfg.ACME != want {
			t.Errorf("ACME = %q, want %q", cfg.ACME, want)
		}
	})

	t.Run("definida vazia desliga a renovacao", func(t *testing.T) {
		cfg, err := LoadConfigFrom(lookupFrom(map[string]string{
			"HOME":        "/home/u0",
			"POCKET_ACME": "",
		}))
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if cfg.ACME != "" {
			t.Errorf("ACME = %q, want vazio (desligado)", cfg.ACME)
		}
	})

	t.Run("definida com caminho custom sem ~", func(t *testing.T) {
		cfg, err := LoadConfigFrom(lookupFrom(map[string]string{
			"POCKET_ACME": "/opt/acme/acme.sh",
		}))
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if cfg.ACME != "/opt/acme/acme.sh" {
			t.Errorf("ACME = %q, want %q", cfg.ACME, "/opt/acme/acme.sh")
		}
	})

	t.Run("default sem HOME no lookup mantem ~ literal", func(t *testing.T) {
		cfg, err := LoadConfigFrom(lookupFrom(map[string]string{}))
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if cfg.ACME != "~/.acme.sh/acme.sh" {
			t.Errorf("ACME = %q, want %q", cfg.ACME, "~/.acme.sh/acme.sh")
		}
	})
}
