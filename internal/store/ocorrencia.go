package store

import (
	"context"
	"database/sql"
	"fmt"

	"pocket/internal/domain"
)

const ocorrenciaColunas = `id, item_id, inicio, fim, meta, status`

func scanOcorrencia(row interface{ Scan(...any) error }) (domain.Ocorrencia, error) {
	var (
		oc     domain.Ocorrencia
		inicio sql.NullString
		fim    sql.NullString
	)
	if err := row.Scan(&oc.ID, &oc.ItemID, &inicio, &fim, &oc.Meta, &oc.Status); err != nil {
		return domain.Ocorrencia{}, err
	}
	if inicio.Valid {
		d, err := domain.ParseDia(inicio.String)
		if err != nil {
			return domain.Ocorrencia{}, fmt.Errorf("inicio gravado inválido (ocorrencia %d): %w", oc.ID, err)
		}
		oc.Inicio = &d
	}
	if fim.Valid {
		d, err := domain.ParseDia(fim.String)
		if err != nil {
			return domain.Ocorrencia{}, fmt.Errorf("fim gravado inválido (ocorrencia %d): %w", oc.ID, err)
		}
		oc.Fim = &d
	}
	return oc, nil
}

func ocorrenciaBuscar(ctx context.Context, q querier, id int64) (domain.Ocorrencia, error) {
	row := q.QueryRowContext(ctx, `SELECT `+ocorrenciaColunas+` FROM ocorrencia WHERE id = ?`, id)
	oc, err := scanOcorrencia(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Ocorrencia{}, ErrNaoEncontrado{Entidade: "ocorrencia", ID: id}
		}
		return domain.Ocorrencia{}, fmt.Errorf("buscar ocorrencia %d: %w", id, err)
	}
	return oc, nil
}

// ocorrenciasPorItem lê todas as ocorrências de um item, ordenadas por
// inicio (NULLs — só compromisso sem data — primeiro; não afeta hábito,
// que sempre tem inicio).
func ocorrenciasPorItem(ctx context.Context, q querier, itemID int64) ([]domain.Ocorrencia, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+ocorrenciaColunas+` FROM ocorrencia WHERE item_id = ? ORDER BY inicio`, itemID)
	if err != nil {
		return nil, fmt.Errorf("listar ocorrencias do item %d: %w", itemID, err)
	}
	defer rows.Close()

	var out []domain.Ocorrencia
	for rows.Next() {
		oc, err := scanOcorrencia(rows)
		if err != nil {
			return nil, fmt.Errorf("ler ocorrencia listada: %w", err)
		}
		out = append(out, oc)
	}
	return out, rows.Err()
}

// ocorrenciaInserir grava oc. inserted=false quando UNIQUE(item_id, inicio)
// já tinha uma linha (ON CONFLICT DO NOTHING, decisão 7) — quem chama decide
// se isso é normal (job idempotente) ou inesperado.
func ocorrenciaInserir(ctx context.Context, q querier, oc domain.Ocorrencia) (id int64, inserted bool, err error) {
	row := q.QueryRowContext(ctx, `
		INSERT INTO ocorrencia (item_id, inicio, fim, meta, status)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(item_id, inicio) DO NOTHING
		RETURNING id`,
		oc.ItemID, dataParaColuna(oc.Inicio), dataParaColuna(oc.Fim), oc.Meta, oc.Status)
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("inserir ocorrencia (item %d): %w", oc.ItemID, err)
	}
	return id, true, nil
}

// ocorrenciaAtualizar grava fim e status de oc (os únicos campos que mudam
// depois de criada — inicio e meta são fixos).
func ocorrenciaAtualizar(ctx context.Context, q querier, oc domain.Ocorrencia) error {
	res, err := q.ExecContext(ctx, `UPDATE ocorrencia SET fim = ?, status = ? WHERE id = ?`,
		dataParaColuna(oc.Fim), oc.Status, oc.ID)
	if err != nil {
		return fmt.Errorf("atualizar ocorrencia %d: %w", oc.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirmar atualização da ocorrencia %d: %w", oc.ID, err)
	}
	if n == 0 {
		return ErrNaoEncontrado{Entidade: "ocorrencia", ID: oc.ID}
	}
	return nil
}

// ocorrenciaApagar remove a ocorrência. ATENÇÃO (decisão 8): o chamador
// (app) precisa avisar o Observador ANTES de chamar isto — notificacao.
// ocorrencia_id não tem ON DELETE, então apagar a ocorrência sem desvincular
// as notificações antes deixaria referência solta.
func ocorrenciaApagar(ctx context.Context, q querier, id int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM ocorrencia WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("apagar ocorrencia %d: %w", id, err)
	}
	return nil
}

func (s *Store) OcorrenciaBuscar(ctx context.Context, id int64) (domain.Ocorrencia, error) {
	return ocorrenciaBuscar(ctx, s.db, id)
}

func (s *Store) OcorrenciasPorItem(ctx context.Context, itemID int64) ([]domain.Ocorrencia, error) {
	return ocorrenciasPorItem(ctx, s.db, itemID)
}

func (t *Tx) OcorrenciaBuscar(ctx context.Context, id int64) (domain.Ocorrencia, error) {
	return ocorrenciaBuscar(ctx, t.tx, id)
}

func (t *Tx) OcorrenciasPorItem(ctx context.Context, itemID int64) ([]domain.Ocorrencia, error) {
	return ocorrenciasPorItem(ctx, t.tx, itemID)
}

func (t *Tx) OcorrenciaInserir(ctx context.Context, oc domain.Ocorrencia) (id int64, inserted bool, err error) {
	return ocorrenciaInserir(ctx, t.tx, oc)
}

func (t *Tx) OcorrenciaAtualizar(ctx context.Context, oc domain.Ocorrencia) error {
	return ocorrenciaAtualizar(ctx, t.tx, oc)
}

func (t *Tx) OcorrenciaApagar(ctx context.Context, id int64) error {
	return ocorrenciaApagar(ctx, t.tx, id)
}
