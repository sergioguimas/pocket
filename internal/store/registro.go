package store

import (
	"context"
	"database/sql"
	"fmt"

	"pocket/internal/domain"
)

const registroColunas = `id, item_id, em, valor, nota, origem`

func scanRegistro(row interface{ Scan(...any) error }) (domain.Registro, error) {
	var (
		r     domain.Registro
		em    string
		valor sql.NullFloat64
		nota  sql.NullString
	)
	if err := row.Scan(&r.ID, &r.ItemID, &em, &valor, &nota, &r.Origem); err != nil {
		return domain.Registro{}, err
	}
	t, err := domain.ParseInstante(em)
	if err != nil {
		return domain.Registro{}, fmt.Errorf("em gravado inválido (registro %d): %w", r.ID, err)
	}
	r.Em = t
	if valor.Valid {
		v := valor.Float64
		r.Valor = &v
	}
	r.Nota = nota.String
	return r, nil
}

func registroBuscar(ctx context.Context, q querier, id int64) (domain.Registro, error) {
	row := q.QueryRowContext(ctx, `SELECT `+registroColunas+` FROM registro WHERE id = ?`, id)
	r, err := scanRegistro(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Registro{}, ErrNaoEncontrado{Entidade: "registro", ID: id}
		}
		return domain.Registro{}, fmt.Errorf("buscar registro %d: %w", id, err)
	}
	return r, nil
}

// registrosPorItem lê todos os registros de um item, em ordem crescente de
// em (DECISÃO Opus, decisão 2: filtragem por dia local é feita em Go pelo
// app — aqui só pré-filtramos por item_id, como pedido).
func registrosPorItem(ctx context.Context, q querier, itemID int64) ([]domain.Registro, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+registroColunas+` FROM registro WHERE item_id = ? ORDER BY em`, itemID)
	if err != nil {
		return nil, fmt.Errorf("listar registros do item %d: %w", itemID, err)
	}
	defer rows.Close()

	var out []domain.Registro
	for rows.Next() {
		r, err := scanRegistro(rows)
		if err != nil {
			return nil, fmt.Errorf("ler registro listado: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// registrosUltimos lê os limite registros mais recentes de um item (mais
// recente primeiro) — usado por GET /api/itens/{id} (decisão 9).
func registrosUltimos(ctx context.Context, q querier, itemID int64, limite int) ([]domain.Registro, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+registroColunas+` FROM registro WHERE item_id = ? ORDER BY em DESC LIMIT ?`, itemID, limite)
	if err != nil {
		return nil, fmt.Errorf("listar últimos registros do item %d: %w", itemID, err)
	}
	defer rows.Close()

	var out []domain.Registro
	for rows.Next() {
		r, err := scanRegistro(rows)
		if err != nil {
			return nil, fmt.Errorf("ler registro listado: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func registroInserir(ctx context.Context, q querier, r domain.Registro) (int64, error) {
	var valor any
	if r.Valor != nil {
		valor = *r.Valor
	}
	res, err := q.ExecContext(ctx, `
		INSERT INTO registro (item_id, em, valor, nota, origem) VALUES (?, ?, ?, ?, ?)`,
		r.ItemID, domain.FormatInstante(r.Em), valor, nullIfVazio(r.Nota), r.Origem)
	if err != nil {
		return 0, fmt.Errorf("inserir registro (item %d): %w", r.ItemID, err)
	}
	return res.LastInsertId()
}

func registroApagar(ctx context.Context, q querier, id int64) error {
	res, err := q.ExecContext(ctx, `DELETE FROM registro WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("apagar registro %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirmar remoção do registro %d: %w", id, err)
	}
	if n == 0 {
		return ErrNaoEncontrado{Entidade: "registro", ID: id}
	}
	return nil
}

// registroAtualizarNota troca só a nota ("" grava NULL) — a nota é o "o que
// eu fiz" de um feito, editável depois (pedido de 28/09).
func registroAtualizarNota(ctx context.Context, q querier, id int64, nota string) error {
	res, err := q.ExecContext(ctx, `UPDATE registro SET nota = ? WHERE id = ?`, nullIfVazio(nota), id)
	if err != nil {
		return fmt.Errorf("atualizar nota do registro %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirmar nota do registro %d: %w", id, err)
	}
	if n == 0 {
		return ErrNaoEncontrado{Entidade: "registro", ID: id}
	}
	return nil
}

func (t *Tx) RegistroAtualizarNota(ctx context.Context, id int64, nota string) error {
	return registroAtualizarNota(ctx, t.tx, id, nota)
}

func (s *Store) RegistrosPorItem(ctx context.Context, itemID int64) ([]domain.Registro, error) {
	return registrosPorItem(ctx, s.db, itemID)
}

func (s *Store) RegistrosUltimos(ctx context.Context, itemID int64, limite int) ([]domain.Registro, error) {
	return registrosUltimos(ctx, s.db, itemID, limite)
}

func (t *Tx) RegistroBuscar(ctx context.Context, id int64) (domain.Registro, error) {
	return registroBuscar(ctx, t.tx, id)
}

func (t *Tx) RegistrosPorItem(ctx context.Context, itemID int64) ([]domain.Registro, error) {
	return registrosPorItem(ctx, t.tx, itemID)
}

func (t *Tx) RegistroInserir(ctx context.Context, r domain.Registro) (int64, error) {
	return registroInserir(ctx, t.tx, r)
}

func (t *Tx) RegistroApagar(ctx context.Context, id int64) error {
	return registroApagar(ctx, t.tx, id)
}
