package push

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// VAPIDKeys espelha literalmente o formato de secrets/vapid.json do spike
// da fase 0 ({"Public","Private"}) — preservado de propósito (CONTRATO §7)
// para a inscrição já feita no Poco sobreviver ao deploy da fase 1.
type VAPIDKeys struct {
	Public  string
	Private string
}

// CarregarOuGerarVAPID lê <secretsDir>/vapid.json; se o arquivo não existir,
// gera um par novo e grava (mesmo formato do spike — ver
// git show master:servicos/pocket/main.go, tipo "vapid").
func CarregarOuGerarVAPID(secretsDir string) (VAPIDKeys, error) {
	path := filepath.Join(secretsDir, "vapid.json")

	b, err := os.ReadFile(path)
	if err == nil {
		var v VAPIDKeys
		if err := json.Unmarshal(b, &v); err != nil {
			return VAPIDKeys{}, fmt.Errorf("vapid.json inválido (%s): %w", path, err)
		}
		if v.Public == "" || v.Private == "" {
			return VAPIDKeys{}, fmt.Errorf("vapid.json incompleto (%s): faltam Public/Private", path)
		}
		return v, nil
	}
	if !os.IsNotExist(err) {
		return VAPIDKeys{}, fmt.Errorf("ler vapid.json (%s): %w", path, err)
	}

	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return VAPIDKeys{}, fmt.Errorf("gerar chaves VAPID: %w", err)
	}
	v := VAPIDKeys{Public: pub, Private: priv}
	if err := gravarVAPID(path, v); err != nil {
		return VAPIDKeys{}, err
	}
	return v, nil
}

// gravarVAPID grava v em path (write-then-rename, como o resto do sistema —
// evita um arquivo pela metade se o processo morrer no meio da escrita).
func gravarVAPID(path string, v VAPIDKeys) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("criar diretório de secrets: %w", err)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("serializar vapid.json: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("gravar %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("renomear %s: %w", tmp, err)
	}
	return nil
}
