package ops

import (
	"context"
	"os/exec"
	"sync"
	"time"
)

// Executor roda um comando externo com timeout — usado pelo job renovar_cert
// (CONTRATO §7) para chamar o acme.sh. Interface para os testes usarem
// ExecutorFake (regra da casa: sem exec real nos testes).
type Executor interface {
	// Executar roda nome com args, com um timeout próprio (derivado de ctx —
	// se ctx já estiver cancelado, o comando nem chega a rodar). Devolve a
	// saída combinada (stdout+stderr) sempre que houver, e um erro se o
	// comando não terminar com sucesso (exit != 0, timeout, ou erro de
	// sistema ao iniciar).
	Executar(ctx context.Context, timeout time.Duration, nome string, args ...string) (saida string, err error)
}

// ExecutorReal roda o comando de verdade via os/exec.
type ExecutorReal struct{}

// Executar implementa Executor com exec.CommandContext, cancelando o
// processo se o timeout estourar.
func (ExecutorReal) Executar(ctx context.Context, timeout time.Duration, nome string, args ...string) (string, error) {
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, nome, args...)
	saida, err := cmd.CombinedOutput()
	return string(saida), err
}

// ChamadaExec é uma chamada registrada por ExecutorFake.
type ChamadaExec struct {
	Timeout time.Duration
	Nome    string
	Args    []string
}

// ExecutorFake grava as chamadas e devolve saída/erro roteirizados pelos
// testes (regra da casa: sem exec real nos testes). Por padrão devolve
// ("ok", nil); Resultado (se definido) decide por chamada.
type ExecutorFake struct {
	mu        sync.Mutex
	Chamadas  []ChamadaExec
	Resultado func(nome string, args ...string) (saida string, err error)
}

// Executar implementa Executor gravando a chamada e delegando a Resultado.
func (f *ExecutorFake) Executar(_ context.Context, timeout time.Duration, nome string, args ...string) (string, error) {
	f.mu.Lock()
	f.Chamadas = append(f.Chamadas, ChamadaExec{Timeout: timeout, Nome: nome, Args: append([]string(nil), args...)})
	resultado := f.Resultado
	f.mu.Unlock()

	if resultado != nil {
		return resultado(nome, args...)
	}
	return "ok", nil
}

// Contagem devolve quantas chamadas foram registradas (helper de teste).
func (f *ExecutorFake) Contagem() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Chamadas)
}
