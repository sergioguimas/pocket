package store

import (
	"context"
	"database/sql"
	"fmt"

	"pocket/internal/domain"
)

const secaoColunas = `id, lista_id, nome, ordem`

func scanSecao(row interface{ Scan(...any) error }) (domain.Secao, error) {
	var sc domain.Secao
	if err := row.Scan(&sc.ID, &sc.ListaID, &sc.Nome, &sc.Ordem); err != nil {
		return domain.Secao{}, err
	}
	return sc, nil
}

func secaoBuscar(ctx context.Context, q querier, id int64) (domain.Secao, error) {
	row := q.QueryRowContext(ctx, `SELECT `+secaoColunas+` FROM secao WHERE id = ?`, id)
	sc, err := scanSecao(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Secao{}, ErrNaoEncontrado{Entidade: "secao", ID: id}
		}
		return domain.Secao{}, fmt.Errorf("buscar secao %d: %w", id, err)
	}
	return sc, nil
}

// secoesPorLista lê as seções de listaID em ordem (ordem, id) — §2.2.
func secoesPorLista(ctx context.Context, q querier, listaID int64) ([]domain.Secao, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+secaoColunas+` FROM secao WHERE lista_id = ? ORDER BY ordem, id`, listaID)
	if err != nil {
		return nil, fmt.Errorf("listar secoes da lista %d: %w", listaID, err)
	}
	defer rows.Close()

	var out []domain.Secao
	for rows.Next() {
		sc, err := scanSecao(rows)
		if err != nil {
			return nil, fmt.Errorf("ler secao listada: %w", err)
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

func secaoInserir(ctx context.Context, q querier, sc domain.Secao) (int64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO secao (lista_id, nome, ordem) VALUES (?, ?, ?)`,
		sc.ListaID, sc.Nome, sc.Ordem)
	if err != nil {
		return 0, fmt.Errorf("inserir secao (lista %d): %w", sc.ListaID, err)
	}
	return res.LastInsertId()
}

func secaoAtualizar(ctx context.Context, q querier, sc domain.Secao) error {
	res, err := q.ExecContext(ctx, `UPDATE secao SET nome = ?, ordem = ? WHERE id = ?`, sc.Nome, sc.Ordem, sc.ID)
	if err != nil {
		return fmt.Errorf("atualizar secao %d: %w", sc.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirmar atualização da secao %d: %w", sc.ID, err)
	}
	if n == 0 {
		return ErrNaoEncontrado{Entidade: "secao", ID: sc.ID}
	}
	return nil
}

// secaoApagar apaga a seção id — quem chama (app) já moveu os itens dela
// para outra seção antes (§2.2: "apagar uma seção move os itens para a
// primeira restante").
func secaoApagar(ctx context.Context, q querier, id int64) error {
	res, err := q.ExecContext(ctx, `DELETE FROM secao WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("apagar secao %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirmar remoção da secao %d: %w", id, err)
	}
	if n == 0 {
		return ErrNaoEncontrado{Entidade: "secao", ID: id}
	}
	return nil
}

// Store — leituras diretas.

func (s *Store) SecaoBuscar(ctx context.Context, id int64) (domain.Secao, error) {
	return secaoBuscar(ctx, s.db, id)
}

func (s *Store) SecoesPorLista(ctx context.Context, listaID int64) ([]domain.Secao, error) {
	return secoesPorLista(ctx, s.db, listaID)
}

// Tx — leituras/escritas dentro da transação.

func (t *Tx) SecaoBuscar(ctx context.Context, id int64) (domain.Secao, error) {
	return secaoBuscar(ctx, t.tx, id)
}

func (t *Tx) SecoesPorLista(ctx context.Context, listaID int64) ([]domain.Secao, error) {
	return secoesPorLista(ctx, t.tx, listaID)
}

func (t *Tx) SecaoInserir(ctx context.Context, sc domain.Secao) (int64, error) {
	return secaoInserir(ctx, t.tx, sc)
}

func (t *Tx) SecaoAtualizar(ctx context.Context, sc domain.Secao) error {
	return secaoAtualizar(ctx, t.tx, sc)
}

func (t *Tx) SecaoApagar(ctx context.Context, id int64) error {
	return secaoApagar(ctx, t.tx, id)
}
