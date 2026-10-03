// Package clock isola "agora" atrás de uma interface: todo o resto do
// serviço recebe um Clock em vez de chamar time.Now() direto, o que permite
// os testes controlarem o tempo (avanço de dia, atraso de lembrete, etc.).
// Regra da casa: nenhum time.Now() fora deste pacote e de main.go.
package clock

import (
	"sync"
	"time"
)

// Clock devolve o instante atual.
type Clock interface {
	Now() time.Time
}

// Real usa o relógio do sistema — a implementação de produção.
type Real struct{}

// Now devolve time.Now().
func (Real) Now() time.Time { return time.Now() }

// Fake é um relógio controlável pelos testes: só muda quando Advance ou Set
// são chamados, nunca sozinho.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

// NewFake cria um relógio falso começando em inicio.
func NewFake(inicio time.Time) *Fake {
	return &Fake{now: inicio}
}

// Now devolve o instante atual do relógio falso.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Advance avança o relógio falso por d (aceita d negativo, se algum teste precisar voltar).
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// Set trava o relógio falso num instante específico.
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t
}
