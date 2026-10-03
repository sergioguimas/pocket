package store

import (
	"context"
	"database/sql"
	"fmt"

	"pocket/internal/domain"
)

const capturaColunas = `id, em, origem, texto, resposta, erro, modelo, latencia_ms, desfecho`

func scanCaptura(row interface{ Scan(...any) error }) (domain.Captura, error) {
	var (
		c        domain.Captura
		em       string
		resposta sql.NullString
		erro     sql.NullString
		desfecho sql.NullString
	)
	if err := row.Scan(&c.ID, &em, &c.Origem, &c.Texto, &resposta, &erro, &c.Modelo, &c.LatenciaMs, &desfecho); err != nil {
		return domain.Captura{}, err
	}
	t, err := domain.ParseInstante(em)
	if err != nil {
		return domain.Captura{}, fmt.Errorf("em gravado inválido (captura %d): %w", c.ID, err)
	}
	c.Em = t
	c.Resposta = resposta.String
	c.Erro = erro.String
	c.Desfecho = desfecho.String
	return c, nil
}

// capturaInserir grava uma linha de captura (CONTRATO-FASE15.md §5) e
// devolve o id gerado. Chamado tanto no caminho de sucesso (resposta
// preenchida, erro vazio) quanto no de falha do Groq (erro preenchido,
// resposta vazia) — a persistência acontece nos dois casos (§4: "app...
// persiste uma linha `captura`... também em erros do provider").
func capturaInserir(ctx context.Context, q querier, c domain.Captura) (int64, error) {
	res, err := q.ExecContext(ctx, `
		INSERT INTO captura (em, origem, texto, resposta, erro, modelo, latencia_ms, desfecho)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		domain.FormatInstante(c.Em), c.Origem, c.Texto, nullIfVazio(c.Resposta), nullIfVazio(c.Erro),
		c.Modelo, c.LatenciaMs, nullIfVazio(c.Desfecho))
	if err != nil {
		return 0, fmt.Errorf("inserir captura: %w", err)
	}
	return res.LastInsertId()
}

// capturaDefinirDesfecho grava o desfecho (confirmada/ajustada/descartada)
// de uma captura já existente (POST /api/captura/{id}/desfecho, §3). Id
// inexistente devolve ErrNaoEncontrado.
func capturaDefinirDesfecho(ctx context.Context, q querier, id int64, desfecho string) error {
	res, err := q.ExecContext(ctx, `UPDATE captura SET desfecho = ? WHERE id = ?`, desfecho, id)
	if err != nil {
		return fmt.Errorf("definir desfecho da captura %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirmar desfecho da captura %d: %w", id, err)
	}
	if n == 0 {
		return ErrNaoEncontrado{Entidade: "captura", ID: id}
	}
	return nil
}

// capturaPodar mantém só as maxLinhas mais recentes (por id — mesma ordem
// de em, já que id é sequencial) e apaga o resto (§5: "Poda: o job diário
// mantém as 500 mais recentes"). Devolve quantas linhas foram removidas.
func capturaPodar(ctx context.Context, q querier, maxLinhas int) (int64, error) {
	res, err := q.ExecContext(ctx, `
		DELETE FROM captura WHERE id NOT IN (
			SELECT id FROM captura ORDER BY id DESC LIMIT ?
		)`, maxLinhas)
	if err != nil {
		return 0, fmt.Errorf("podar captura: %w", err)
	}
	return res.RowsAffected()
}

func capturaBuscar(ctx context.Context, q querier, id int64) (domain.Captura, error) {
	row := q.QueryRowContext(ctx, `SELECT `+capturaColunas+` FROM captura WHERE id = ?`, id)
	c, err := scanCaptura(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Captura{}, ErrNaoEncontrado{Entidade: "captura", ID: id}
		}
		return domain.Captura{}, fmt.Errorf("buscar captura %d: %w", id, err)
	}
	return c, nil
}

// Store — leituras diretas / escrita fora de transação (captura não precisa
// de atomicidade com mais nada — é uma linha só, sem ocorrência/observador
// envolvido).

func (s *Store) CapturaInserir(ctx context.Context, c domain.Captura) (int64, error) {
	return capturaInserir(ctx, s.db, c)
}

func (s *Store) CapturaBuscar(ctx context.Context, id int64) (domain.Captura, error) {
	return capturaBuscar(ctx, s.db, id)
}

func (s *Store) CapturaDefinirDesfecho(ctx context.Context, id int64, desfecho string) error {
	return capturaDefinirDesfecho(ctx, s.db, id, desfecho)
}

func (s *Store) CapturaPodar(ctx context.Context, maxLinhas int) (int64, error) {
	return capturaPodar(ctx, s.db, maxLinhas)
}

func (t *Tx) CapturaPodar(ctx context.Context, maxLinhas int) (int64, error) {
	return capturaPodar(ctx, t.tx, maxLinhas)
}
