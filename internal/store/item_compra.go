package store

import (
	"context"
	"database/sql"
	"fmt"

	"pocket/internal/domain"
)

const itemCompraColunas = `id, lista_id, secao_id, nome, nome_chave, quantidade, nota, comprado, comprado_em, origem, receita_id, criado_em`

func scanItemCompra(row interface{ Scan(...any) error }) (domain.ItemCompra, error) {
	var (
		ic         domain.ItemCompra
		quantidade sql.NullString
		nota       sql.NullString
		comprado   int
		compradoEm sql.NullString
		receitaID  sql.NullInt64
	)
	if err := row.Scan(&ic.ID, &ic.ListaID, &ic.SecaoID, &ic.Nome, &ic.NomeChave, &quantidade, &nota,
		&comprado, &compradoEm, &ic.Origem, &receitaID, &ic.CriadoEm); err != nil {
		return domain.ItemCompra{}, err
	}
	ic.Quantidade = quantidade.String
	ic.Nota = nota.String
	ic.Comprado = comprado != 0
	ic.CompradoEm = compradoEm.String
	if receitaID.Valid {
		v := receitaID.Int64
		ic.ReceitaID = &v
	}
	return ic, nil
}

func itemCompraBuscar(ctx context.Context, q querier, id int64) (domain.ItemCompra, error) {
	row := q.QueryRowContext(ctx, `SELECT `+itemCompraColunas+` FROM item_compra WHERE id = ?`, id)
	ic, err := scanItemCompra(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.ItemCompra{}, ErrNaoEncontrado{Entidade: "item_compra", ID: id}
		}
		return domain.ItemCompra{}, fmt.Errorf("buscar item_compra %d: %w", id, err)
	}
	return ic, nil
}

// itensPorLista lê os itens de listaID na ordem de CONTRATO-FASE3 §2.3
// regra 4: seção (ordem) -> pendentes por criado_em -> riscados por
// comprado_em. O JOIN com secao só serve para pegar a ordem da seção; a
// seção em si (§2.2, ordenada) o app busca à parte com SecoesPorLista.
func itensPorLista(ctx context.Context, q querier, listaID int64) ([]domain.ItemCompra, error) {
	const colunasComPrefixo = `ic.id, ic.lista_id, ic.secao_id, ic.nome, ic.nome_chave, ic.quantidade, ic.nota,
		ic.comprado, ic.comprado_em, ic.origem, ic.receita_id, ic.criado_em`
	rows, err := q.QueryContext(ctx, `
		SELECT `+colunasComPrefixo+`
		FROM item_compra ic
		JOIN secao s ON s.id = ic.secao_id
		WHERE ic.lista_id = ?
		ORDER BY s.ordem, ic.comprado, CASE WHEN ic.comprado = 0 THEN ic.criado_em ELSE ic.comprado_em END`,
		listaID)
	if err != nil {
		return nil, fmt.Errorf("listar itens da lista %d: %w", listaID, err)
	}
	defer rows.Close()

	var out []domain.ItemCompra
	for rows.Next() {
		ic, err := scanItemCompra(rows)
		if err != nil {
			return nil, fmt.Errorf("ler item_compra listado: %w", err)
		}
		out = append(out, ic)
	}
	return out, rows.Err()
}

// itemCompraPendentePorNomeChave busca o item pendente (comprado = 0) com
// nomeChave na lista — regra 1 de §2.3 (dedupe). Não encontrado devolve
// ErrNaoEncontrado (o app trata como "pode inserir").
func itemCompraPendentePorNomeChave(ctx context.Context, q querier, listaID int64, nomeChave string) (domain.ItemCompra, error) {
	row := q.QueryRowContext(ctx, `SELECT `+itemCompraColunas+`
		FROM item_compra WHERE lista_id = ? AND nome_chave = ? AND comprado = 0`, listaID, nomeChave)
	ic, err := scanItemCompra(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.ItemCompra{}, ErrNaoEncontrado{Entidade: "item_compra", ID: 0}
		}
		return domain.ItemCompra{}, fmt.Errorf("buscar item_compra pendente (lista %d, %q): %w", listaID, nomeChave, err)
	}
	return ic, nil
}

func itemCompraInserir(ctx context.Context, q querier, ic domain.ItemCompra) (int64, error) {
	comprado := 0
	if ic.Comprado {
		comprado = 1
	}
	res, err := q.ExecContext(ctx, `
		INSERT INTO item_compra (lista_id, secao_id, nome, nome_chave, quantidade, nota, comprado, comprado_em, origem, receita_id, criado_em)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ic.ListaID, ic.SecaoID, ic.Nome, ic.NomeChave, nullIfVazio(ic.Quantidade), nullIfVazio(ic.Nota),
		comprado, nullIfVazio(ic.CompradoEm), ic.Origem, ptrParaColuna(ic.ReceitaID), ic.CriadoEm)
	if err != nil {
		return 0, fmt.Errorf("inserir item_compra (lista %d): %w", ic.ListaID, err)
	}
	return res.LastInsertId()
}

// itemCompraAtualizar grava por completo os campos editáveis (id, lista_id,
// origem, receita_id e criado_em nunca mudam depois de criado — mesmo
// espírito de itemAtualizar da fase 1).
func itemCompraAtualizar(ctx context.Context, q querier, ic domain.ItemCompra) error {
	comprado := 0
	if ic.Comprado {
		comprado = 1
	}
	res, err := q.ExecContext(ctx, `
		UPDATE item_compra SET secao_id = ?, nome = ?, nome_chave = ?, quantidade = ?, nota = ?,
			comprado = ?, comprado_em = ?
		WHERE id = ?`,
		ic.SecaoID, ic.Nome, ic.NomeChave, nullIfVazio(ic.Quantidade), nullIfVazio(ic.Nota),
		comprado, nullIfVazio(ic.CompradoEm), ic.ID)
	if err != nil {
		return fmt.Errorf("atualizar item_compra %d: %w", ic.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirmar atualização do item_compra %d: %w", ic.ID, err)
	}
	if n == 0 {
		return ErrNaoEncontrado{Entidade: "item_compra", ID: ic.ID}
	}
	return nil
}

func itemCompraApagar(ctx context.Context, q querier, id int64) error {
	res, err := q.ExecContext(ctx, `DELETE FROM item_compra WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("apagar item_compra %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirmar remoção do item_compra %d: %w", id, err)
	}
	if n == 0 {
		return ErrNaoEncontrado{Entidade: "item_compra", ID: id}
	}
	return nil
}

// itemCompraMoverSecao move todos os itens de secaoOrigem (numa lista) para
// secaoDestino — usado por ApagarSecao (§2.2: "apagar uma seção move os
// itens dela para a primeira seção restante").
func itemCompraMoverSecao(ctx context.Context, q querier, secaoOrigem, secaoDestino int64) error {
	_, err := q.ExecContext(ctx, `UPDATE item_compra SET secao_id = ? WHERE secao_id = ?`, secaoDestino, secaoOrigem)
	if err != nil {
		return fmt.Errorf("mover itens da secao %d para %d: %w", secaoOrigem, secaoDestino, err)
	}
	return nil
}

// limparComprados apaga (hard delete — §2.3 regra 3: "o histórico guarda")
// os itens riscados de listaID e devolve quantos foram removidos.
func limparComprados(ctx context.Context, q querier, listaID int64) (int, error) {
	res, err := q.ExecContext(ctx, `DELETE FROM item_compra WHERE lista_id = ? AND comprado = 1`, listaID)
	if err != nil {
		return 0, fmt.Errorf("limpar comprados da lista %d: %w", listaID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("confirmar limpeza de comprados da lista %d: %w", listaID, err)
	}
	return int(n), nil
}

// pendentesPorLista conta os itens não comprados de listaID (§2.7: "Hoje
// ganha compras: [{lista_id, nome, pendentes}]").
func pendentesPorLista(ctx context.Context, q querier, listaID int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM item_compra WHERE lista_id = ? AND comprado = 0`, listaID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("contar pendentes da lista %d: %w", listaID, err)
	}
	return n, nil
}

// Store — leituras diretas.

func (s *Store) ItemCompraBuscar(ctx context.Context, id int64) (domain.ItemCompra, error) {
	return itemCompraBuscar(ctx, s.db, id)
}

func (s *Store) ItensPorLista(ctx context.Context, listaID int64) ([]domain.ItemCompra, error) {
	return itensPorLista(ctx, s.db, listaID)
}

func (s *Store) PendentesPorLista(ctx context.Context, listaID int64) (int, error) {
	return pendentesPorLista(ctx, s.db, listaID)
}

// Tx — leituras/escritas dentro da transação.

func (t *Tx) ItemCompraBuscar(ctx context.Context, id int64) (domain.ItemCompra, error) {
	return itemCompraBuscar(ctx, t.tx, id)
}

func (t *Tx) ItensPorLista(ctx context.Context, listaID int64) ([]domain.ItemCompra, error) {
	return itensPorLista(ctx, t.tx, listaID)
}

func (t *Tx) ItemCompraPendentePorNomeChave(ctx context.Context, listaID int64, nomeChave string) (domain.ItemCompra, error) {
	return itemCompraPendentePorNomeChave(ctx, t.tx, listaID, nomeChave)
}

func (t *Tx) ItemCompraInserir(ctx context.Context, ic domain.ItemCompra) (int64, error) {
	return itemCompraInserir(ctx, t.tx, ic)
}

func (t *Tx) ItemCompraAtualizar(ctx context.Context, ic domain.ItemCompra) error {
	return itemCompraAtualizar(ctx, t.tx, ic)
}

func (t *Tx) ItemCompraApagar(ctx context.Context, id int64) error {
	return itemCompraApagar(ctx, t.tx, id)
}

func (t *Tx) ItemCompraMoverSecao(ctx context.Context, secaoOrigem, secaoDestino int64) error {
	return itemCompraMoverSecao(ctx, t.tx, secaoOrigem, secaoDestino)
}

func (t *Tx) LimparComprados(ctx context.Context, listaID int64) (int, error) {
	return limparComprados(ctx, t.tx, listaID)
}

func (t *Tx) PendentesPorLista(ctx context.Context, listaID int64) (int, error) {
	return pendentesPorLista(ctx, t.tx, listaID)
}
