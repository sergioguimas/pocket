package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"pocket/internal/domain"
)

// itemColunas lista as colunas de item na ordem usada por todo SELECT deste
// arquivo — um lugar só para SELECT e scanItem não desalinharem.
const itemColunas = `id, tipo, titulo, notas, regra, data, hora, horarios, estado, cor, ordem, criado_em, atualizado_em`

// scanItem lê uma linha de item (na ordem de itemColunas) de row (que tanto
// *sql.Row quanto *sql.Rows satisfazem).
func scanItem(row interface{ Scan(...any) error }) (domain.Item, error) {
	var (
		it           domain.Item
		notas        sql.NullString
		regraJSON    sql.NullString
		dataStr      sql.NullString
		hora         sql.NullString
		horariosJSON string
		cor          sql.NullString
		criadoEm     string
		atualizadoEm string
	)
	if err := row.Scan(&it.ID, &it.Tipo, &it.Titulo, &notas, &regraJSON, &dataStr, &hora,
		&horariosJSON, &it.Estado, &cor, &it.Ordem, &criadoEm, &atualizadoEm); err != nil {
		return domain.Item{}, err
	}
	it.Notas = notas.String
	it.Hora = hora.String
	it.Cor = cor.String

	if regraJSON.Valid {
		regra, err := domain.ParseRegra([]byte(regraJSON.String))
		if err != nil {
			return domain.Item{}, fmt.Errorf("regra gravada inválida (item %d): %w", it.ID, err)
		}
		it.Regra = regra
	}
	if dataStr.Valid {
		d, err := domain.ParseDia(dataStr.String)
		if err != nil {
			return domain.Item{}, fmt.Errorf("data gravada inválida (item %d): %w", it.ID, err)
		}
		it.Data = &d
	}
	if err := json.Unmarshal([]byte(horariosJSON), &it.Horarios); err != nil {
		return domain.Item{}, fmt.Errorf("horarios gravados inválidos (item %d): %w", it.ID, err)
	}
	criado, err := domain.ParseInstante(criadoEm)
	if err != nil {
		return domain.Item{}, fmt.Errorf("criado_em gravado inválido (item %d): %w", it.ID, err)
	}
	it.CriadoEm = criado
	atualizado, err := domain.ParseInstante(atualizadoEm)
	if err != nil {
		return domain.Item{}, fmt.Errorf("atualizado_em gravado inválido (item %d): %w", it.ID, err)
	}
	it.AtualizadoEm = atualizado

	return it, nil
}

// itemBuscar lê um item por id. Não encontrado devolve ErrNaoEncontrado (o
// app mapeia para 404 — este pacote não conhece HTTP, mas expõe o mesmo
// vocabulário de erro do app via os tipos abaixo, para o app só usar
// errors.As uma vez).
func itemBuscar(ctx context.Context, q querier, id int64) (domain.Item, error) {
	row := q.QueryRowContext(ctx, `SELECT `+itemColunas+` FROM item WHERE id = ?`, id)
	it, err := scanItem(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Item{}, ErrNaoEncontrado{Entidade: "item", ID: id}
		}
		return domain.Item{}, fmt.Errorf("buscar item %d: %w", id, err)
	}
	return it, nil
}

// itemListar lê itens filtrando por tipo/estado quando não vazios, em
// ordem (ordem, id).
func itemListar(ctx context.Context, q querier, tipo, estado string) ([]domain.Item, error) {
	query := `SELECT ` + itemColunas + ` FROM item WHERE 1=1`
	var args []any
	if tipo != "" {
		query += ` AND tipo = ?`
		args = append(args, tipo)
	}
	if estado != "" {
		query += ` AND estado = ?`
		args = append(args, estado)
	}
	query += ` ORDER BY ordem, id`

	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listar itens: %w", err)
	}
	defer rows.Close()

	var itens []domain.Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("ler item listado: %w", err)
		}
		itens = append(itens, it)
	}
	return itens, rows.Err()
}

// itemInserir grava um novo item e devolve o id gerado.
func itemInserir(ctx context.Context, q querier, it domain.Item) (int64, error) {
	regraJSON, err := regraParaColuna(it.Regra)
	if err != nil {
		return 0, err
	}
	horariosJSON, err := json.Marshal(it.Horarios)
	if err != nil {
		return 0, fmt.Errorf("serializar horarios: %w", err)
	}

	res, err := q.ExecContext(ctx, `
		INSERT INTO item (tipo, titulo, notas, regra, data, hora, horarios, estado, cor, ordem, criado_em, atualizado_em)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		it.Tipo, it.Titulo, nullIfVazio(it.Notas), regraJSON, dataParaColuna(it.Data), nullIfVazio(it.Hora),
		string(horariosJSON), it.Estado, nullIfVazio(it.Cor), it.Ordem,
		domain.FormatInstante(it.CriadoEm), domain.FormatInstante(it.AtualizadoEm))
	if err != nil {
		return 0, fmt.Errorf("inserir item: %w", err)
	}
	return res.LastInsertId()
}

// itemAtualizar grava por completo os campos editáveis do item (id fixo,
// tipo e criado_em nunca mudam depois de criado).
func itemAtualizar(ctx context.Context, q querier, it domain.Item) error {
	regraJSON, err := regraParaColuna(it.Regra)
	if err != nil {
		return err
	}
	horariosJSON, err := json.Marshal(it.Horarios)
	if err != nil {
		return fmt.Errorf("serializar horarios: %w", err)
	}

	res, err := q.ExecContext(ctx, `
		UPDATE item SET titulo = ?, notas = ?, regra = ?, data = ?, hora = ?, horarios = ?,
			estado = ?, cor = ?, ordem = ?, atualizado_em = ?
		WHERE id = ?`,
		it.Titulo, nullIfVazio(it.Notas), regraJSON, dataParaColuna(it.Data), nullIfVazio(it.Hora),
		string(horariosJSON), it.Estado, nullIfVazio(it.Cor), it.Ordem,
		domain.FormatInstante(it.AtualizadoEm), it.ID)
	if err != nil {
		return fmt.Errorf("atualizar item %d: %w", it.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirmar atualização do item %d: %w", it.ID, err)
	}
	if n == 0 {
		return ErrNaoEncontrado{Entidade: "item", ID: it.ID}
	}
	return nil
}

// regraParaColuna serializa a regra de um item (nil para compromisso, JSON
// canônico de domain.Regra para hábito).
func regraParaColuna(r *domain.Regra) (any, error) {
	if r == nil {
		return nil, nil
	}
	b, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("serializar regra: %w", err)
	}
	return string(b), nil
}

func dataParaColuna(d *domain.Dia) any {
	if d == nil {
		return nil
	}
	return string(*d)
}

func nullIfVazio(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Store — leituras diretas (fora de transação; §11/decisão 1).

func (s *Store) ItemBuscar(ctx context.Context, id int64) (domain.Item, error) {
	return itemBuscar(ctx, s.db, id)
}

func (s *Store) ItemListar(ctx context.Context, tipo, estado string) ([]domain.Item, error) {
	return itemListar(ctx, s.db, tipo, estado)
}

// Tx — leituras e escritas dentro da transação.

func (t *Tx) ItemBuscar(ctx context.Context, id int64) (domain.Item, error) {
	return itemBuscar(ctx, t.tx, id)
}

func (t *Tx) ItemListar(ctx context.Context, tipo, estado string) ([]domain.Item, error) {
	return itemListar(ctx, t.tx, tipo, estado)
}

func (t *Tx) ItemInserir(ctx context.Context, it domain.Item) (int64, error) {
	return itemInserir(ctx, t.tx, it)
}

func (t *Tx) ItemAtualizar(ctx context.Context, it domain.Item) error {
	return itemAtualizar(ctx, t.tx, it)
}
