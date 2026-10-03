// Package store persiste o Pocket em SQLite (CONTRATO §4). Uma única
// conexão de escrita (SetMaxOpenConns(1)), migrações embutidas no binário —
// mesmo padrão de servicos/smaug/internal/store.
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // driver "sqlite", registrado no init do pacote

	"pocket/internal/clock"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// ErrNaoEncontrado é o erro sentinela de "não achei a linha" (item,
// ocorrência ou registro) — vocabulário do store, não de HTTP. O app (etapa
// 3/decisão 10) traduz isto para app.ErrNaoEncontrado, que o httpapi mapeia
// para 404; este pacote não precisa saber de status HTTP.
type ErrNaoEncontrado struct {
	Entidade string
	ID       int64
}

func (e ErrNaoEncontrado) Error() string {
	return fmt.Sprintf("%s %d não encontrado", e.Entidade, e.ID)
}

// Store encapsula o *sql.DB e o clock injetado (nenhum time.Now() aqui —
// regra do build).
type Store struct {
	db    *sql.DB
	clock clock.Clock
}

// Open abre (criando se preciso) o banco em path, com WAL, foreign_keys e
// busy_timeout=5000 (CONTRATO §2), e aplica as migrações pendentes.
func Open(path string, c clock.Clock) (*Store, error) {
	if c == nil {
		c = clock.Real{}
	}

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("criar diretório do banco (%s): %w", dir, err)
		}
	}

	dsn := "file:" + path + "?" +
		"_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(ON)" +
		"&_pragma=busy_timeout(5000)" +
		"&_txlock=" + url.QueryEscape("immediate")

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("abrir banco (%s): %w", path, err)
	}
	// DECISÃO: uma única conexão de escrita, como o Smaug — mais simples e
	// seguro com WAL e um processo só.
	db.SetMaxOpenConns(1)

	s := &Store{db: db, clock: c}

	ctx := context.Background()
	if err := migrate(ctx, db, c); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrar banco: %w", err)
	}

	return s, nil
}

// Ping verifica a saúde do banco (usado por GET /health).
func (s *Store) Ping() error {
	return s.db.Ping()
}

// VacuumInto grava uma cópia consistente do banco em path (CONTRATO §7: job
// diário de backup, "VACUUM INTO data/backup/pocket-YYYY-MM-DD.db"). O
// diretório de path já deve existir — quem chama (internal/ops) cria antes.
func (s *Store) VacuumInto(ctx context.Context, path string) error {
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return fmt.Errorf("vacuum into (%s): %w", path, err)
	}
	return nil
}

// Close fecha a conexão com o banco.
func (s *Store) Close() error {
	return s.db.Close()
}

// withTx roda fn dentro de uma transação BEGIN IMMEDIATE (a DSN carrega
// _txlock=immediate — ver comentário equivalente em servicos/smaug). Toda
// escrita de várias linhas do app deve usar isto.
func (s *Store) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("iniciar transação: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// querier é o denominador comum de *sql.DB e *sql.Tx — as funções de
// repositório (item.go, ocorrencia.go, registro.go, job.go) recebem uma
// querier em vez de um dos dois tipos concretos, para servir tanto às
// leituras diretas do Store quanto às leituras/escritas dentro de uma Tx.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Tx é uma transação aberta com os métodos de repositório disponíveis nela
// (DECISÃO Opus, etapa 3/decisão 1). Cada caso de uso do app que escreve
// (registrar, mudar estado, editar item...) faz tudo numa Tx só, para que
// escrita da linha principal e recálculo de ocorrência fiquem atômicos.
type Tx struct {
	tx    *sql.Tx
	clock clock.Clock
}

// WithTx abre uma transação e a passa para fn; erro de fn desfaz (rollback),
// sucesso confirma (commit). Leituras isoladas podem ir direto pelo Store
// (métodos sem Tx) — só escritas de mais de uma linha precisam disto.
func (s *Store) WithTx(ctx context.Context, fn func(*Tx) error) error {
	return s.withTx(ctx, func(sqlTx *sql.Tx) error {
		return fn(&Tx{tx: sqlTx, clock: s.clock})
	})
}

// migrate cria a tabela schema_version se preciso e aplica, em ordem, os
// arquivos migrations/NNNN_*.sql ainda não registrados nela.
func migrate(ctx context.Context, db *sql.DB, c clock.Clock) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (
		version     INTEGER PRIMARY KEY,
		aplicada_em TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("criar schema_version: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("ler migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		version, err := versionFromName(name)
		if err != nil {
			return err
		}

		var ja int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_version WHERE version = ?`, version).Scan(&ja); err != nil {
			return fmt.Errorf("checar versão %d: %w", version, err)
		}
		if ja > 0 {
			continue
		}

		sqlBytes, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("ler %s: %w", name, err)
		}
		if _, err := db.ExecContext(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("aplicar %s: %w", name, err)
		}
		// DECISÃO: aplicada_em em RFC3339 UTC pelo clock injetado — é
		// bookkeeping de infraestrutura, não data de domínio (§4 fala das
		// datas do schema, não desta tabela), mas a regra "nenhum
		// time.Now() fora de clock/main" vale igual.
		if _, err := db.ExecContext(ctx,
			`INSERT INTO schema_version (version, aplicada_em) VALUES (?, ?)`,
			version, c.Now().UTC().Format(time.RFC3339),
		); err != nil {
			return fmt.Errorf("marcar versão %d aplicada: %w", version, err)
		}
	}
	return nil
}

// versionFromName extrai o número de versão do prefixo "NNNN_" do nome do
// arquivo de migração (ex.: "0001_init.sql" -> 1).
func versionFromName(name string) (int, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("nome de migração inválido (%q): esperado NNNN_descricao.sql", name)
	}
	v, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("nome de migração inválido (%q): %w", name, err)
	}
	return v, nil
}
