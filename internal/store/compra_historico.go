package store

import (
	"context"
	"database/sql"
	"fmt"

	"pocket/internal/domain"
)

const compraHistoricoColunas = `lista_id, nome_chave, nome, secao_id, ultima_quantidade, vezes, ultima_em`

func scanCompraHistorico(row interface{ Scan(...any) error }) (domain.CompraHistorico, error) {
	var (
		h         domain.CompraHistorico
		secaoID   sql.NullInt64
		ultimaQtd sql.NullString
	)
	if err := row.Scan(&h.ListaID, &h.NomeChave, &h.Nome, &secaoID, &ultimaQtd, &h.Vezes, &h.UltimaEm); err != nil {
		return domain.CompraHistorico{}, err
	}
	if secaoID.Valid {
		v := secaoID.Int64
		h.SecaoID = &v
	}
	h.UltimaQuantidade = ultimaQtd.String
	return h, nil
}

// compraHistoricoBuscar lê a linha (listaID, nomeChave). Não encontrado
// devolve ErrNaoEncontrado (o app trata como "primeira vez").
func compraHistoricoBuscar(ctx context.Context, q querier, listaID int64, nomeChave string) (domain.CompraHistorico, error) {
	row := q.QueryRowContext(ctx, `SELECT `+compraHistoricoColunas+`
		FROM compra_historico WHERE lista_id = ? AND nome_chave = ?`, listaID, nomeChave)
	h, err := scanCompraHistorico(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.CompraHistorico{}, ErrNaoEncontrado{Entidade: "compra_historico", ID: 0}
		}
		return domain.CompraHistorico{}, fmt.Errorf("buscar compra_historico (lista %d, %q): %w", listaID, nomeChave, err)
	}
	return h, nil
}

// compraHistoricoPorLista lê todo o histórico de listaID (frequentes, §2.4
// — a ordenação por score/prefixo é feita em Go pelo app, não aqui: volume
// pequeno por lista, não vale complicar o SQL).
func compraHistoricoPorLista(ctx context.Context, q querier, listaID int64) ([]domain.CompraHistorico, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+compraHistoricoColunas+` FROM compra_historico WHERE lista_id = ?`, listaID)
	if err != nil {
		return nil, fmt.Errorf("listar compra_historico da lista %d: %w", listaID, err)
	}
	defer rows.Close()

	var out []domain.CompraHistorico
	for rows.Next() {
		h, err := scanCompraHistorico(rows)
		if err != nil {
			return nil, fmt.Errorf("ler compra_historico listado: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// compraHistoricoUpsert grava h por completo (INSERT OR REPLACE pela chave
// primária lista_id, nome_chave) — usado tanto para gravar vezes+1 (riscar)
// quanto vezes-1 (desriscar no mesmo dia, §2.4).
func compraHistoricoUpsert(ctx context.Context, q querier, h domain.CompraHistorico) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO compra_historico (lista_id, nome_chave, nome, secao_id, ultima_quantidade, vezes, ultima_em)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(lista_id, nome_chave) DO UPDATE SET
			nome = excluded.nome, secao_id = excluded.secao_id,
			ultima_quantidade = excluded.ultima_quantidade, vezes = excluded.vezes, ultima_em = excluded.ultima_em`,
		h.ListaID, h.NomeChave, h.Nome, ptrParaColuna(h.SecaoID), nullIfVazio(h.UltimaQuantidade), h.Vezes, h.UltimaEm)
	if err != nil {
		return fmt.Errorf("upsert compra_historico (lista %d, %q): %w", h.ListaID, h.NomeChave, err)
	}
	return nil
}

// compraHistoricoApagar remove a linha (usado só quando desriscar zera
// vezes de um histórico que tinha acabado de nascer — ver app/compras.go).
func compraHistoricoApagar(ctx context.Context, q querier, listaID int64, nomeChave string) error {
	_, err := q.ExecContext(ctx, `DELETE FROM compra_historico WHERE lista_id = ? AND nome_chave = ?`, listaID, nomeChave)
	if err != nil {
		return fmt.Errorf("apagar compra_historico (lista %d, %q): %w", listaID, nomeChave, err)
	}
	return nil
}

// Store — leituras diretas.

func (s *Store) CompraHistoricoPorLista(ctx context.Context, listaID int64) ([]domain.CompraHistorico, error) {
	return compraHistoricoPorLista(ctx, s.db, listaID)
}

// Tx — leituras/escritas dentro da transação.

func (t *Tx) CompraHistoricoBuscar(ctx context.Context, listaID int64, nomeChave string) (domain.CompraHistorico, error) {
	return compraHistoricoBuscar(ctx, t.tx, listaID, nomeChave)
}

func (t *Tx) CompraHistoricoPorLista(ctx context.Context, listaID int64) ([]domain.CompraHistorico, error) {
	return compraHistoricoPorLista(ctx, t.tx, listaID)
}

func (t *Tx) CompraHistoricoUpsert(ctx context.Context, h domain.CompraHistorico) error {
	return compraHistoricoUpsert(ctx, t.tx, h)
}

func (t *Tx) CompraHistoricoApagar(ctx context.Context, listaID int64, nomeChave string) error {
	return compraHistoricoApagar(ctx, t.tx, listaID, nomeChave)
}
