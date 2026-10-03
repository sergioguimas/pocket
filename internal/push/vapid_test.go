package push

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestCarregarVapidFormatoDoSpikeSemRegenerar: um vapid.json já existente,
// no formato literal do spike ({"Public","Private"}), é lido sem gerar
// chaves novas nem reescrever o arquivo — é o que preserva a inscrição já
// feita no Poco através do deploy da fase 1 (CONTRATO §7).
func TestCarregarVapidFormatoDoSpikeSemRegenerar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vapid.json")
	original := VAPIDKeys{Public: "chave-publica-do-spike", Private: "chave-privada-do-spike"}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	antes, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	got, err := CarregarOuGerarVAPID(dir)
	if err != nil {
		t.Fatalf("CarregarOuGerarVAPID: %v", err)
	}
	if got != original {
		t.Fatalf("chaves = %+v, want %+v", got, original)
	}

	depois, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat depois: %v", err)
	}
	if !depois.ModTime().Equal(antes.ModTime()) || depois.Size() != antes.Size() {
		t.Fatalf("vapid.json foi reescrito (mtime/tamanho mudaram) — devia só ler")
	}

	b2, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b2) != string(b) {
		t.Fatalf("conteúdo do arquivo mudou:\nantes:  %s\ndepois: %s", b, b2)
	}
}

// TestGerarVapidQuandoAusente: sem vapid.json, gera um par novo e grava no
// formato do spike.
func TestGerarVapidQuandoAusente(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vapid.json")

	got, err := CarregarOuGerarVAPID(dir)
	if err != nil {
		t.Fatalf("CarregarOuGerarVAPID: %v", err)
	}
	if got.Public == "" || got.Private == "" {
		t.Fatalf("chaves geradas vazias: %+v", got)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("vapid.json não foi gravado: %v", err)
	}
	var gravado VAPIDKeys
	if err := json.Unmarshal(b, &gravado); err != nil {
		t.Fatalf("vapid.json gravado num formato inválido: %v", err)
	}
	if gravado != got {
		t.Fatalf("gravado = %+v, want %+v", gravado, got)
	}

	// chamar de novo lê o mesmo arquivo, sem gerar outro par.
	got2, err := CarregarOuGerarVAPID(dir)
	if err != nil {
		t.Fatalf("CarregarOuGerarVAPID (2ª vez): %v", err)
	}
	if got2 != got {
		t.Fatalf("2ª chamada gerou chaves diferentes: %+v != %+v", got2, got)
	}
}
