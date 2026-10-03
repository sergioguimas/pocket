package push

import "sync"

// Envio é um registro de chamada a Fake.Enviar — para os testes checarem o
// que foi (ou não) mandado, sem rede.
type Envio struct {
	Sub     Subscription
	Payload []byte
}

// Fake é o Pusher usado nos testes (regra da casa: sem rede). Por padrão
// devolve 201 sem erro; Resultado (se definido) decide o status/erro por
// chamada — para os testes simularem 410 (inscrição morta) ou erro de rede.
type Fake struct {
	mu        sync.Mutex
	Envios    []Envio
	Resultado func(sub Subscription, payload []byte) (status int, err error)
}

func (f *Fake) Enviar(sub Subscription, payload []byte) (status int, err error) {
	f.mu.Lock()
	f.Envios = append(f.Envios, Envio{Sub: sub, Payload: payload})
	resultado := f.Resultado
	f.mu.Unlock()

	if resultado != nil {
		return resultado(sub, payload)
	}
	return 201, nil
}

// Contagem devolve quantos envios foram registrados (helper de teste).
func (f *Fake) Contagem() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Envios)
}
