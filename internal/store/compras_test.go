package store

import (
	"database/sql"
	"testing"
	"time"

	"pocket/internal/clock"
	"pocket/internal/domain"
)

// abrirSoComMigracao0001 abre um banco novo e aplica só a migração
// 0001_init.sql (sem passar pelo migrate() do pacote, que aplicaria as duas
// disponíveis) — simula o estado de produção antes do deploy da fase 3
// (CONTRATO-FASE3 §4/K9). O chamador semeia dados de fase 1 nele, fecha, e
// reabre com abrirTeste2 (aí sim pelo mecanismo normal) para a 0002 rodar
// por cima.
func abrirSoComMigracao0001(t *testing.T, path string) *sql.DB {
	t.Helper()
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("abrir banco cru: %v", err)
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`CREATE TABLE schema_version (version INTEGER PRIMARY KEY, aplicada_em TEXT NOT NULL)`); err != nil {
		t.Fatalf("criar schema_version: %v", err)
	}
	sqlBytes, err := migrationsFS.ReadFile("migrations/0001_init.sql")
	if err != nil {
		t.Fatalf("ler 0001_init.sql: %v", err)
	}
	if _, err := db.Exec(string(sqlBytes)); err != nil {
		t.Fatalf("aplicar 0001_init.sql: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO schema_version (version, aplicada_em) VALUES (1, '2026-09-01T00:00:00Z')`); err != nil {
		t.Fatalf("marcar 0001 aplicada: %v", err)
	}
	return db
}

// abrirTeste2 é abrirTeste com um path escolhido pelo chamador (em vez de
// t.TempDir() sempre novo) — usado por TestMigracao0002SobreBancoDaFase1
// para reabrir o mesmo arquivo semeado por abrirSoComMigracao0001.
func abrirTeste2(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(path, clock.NewFake(time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// criarListaTeste cria uma lista "Teste" (fora do seed "Mercado" da
// migração) e devolve seu id.
func criarListaTeste(t *testing.T, s *Store, nome string) int64 {
	t.Helper()
	id, err := listaInserir(t.Context(), s.db, domain.Lista{
		Nome: nome, Ordem: 0, CriadoEm: "2026-09-26T10:00:00-03:00",
	})
	if err != nil {
		t.Fatalf("criarListaTeste: %v", err)
	}
	return id
}

func criarSecaoTeste(t *testing.T, s *Store, listaID int64, nome string, ordem int) int64 {
	t.Helper()
	id, err := secaoInserir(t.Context(), s.db, domain.Secao{ListaID: listaID, Nome: nome, Ordem: ordem})
	if err != nil {
		t.Fatalf("criarSecaoTeste: %v", err)
	}
	return id
}

func TestListaInserirBuscarListar(t *testing.T) {
	s := abrirTeste(t)
	ctx := t.Context()

	id := criarListaTeste(t, s, "Farmácia")
	lida, err := listaBuscar(ctx, s.db, id)
	if err != nil {
		t.Fatalf("listaBuscar: %v", err)
	}
	if lida.Nome != "Farmácia" || lida.Arquivada {
		t.Fatalf("lista lida = %+v", lida)
	}

	// a migração já semeou "Mercado" (id 1) — listar sem filtro devolve as duas.
	todas, err := listaListar(ctx, s.db, false)
	if err != nil {
		t.Fatalf("listaListar: %v", err)
	}
	if len(todas) != 2 {
		t.Fatalf("listaListar = %d listas, want 2 (Mercado + Farmácia)", len(todas))
	}
}

func TestListaLembreteRoundTrip(t *testing.T) {
	s := abrirTeste(t)
	ctx := t.Context()

	id := criarListaTeste(t, s, "Farmácia")
	lida, err := listaBuscar(ctx, s.db, id)
	if err != nil {
		t.Fatalf("listaBuscar: %v", err)
	}
	if lida.Lembrete != nil {
		t.Fatalf("lembrete inicial = %+v, want nil", lida.Lembrete)
	}

	lida.Lembrete = &domain.ListaLembrete{Regra: &domain.Regra{Tipo: domain.RegraDiario}, Horarios: []string{"09:00"}}
	if err := listaAtualizar(ctx, s.db, lida); err != nil {
		t.Fatalf("listaAtualizar: %v", err)
	}
	relida, err := listaBuscar(ctx, s.db, id)
	if err != nil {
		t.Fatalf("listaBuscar após atualizar: %v", err)
	}
	if relida.Lembrete == nil || relida.Lembrete.Regra.Tipo != domain.RegraDiario || len(relida.Lembrete.Horarios) != 1 {
		t.Fatalf("lembrete relido = %+v", relida.Lembrete)
	}
}

func TestSecaoCRUD(t *testing.T) {
	s := abrirTeste(t)
	ctx := t.Context()

	listaID := criarListaTeste(t, s, "Farmácia")
	id := criarSecaoTeste(t, s, listaID, "Geral", 0)

	secoes, err := secoesPorLista(ctx, s.db, listaID)
	if err != nil {
		t.Fatalf("secoesPorLista: %v", err)
	}
	if len(secoes) != 1 || secoes[0].ID != id {
		t.Fatalf("secoesPorLista = %+v", secoes)
	}

	sc := secoes[0]
	sc.Nome = "Remédios"
	if err := secaoAtualizar(ctx, s.db, sc); err != nil {
		t.Fatalf("secaoAtualizar: %v", err)
	}
	relida, err := secaoBuscar(ctx, s.db, id)
	if err != nil {
		t.Fatalf("secaoBuscar: %v", err)
	}
	if relida.Nome != "Remédios" {
		t.Fatalf("nome após atualizar = %q", relida.Nome)
	}

	if err := secaoApagar(ctx, s.db, id); err != nil {
		t.Fatalf("secaoApagar: %v", err)
	}
	if _, err := secaoBuscar(ctx, s.db, id); !isErrNaoEncontrado(err) {
		t.Fatalf("secaoBuscar após apagar: err = %v, want ErrNaoEncontrado", err)
	}
}

func TestItemCompraInserirBuscarAtualizarApagar(t *testing.T) {
	s := abrirTeste(t)
	ctx := t.Context()

	listaID := criarListaTeste(t, s, "Farmácia")
	secaoID := criarSecaoTeste(t, s, listaID, "Geral", 0)

	ic := domain.ItemCompra{
		ListaID: listaID, SecaoID: secaoID, Nome: "Dipirona", NomeChave: domain.NomeChave("Dipirona"),
		Quantidade: "1 cx", Origem: domain.OrigemCompraManual, CriadoEm: "2026-09-26T10:00:00-03:00",
	}
	id, err := itemCompraInserir(ctx, s.db, ic)
	if err != nil {
		t.Fatalf("itemCompraInserir: %v", err)
	}

	lido, err := itemCompraBuscar(ctx, s.db, id)
	if err != nil {
		t.Fatalf("itemCompraBuscar: %v", err)
	}
	if lido.Nome != "Dipirona" || lido.NomeChave != "dipirona" || lido.Comprado {
		t.Fatalf("item lido = %+v", lido)
	}

	lido.Comprado = true
	lido.CompradoEm = "2026-09-26T11:00:00-03:00"
	if err := itemCompraAtualizar(ctx, s.db, lido); err != nil {
		t.Fatalf("itemCompraAtualizar: %v", err)
	}
	relido, err := itemCompraBuscar(ctx, s.db, id)
	if err != nil {
		t.Fatalf("itemCompraBuscar após atualizar: %v", err)
	}
	if !relido.Comprado || relido.CompradoEm == "" {
		t.Fatalf("item relido = %+v", relido)
	}

	if err := itemCompraApagar(ctx, s.db, id); err != nil {
		t.Fatalf("itemCompraApagar: %v", err)
	}
	if _, err := itemCompraBuscar(ctx, s.db, id); !isErrNaoEncontrado(err) {
		t.Fatalf("itemCompraBuscar após apagar: err = %v, want ErrNaoEncontrado", err)
	}
}

// TestItemCompraPendenteUnicoPorNomeChave prova o índice único parcial da
// migração 0002 (regra 1 de §2.3): dois pendentes com o mesmo nome_chave na
// mesma lista não podem coexistir, mas um riscado + um pendente podem.
func TestItemCompraPendenteUnicoPorNomeChave(t *testing.T) {
	s := abrirTeste(t)
	ctx := t.Context()

	listaID := criarListaTeste(t, s, "Farmácia")
	secaoID := criarSecaoTeste(t, s, listaID, "Geral", 0)

	base := domain.ItemCompra{
		ListaID: listaID, SecaoID: secaoID, Nome: "Dipirona", NomeChave: "dipirona",
		Origem: domain.OrigemCompraManual, CriadoEm: "2026-09-26T10:00:00-03:00",
	}
	if _, err := itemCompraInserir(ctx, s.db, base); err != nil {
		t.Fatalf("primeira inserção: %v", err)
	}
	if _, err := itemCompraInserir(ctx, s.db, base); err == nil {
		t.Fatal("esperava erro: segundo pendente com o mesmo nome_chave devia violar o índice único")
	}

	// riscado com o mesmo nome_chave pode coexistir com um pendente novo.
	riscado := base
	riscado.Comprado = true
	riscado.CompradoEm = "2026-09-26T11:00:00-03:00"
	if _, err := itemCompraInserir(ctx, s.db, riscado); err != nil {
		t.Fatalf("riscado com mesmo nome_chave devia ser aceito: %v", err)
	}
}

func TestItemCompraPendentePorNomeChave(t *testing.T) {
	s := abrirTeste(t)
	ctx := t.Context()

	listaID := criarListaTeste(t, s, "Farmácia")
	secaoID := criarSecaoTeste(t, s, listaID, "Geral", 0)

	if _, err := itemCompraPendentePorNomeChave(ctx, s.db, listaID, "dipirona"); !isErrNaoEncontrado(err) {
		t.Fatalf("busca antes de inserir: err = %v, want ErrNaoEncontrado", err)
	}

	ic := domain.ItemCompra{
		ListaID: listaID, SecaoID: secaoID, Nome: "Dipirona", NomeChave: "dipirona",
		Origem: domain.OrigemCompraManual, CriadoEm: "2026-09-26T10:00:00-03:00",
	}
	id, err := itemCompraInserir(ctx, s.db, ic)
	if err != nil {
		t.Fatalf("itemCompraInserir: %v", err)
	}

	achado, err := itemCompraPendentePorNomeChave(ctx, s.db, listaID, "dipirona")
	if err != nil {
		t.Fatalf("itemCompraPendentePorNomeChave: %v", err)
	}
	if achado.ID != id {
		t.Fatalf("achado.ID = %d, want %d", achado.ID, id)
	}
}

func TestItensPorListaOrdem(t *testing.T) {
	s := abrirTeste(t)
	ctx := t.Context()

	listaID := criarListaTeste(t, s, "Farmácia")
	secA := criarSecaoTeste(t, s, listaID, "A", 0)
	secB := criarSecaoTeste(t, s, listaID, "B", 1)

	// seção B, pendente
	inserirItemTeste(t, s, listaID, secB, "zulu", false, "2026-09-26T10:00:00-03:00", "")
	// seção A, pendente criado depois
	inserirItemTeste(t, s, listaID, secA, "bravo", false, "2026-09-26T10:02:00-03:00", "")
	// seção A, pendente criado antes
	inserirItemTeste(t, s, listaID, secA, "alfa", false, "2026-09-26T10:01:00-03:00", "")
	// seção A, riscado
	inserirItemTeste(t, s, listaID, secA, "riscado", true, "2026-09-26T10:00:00-03:00", "2026-09-26T10:05:00-03:00")

	itens, err := itensPorLista(ctx, s.db, listaID)
	if err != nil {
		t.Fatalf("itensPorLista: %v", err)
	}
	var nomes []string
	for _, it := range itens {
		nomes = append(nomes, it.Nome)
	}
	want := []string{"alfa", "bravo", "riscado", "zulu"}
	if len(nomes) != len(want) {
		t.Fatalf("itensPorLista = %v, want %v", nomes, want)
	}
	for i := range want {
		if nomes[i] != want[i] {
			t.Fatalf("itensPorLista = %v, want %v", nomes, want)
		}
	}
}

func inserirItemTeste(t *testing.T, s *Store, listaID, secaoID int64, nome string, comprado bool, criadoEm, compradoEm string) {
	t.Helper()
	_, err := itemCompraInserir(t.Context(), s.db, domain.ItemCompra{
		ListaID: listaID, SecaoID: secaoID, Nome: nome, NomeChave: domain.NomeChave(nome),
		Comprado: comprado, CompradoEm: compradoEm, Origem: domain.OrigemCompraManual, CriadoEm: criadoEm,
	})
	if err != nil {
		t.Fatalf("inserirItemTeste(%q): %v", nome, err)
	}
}

func TestLimparCompradosApagaSoRiscados(t *testing.T) {
	s := abrirTeste(t)
	ctx := t.Context()

	listaID := criarListaTeste(t, s, "Farmácia")
	secaoID := criarSecaoTeste(t, s, listaID, "Geral", 0)
	inserirItemTeste(t, s, listaID, secaoID, "pendente", false, "2026-09-26T10:00:00-03:00", "")
	inserirItemTeste(t, s, listaID, secaoID, "riscado1", true, "2026-09-26T10:00:00-03:00", "2026-09-26T10:05:00-03:00")
	inserirItemTeste(t, s, listaID, secaoID, "riscado2", true, "2026-09-26T10:00:00-03:00", "2026-09-26T10:06:00-03:00")

	n, err := limparComprados(ctx, s.db, listaID)
	if err != nil {
		t.Fatalf("limparComprados: %v", err)
	}
	if n != 2 {
		t.Fatalf("limparComprados devolveu %d, want 2", n)
	}

	restantes, err := itensPorLista(ctx, s.db, listaID)
	if err != nil {
		t.Fatalf("itensPorLista: %v", err)
	}
	if len(restantes) != 1 || restantes[0].Nome != "pendente" {
		t.Fatalf("restantes = %+v, want só 'pendente'", restantes)
	}
}

func TestCompraHistoricoUpsert(t *testing.T) {
	s := abrirTeste(t)
	ctx := t.Context()

	listaID := criarListaTeste(t, s, "Farmácia")
	secaoID := criarSecaoTeste(t, s, listaID, "Geral", 0)

	if _, err := compraHistoricoBuscar(ctx, s.db, listaID, "dipirona"); !isErrNaoEncontrado(err) {
		t.Fatalf("busca antes de existir: err = %v, want ErrNaoEncontrado", err)
	}

	h := domain.CompraHistorico{
		ListaID: listaID, NomeChave: "dipirona", Nome: "Dipirona", SecaoID: &secaoID,
		UltimaQuantidade: "1 cx", Vezes: 1, UltimaEm: "2026-09-26T10:00:00-03:00",
	}
	if err := compraHistoricoUpsert(ctx, s.db, h); err != nil {
		t.Fatalf("compraHistoricoUpsert (insert): %v", err)
	}

	h.Vezes = 2
	h.UltimaEm = "2026-10-03T10:00:00-03:00"
	if err := compraHistoricoUpsert(ctx, s.db, h); err != nil {
		t.Fatalf("compraHistoricoUpsert (update): %v", err)
	}

	lido, err := compraHistoricoBuscar(ctx, s.db, listaID, "dipirona")
	if err != nil {
		t.Fatalf("compraHistoricoBuscar: %v", err)
	}
	if lido.Vezes != 2 || lido.UltimaEm != "2026-10-03T10:00:00-03:00" {
		t.Fatalf("histórico lido = %+v", lido)
	}

	if err := compraHistoricoApagar(ctx, s.db, listaID, "dipirona"); err != nil {
		t.Fatalf("compraHistoricoApagar: %v", err)
	}
	if _, err := compraHistoricoBuscar(ctx, s.db, listaID, "dipirona"); !isErrNaoEncontrado(err) {
		t.Fatalf("busca após apagar: err = %v, want ErrNaoEncontrado", err)
	}
}

// TestMigracao0002SobreBancoDaFase1 é o teste K9: abre um banco só com
// 0001, insere itens/ocorrências/registros/notificações pendentes de item
// como a fase 1 faria, aplica 0002 e confere que tudo sobreviveu com o
// mesmo id, que a lista Mercado + as 10 seções existem, e que o CHECK
// (item_id xor lista_id) e os índices parciais funcionam.
func TestMigracao0002SobreBancoDaFase1(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/pocket.db"

	// abre um banco cru e aplica só a 0001 (sem passar pelo migrate() do
	// pacote, que já aplicaria as duas) — simula o estado real de produção
	// antes do deploy da fase 3.
	db := abrirSoComMigracao0001(t, path)

	agora := "2026-09-26T10:00:00-03:00"
	if _, err := db.Exec(`INSERT INTO item (id, tipo, titulo, regra, horarios, estado, ordem, criado_em, atualizado_em)
		VALUES (1, 'habito', 'ler', '{"tipo":"diario"}', '[]', 'ativo', 0, ?, ?)`, agora, agora); err != nil {
		t.Fatalf("seed item habito: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO item (id, tipo, titulo, data, horarios, estado, ordem, criado_em, atualizado_em)
		VALUES (2, 'compromisso', 'Churrasco UMP', '2026-10-03', '[]', 'ativo', 0, ?, ?)`, agora, agora); err != nil {
		t.Fatalf("seed item compromisso: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO ocorrencia (id, item_id, inicio, fim, meta, status) VALUES (1, 1, '2026-09-26', '2026-09-26', 1, 'aberta')`); err != nil {
		t.Fatalf("seed ocorrencia: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO registro (id, item_id, em, origem) VALUES (1, 1, ?, 'app')`, agora); err != nil {
		t.Fatalf("seed registro: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO notificacao (id, item_id, ocorrencia_id, tipo, agendada_para, status)
		VALUES (1, 2, NULL, 'lembrete', '2026-10-03T15:00:00-03:00', 'pendente')`); err != nil {
		t.Fatalf("seed notificacao (Churrasco UMP): %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("fechar banco pré-migração: %v", err)
	}

	// reabre pelo mecanismo normal (Store.Open) — aplica 0002 por cima.
	s := abrirTeste2(t, path)
	ctx := t.Context()

	it1, err := itemBuscar(ctx, s.db, 1)
	if err != nil || it1.ID != 1 || it1.Titulo != "ler" {
		t.Fatalf("item 1 não sobreviveu: %+v, err=%v", it1, err)
	}
	it2, err := itemBuscar(ctx, s.db, 2)
	if err != nil || it2.ID != 2 || it2.Titulo != "Churrasco UMP" {
		t.Fatalf("item 2 não sobreviveu: %+v, err=%v", it2, err)
	}
	oc, err := ocorrenciaBuscar(ctx, s.db, 1)
	if err != nil || oc.ID != 1 {
		t.Fatalf("ocorrencia 1 não sobreviveu: %+v, err=%v", oc, err)
	}
	reg, err := registroBuscar(ctx, s.db, 1)
	if err != nil || reg.ID != 1 {
		t.Fatalf("registro 1 não sobreviveu: %+v, err=%v", reg, err)
	}
	n, err := notificacaoBuscar(ctx, s.db, 1)
	if err != nil {
		t.Fatalf("notificacao 1 não sobreviveu: %v", err)
	}
	if n.ID != 1 || n.ItemID == nil || *n.ItemID != 2 || n.ListaID != nil || n.Status != domain.StatusNotifPendente {
		t.Fatalf("notificacao 1 (Churrasco UMP) pós-migração = %+v", n)
	}

	// lista "Mercado" + as 10 seções, nesta ordem.
	listas, err := listaListar(ctx, s.db, false)
	if err != nil {
		t.Fatalf("listaListar: %v", err)
	}
	var mercado *domain.Lista
	for i := range listas {
		if listas[i].Nome == "Mercado" {
			mercado = &listas[i]
		}
	}
	if mercado == nil {
		t.Fatalf("lista Mercado não existe: %+v", listas)
	}
	secoes, err := secoesPorLista(ctx, s.db, mercado.ID)
	if err != nil {
		t.Fatalf("secoesPorLista: %v", err)
	}
	wantSecoes := []string{
		"Hortifrúti", "Padaria", "Açougue e peixaria", "Frios e laticínios", "Mercearia",
		"Bebidas", "Congelados", "Limpeza", "Higiene", "Outros",
	}
	if len(secoes) != len(wantSecoes) {
		t.Fatalf("secoesPorLista(Mercado) tem %d, want %d: %+v", len(secoes), len(wantSecoes), secoes)
	}
	for i, nome := range wantSecoes {
		if secoes[i].Nome != nome {
			t.Fatalf("secao[%d] = %q, want %q", i, secoes[i].Nome, nome)
		}
	}

	// CHECK (item_id IS NULL) <> (lista_id IS NULL): nem os dois, nem nenhum.
	if _, err := s.db.Exec(`INSERT INTO notificacao (item_id, lista_id, tipo, agendada_para, status)
		VALUES (2, ?, 'lembrete', '2026-10-04T09:00:00-03:00', 'pendente')`, mercado.ID); err == nil {
		t.Fatal("esperava erro: item_id e lista_id juntos deveriam violar o CHECK")
	}
	if _, err := s.db.Exec(`INSERT INTO notificacao (tipo, agendada_para, status)
		VALUES ('lembrete', '2026-10-04T09:00:00-03:00', 'pendente')`); err == nil {
		t.Fatal("esperava erro: nem item_id nem lista_id deveria violar o CHECK")
	}

	// índice parcial de item: duplicar (item_id, agendada_para, tipo) falha.
	if _, err := s.db.Exec(`INSERT INTO notificacao (item_id, tipo, agendada_para, status)
		VALUES (2, 'lembrete', '2026-10-03T15:00:00-03:00', 'pendente')`); err == nil {
		t.Fatal("esperava erro: duplicar lembrete de item deveria violar o índice único parcial")
	}

	// índice parcial de lista: primeira ok, duplicar falha.
	if _, err := s.db.Exec(`INSERT INTO notificacao (lista_id, tipo, agendada_para, status)
		VALUES (?, 'lista', '2026-10-04T09:00:00-03:00', 'pendente')`, mercado.ID); err != nil {
		t.Fatalf("primeira notificação de lista devia ser aceita: %v", err)
	}
	if _, err := s.db.Exec(`INSERT INTO notificacao (lista_id, tipo, agendada_para, status)
		VALUES (?, 'lista', '2026-10-04T09:00:00-03:00', 'pendente')`, mercado.ID); err == nil {
		t.Fatal("esperava erro: duplicar lembrete de lista deveria violar o índice único parcial")
	}
}
