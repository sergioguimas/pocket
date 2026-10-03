package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"pocket/internal/domain"
)

const listaColunas = `id, nome, ordem, arquivada, lembrete, criado_em`

// lembreteJSON é a forma crua gravada na coluna lista.lembrete (CONTRATO-
// FASE3 §2.6/§4): {"regra":{...},"horarios":["09:00"]}. A regra usa o
// mesmo MarshalJSON/UnmarshalJSON de domain.Regra (§3.2 da fase 1).
type lembreteJSON struct {
	Regra    *domain.Regra `json:"regra"`
	Horarios []string      `json:"horarios"`
}

func lembreteParaColuna(l *domain.ListaLembrete) (any, error) {
	if l == nil {
		return nil, nil
	}
	b, err := json.Marshal(lembreteJSON{Regra: l.Regra, Horarios: l.Horarios})
	if err != nil {
		return nil, fmt.Errorf("serializar lembrete: %w", err)
	}
	return string(b), nil
}

func lembreteDeColuna(raw sql.NullString) (*domain.ListaLembrete, error) {
	if !raw.Valid {
		return nil, nil
	}
	var lj lembreteJSON
	if err := json.Unmarshal([]byte(raw.String), &lj); err != nil {
		return nil, fmt.Errorf("lembrete gravado inválido: %w", err)
	}
	return &domain.ListaLembrete{Regra: lj.Regra, Horarios: lj.Horarios}, nil
}

func scanLista(row interface{ Scan(...any) error }) (domain.Lista, error) {
	var (
		l         domain.Lista
		arquivada int
		lembrete  sql.NullString
	)
	if err := row.Scan(&l.ID, &l.Nome, &l.Ordem, &arquivada, &lembrete, &l.CriadoEm); err != nil {
		return domain.Lista{}, err
	}
	l.Arquivada = arquivada != 0
	lem, err := lembreteDeColuna(lembrete)
	if err != nil {
		return domain.Lista{}, fmt.Errorf("lista %d: %w", l.ID, err)
	}
	l.Lembrete = lem
	return l, nil
}

func listaBuscar(ctx context.Context, q querier, id int64) (domain.Lista, error) {
	row := q.QueryRowContext(ctx, `SELECT `+listaColunas+` FROM lista WHERE id = ?`, id)
	l, err := scanLista(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Lista{}, ErrNaoEncontrado{Entidade: "lista", ID: id}
		}
		return domain.Lista{}, fmt.Errorf("buscar lista %d: %w", id, err)
	}
	return l, nil
}

// listaListar lê listas em ordem (ordem, id); soNaoArquivadas filtra
// arquivada = 0 (GET /api/listas, §5).
func listaListar(ctx context.Context, q querier, soNaoArquivadas bool) ([]domain.Lista, error) {
	query := `SELECT ` + listaColunas + ` FROM lista`
	if soNaoArquivadas {
		query += ` WHERE arquivada = 0`
	}
	query += ` ORDER BY ordem, id`

	rows, err := q.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listar listas: %w", err)
	}
	defer rows.Close()

	var out []domain.Lista
	for rows.Next() {
		l, err := scanLista(rows)
		if err != nil {
			return nil, fmt.Errorf("ler lista listada: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func listaInserir(ctx context.Context, q querier, l domain.Lista) (int64, error) {
	lembrete, err := lembreteParaColuna(l.Lembrete)
	if err != nil {
		return 0, err
	}
	arquivada := 0
	if l.Arquivada {
		arquivada = 1
	}
	res, err := q.ExecContext(ctx, `
		INSERT INTO lista (nome, ordem, arquivada, lembrete, criado_em)
		VALUES (?, ?, ?, ?, ?)`,
		l.Nome, l.Ordem, arquivada, lembrete, l.CriadoEm)
	if err != nil {
		return 0, fmt.Errorf("inserir lista: %w", err)
	}
	return res.LastInsertId()
}

func listaAtualizar(ctx context.Context, q querier, l domain.Lista) error {
	lembrete, err := lembreteParaColuna(l.Lembrete)
	if err != nil {
		return err
	}
	arquivada := 0
	if l.Arquivada {
		arquivada = 1
	}
	res, err := q.ExecContext(ctx, `
		UPDATE lista SET nome = ?, ordem = ?, arquivada = ?, lembrete = ? WHERE id = ?`,
		l.Nome, l.Ordem, arquivada, lembrete, l.ID)
	if err != nil {
		return fmt.Errorf("atualizar lista %d: %w", l.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirmar atualização da lista %d: %w", l.ID, err)
	}
	if n == 0 {
		return ErrNaoEncontrado{Entidade: "lista", ID: l.ID}
	}
	return nil
}

// Store — leituras diretas.

func (s *Store) ListaBuscar(ctx context.Context, id int64) (domain.Lista, error) {
	return listaBuscar(ctx, s.db, id)
}

func (s *Store) ListaListar(ctx context.Context, soNaoArquivadas bool) ([]domain.Lista, error) {
	return listaListar(ctx, s.db, soNaoArquivadas)
}

// Tx — leituras/escritas dentro da transação.

func (t *Tx) ListaBuscar(ctx context.Context, id int64) (domain.Lista, error) {
	return listaBuscar(ctx, t.tx, id)
}

func (t *Tx) ListaListar(ctx context.Context, soNaoArquivadas bool) ([]domain.Lista, error) {
	return listaListar(ctx, t.tx, soNaoArquivadas)
}

func (t *Tx) ListaInserir(ctx context.Context, l domain.Lista) (int64, error) {
	return listaInserir(ctx, t.tx, l)
}

func (t *Tx) ListaAtualizar(ctx context.Context, l domain.Lista) error {
	return listaAtualizar(ctx, t.tx, l)
}
