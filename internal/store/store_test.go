package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pocket/internal/clock"
)

func abrirTeste(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "pocket.db"), clock.NewFake(time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// numeroDeMigracoes conta os arquivos migrations/NNNN_*.sql embutidos —
// usado nos testes abaixo em vez de um número fixo, para não ficarem
// desatualizados a cada nova migração (F3-1 acrescentou 0002_compras.sql).
func numeroDeMigracoes(t *testing.T) int {
	t.Helper()
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("ler migrations: %v", err)
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			n++
		}
	}
	return n
}

func TestOpenAplicaMigracao(t *testing.T) {
	s := abrirTeste(t)
	want := numeroDeMigracoes(t)

	var versoes int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_version`).Scan(&versoes); err != nil {
		t.Fatalf("contar schema_version: %v", err)
	}
	if versoes != want {
		t.Fatalf("schema_version tem %d linhas, want %d", versoes, want)
	}

	// tabelas do schema existem (fase 1 + fase 3)
	for _, tabela := range []string{
		"item", "ocorrencia", "registro", "notificacao", "push_sub", "job",
		"lista", "secao", "item_compra", "compra_historico",
	} {
		var nome string
		err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tabela).Scan(&nome)
		if err != nil {
			t.Errorf("tabela %s não existe: %v", tabela, err)
		}
	}
}

func TestOpenMigracaoIdempotente(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pocket.db")
	c := clock.NewFake(time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC))

	s1, err := Open(path, c)
	if err != nil {
		t.Fatalf("primeiro Open: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := Open(path, c)
	if err != nil {
		t.Fatalf("segundo Open: %v", err)
	}
	defer s2.Close()

	want := numeroDeMigracoes(t)
	var versoes int
	if err := s2.db.QueryRow(`SELECT COUNT(*) FROM schema_version`).Scan(&versoes); err != nil {
		t.Fatalf("contar schema_version: %v", err)
	}
	if versoes != want {
		t.Fatalf("reabrir duplicou migração: schema_version tem %d linhas, want %d", versoes, want)
	}
}

func TestOpenPing(t *testing.T) {
	s := abrirTeste(t)
	if err := s.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestSchemaCheckHabitoSemRegra(t *testing.T) {
	s := abrirTeste(t)
	_, err := s.db.Exec(`INSERT INTO item (tipo, titulo, horarios, criado_em, atualizado_em)
		VALUES ('habito', 'ler', '[]', '2026-09-26T10:00:00Z', '2026-09-26T10:00:00Z')`)
	if err == nil {
		t.Fatal("esperava erro: hábito sem regra deveria violar o CHECK")
	}
}

func TestSchemaCheckCompromissoComRegra(t *testing.T) {
	s := abrirTeste(t)
	_, err := s.db.Exec(`INSERT INTO item (tipo, titulo, regra, horarios, criado_em, atualizado_em)
		VALUES ('compromisso', 'dentista', '{"tipo":"diario"}', '[]', '2026-09-26T10:00:00Z', '2026-09-26T10:00:00Z')`)
	if err == nil {
		t.Fatal("esperava erro: compromisso com regra deveria violar o CHECK")
	}
}

func TestSchemaCheckHabitoComRegraOk(t *testing.T) {
	s := abrirTeste(t)
	_, err := s.db.Exec(`INSERT INTO item (tipo, titulo, regra, horarios, criado_em, atualizado_em)
		VALUES ('habito', 'ler', '{"tipo":"diario"}', '[]', '2026-09-26T10:00:00Z', '2026-09-26T10:00:00Z')`)
	if err != nil {
		t.Fatalf("hábito com regra devia ser aceito: %v", err)
	}
}

func TestSchemaCheckCompromissoSemRegraOk(t *testing.T) {
	s := abrirTeste(t)
	_, err := s.db.Exec(`INSERT INTO item (tipo, titulo, horarios, criado_em, atualizado_em)
		VALUES ('compromisso', 'dentista', '[]', '2026-09-26T10:00:00Z', '2026-09-26T10:00:00Z')`)
	if err != nil {
		t.Fatalf("compromisso sem regra devia ser aceito: %v", err)
	}
}

func TestSchemaForeignKeysLigadas(t *testing.T) {
	s := abrirTeste(t)
	_, err := s.db.Exec(`INSERT INTO ocorrencia (item_id, inicio, meta) VALUES (999, '2026-09-26', 1)`)
	if err == nil {
		t.Fatal("esperava erro: item_id 999 não existe (foreign_keys deveria estar ON)")
	}
}

func TestSchemaTituloVazioFalha(t *testing.T) {
	s := abrirTeste(t)
	_, err := s.db.Exec(`INSERT INTO item (tipo, titulo, horarios, criado_em, atualizado_em)
		VALUES ('compromisso', '   ', '[]', '2026-09-26T10:00:00Z', '2026-09-26T10:00:00Z')`)
	if err == nil {
		t.Fatal("esperava erro: título em branco deveria violar o CHECK")
	}
}

func TestWithTxRollbackEmErro(t *testing.T) {
	s := abrirTeste(t)
	falhaEsperada := errors.New("falha proposital")
	err := s.withTx(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO item (tipo, titulo, horarios, criado_em, atualizado_em)
			VALUES ('compromisso', 'tarefa', '[]', '2026-09-26T10:00:00Z', '2026-09-26T10:00:00Z')`); err != nil {
			return err
		}
		return falhaEsperada
	})
	if !errors.Is(err, falhaEsperada) {
		t.Fatalf("withTx erro = %v, want %v", err, falhaEsperada)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM item`).Scan(&n); err != nil {
		t.Fatalf("contar item: %v", err)
	}
	if n != 0 {
		t.Fatalf("rollback não desfez o insert: item tem %d linhas, want 0", n)
	}
}
