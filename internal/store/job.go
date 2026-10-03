package store

import (
	"context"
	"fmt"
)

// jobExecutado diz se nome já rodou com sucesso para dataRef (mesmo padrão
// do Smaug: PRIMARY KEY (nome, data_ref) garante idempotência mesmo sob
// corrida, já que a segunda inserção falha por violação de unicidade).
func jobExecutado(ctx context.Context, q querier, nome, dataRef string) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM job WHERE nome = ? AND data_ref = ?`, nome, dataRef).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("checar job %s/%s: %w", nome, dataRef, err)
	}
	return n > 0, nil
}

// jobMarcar registra nome como executado em dataRef, no instante em (já
// formatado por domain.FormatInstante por quem chama). Chamar depois que o
// trabalho do job terminou com sucesso — nunca antes.
func jobMarcar(ctx context.Context, q querier, nome, dataRef, em string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO job (nome, data_ref, em) VALUES (?, ?, ?)`, nome, dataRef, em)
	if err != nil {
		return fmt.Errorf("marcar job %s/%s: %w", nome, dataRef, err)
	}
	return nil
}

func (s *Store) JobExecutado(ctx context.Context, nome, dataRef string) (bool, error) {
	return jobExecutado(ctx, s.db, nome, dataRef)
}

func (t *Tx) JobExecutado(ctx context.Context, nome, dataRef string) (bool, error) {
	return jobExecutado(ctx, t.tx, nome, dataRef)
}

func (t *Tx) JobMarcar(ctx context.Context, nome, dataRef, em string) error {
	return jobMarcar(ctx, t.tx, nome, dataRef, em)
}
