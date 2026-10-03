package captura

import (
	"context"
	"time"
)

// sleepFunc espera d respeitando ctx — injetável nos testes (mesmo desenho
// de servicos/smaug/internal/transcricao/groq.go).
type sleepFunc func(ctx context.Context, d time.Duration) error

func defaultSleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// retryableStatus: 429 ou 5xx — a única tentativa extra do usuário
// esperando com o celular na mão (§4: "1 nova tentativa em 429/5xx/rede,
// depois erro").
func retryableStatus(status int) bool {
	switch status {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

// backoffRetry é a espera única entre a 1ª tentativa e a extra — curta de
// propósito (a meta é responder em ≤ 5s do fim da fala, §4).
const backoffRetry = 500 * time.Millisecond
