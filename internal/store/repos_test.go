package store

import (
	"testing"
	"time"

	"pocket/internal/domain"
)

func TestItemInserirBuscarAtualizar(t *testing.T) {
	s := abrirTeste(t)
	ctx := t.Context()

	agora := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	regra := &domain.Regra{Tipo: domain.RegraDiario}
	it := domain.Item{
		Tipo: domain.TipoHabito, Titulo: "ler", Regra: regra, Horarios: []string{"21:00"},
		Estado: domain.EstadoAtivo, CriadoEm: agora, AtualizadoEm: agora,
	}

	id, err := itemInserir(ctx, s.db, it)
	if err != nil {
		t.Fatalf("itemInserir: %v", err)
	}

	lido, err := itemBuscar(ctx, s.db, id)
	if err != nil {
		t.Fatalf("itemBuscar: %v", err)
	}
	if lido.Titulo != "ler" || lido.Regra == nil || lido.Regra.Tipo != domain.RegraDiario {
		t.Fatalf("item lido = %+v", lido)
	}
	if len(lido.Horarios) != 1 || lido.Horarios[0] != "21:00" {
		t.Fatalf("horarios lidos = %v", lido.Horarios)
	}
	if !lido.CriadoEm.Equal(agora) {
		t.Errorf("criado_em = %v, want %v", lido.CriadoEm, agora)
	}

	lido.Titulo = "ler mais"
	lido.AtualizadoEm = agora.Add(time.Hour)
	if err := itemAtualizar(ctx, s.db, lido); err != nil {
		t.Fatalf("itemAtualizar: %v", err)
	}
	relido, err := itemBuscar(ctx, s.db, id)
	if err != nil {
		t.Fatalf("itemBuscar após atualizar: %v", err)
	}
	if relido.Titulo != "ler mais" {
		t.Fatalf("titulo após atualizar = %q", relido.Titulo)
	}

	if _, err := itemBuscar(ctx, s.db, 999); !isErrNaoEncontrado(err) {
		t.Fatalf("itemBuscar inexistente: err = %v, want ErrNaoEncontrado", err)
	}
}

func TestOcorrenciaInserirConflitoNaoDuplica(t *testing.T) {
	s := abrirTeste(t)
	ctx := t.Context()

	itemID := criarItemTeste(t, s)
	dia, _ := domain.ParseDia("2026-09-28")
	oc := domain.Ocorrencia{ItemID: itemID, Inicio: &dia, Fim: &dia, Meta: 1, Status: domain.StatusAberta}

	id1, ok1, err := ocorrenciaInserir(ctx, s.db, oc)
	if err != nil || !ok1 || id1 == 0 {
		t.Fatalf("primeira inserção: id=%d ok=%v err=%v", id1, ok1, err)
	}
	id2, ok2, err := ocorrenciaInserir(ctx, s.db, oc)
	if err != nil {
		t.Fatalf("segunda inserção: %v", err)
	}
	if ok2 {
		t.Fatalf("segunda inserção com mesmo (item_id, inicio) devia ser ignorada (ON CONFLICT DO NOTHING), id=%d", id2)
	}

	todas, err := ocorrenciasPorItem(ctx, s.db, itemID)
	if err != nil {
		t.Fatalf("ocorrenciasPorItem: %v", err)
	}
	if len(todas) != 1 {
		t.Fatalf("ocorrencias do item = %d, want 1 (UNIQUE deveria ter bloqueado a duplicata)", len(todas))
	}
}

func TestOcorrenciaAtualizarEApagar(t *testing.T) {
	s := abrirTeste(t)
	ctx := t.Context()

	itemID := criarItemTeste(t, s)
	dia, _ := domain.ParseDia("2026-09-28")
	oc := domain.Ocorrencia{ItemID: itemID, Inicio: &dia, Fim: &dia, Meta: 1, Status: domain.StatusAberta}
	id, _, err := ocorrenciaInserir(ctx, s.db, oc)
	if err != nil {
		t.Fatalf("inserir: %v", err)
	}

	oc.ID = id
	oc.Status = domain.StatusCumprida
	if err := ocorrenciaAtualizar(ctx, s.db, oc); err != nil {
		t.Fatalf("atualizar: %v", err)
	}
	lida, err := ocorrenciaBuscar(ctx, s.db, id)
	if err != nil {
		t.Fatalf("buscar: %v", err)
	}
	if lida.Status != domain.StatusCumprida {
		t.Fatalf("status = %q, want cumprida", lida.Status)
	}

	if err := ocorrenciaApagar(ctx, s.db, id); err != nil {
		t.Fatalf("apagar: %v", err)
	}
	if _, err := ocorrenciaBuscar(ctx, s.db, id); !isErrNaoEncontrado(err) {
		t.Fatalf("buscar após apagar: err = %v, want ErrNaoEncontrado", err)
	}
}

func TestRegistroInserirBuscarUltimosApagar(t *testing.T) {
	s := abrirTeste(t)
	ctx := t.Context()

	itemID := criarItemTeste(t, s)
	base := time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC)
	var ultimoID int64
	for i := 0; i < 3; i++ {
		id, err := registroInserir(ctx, s.db, domain.Registro{
			ItemID: itemID, Em: base.Add(time.Duration(i) * time.Hour), Origem: domain.OrigemApp,
		})
		if err != nil {
			t.Fatalf("registroInserir %d: %v", i, err)
		}
		ultimoID = id
	}

	todos, err := registrosPorItem(ctx, s.db, itemID)
	if err != nil || len(todos) != 3 {
		t.Fatalf("registrosPorItem: %v, %d", err, len(todos))
	}

	ultimos, err := registrosUltimos(ctx, s.db, itemID, 2)
	if err != nil {
		t.Fatalf("registrosUltimos: %v", err)
	}
	if len(ultimos) != 2 || ultimos[0].ID != ultimoID {
		t.Fatalf("registrosUltimos = %+v, want o mais recente primeiro", ultimos)
	}

	if err := registroApagar(ctx, s.db, ultimoID); err != nil {
		t.Fatalf("registroApagar: %v", err)
	}
	if _, err := registroBuscar(ctx, s.db, ultimoID); !isErrNaoEncontrado(err) {
		t.Fatalf("registroBuscar após apagar: err = %v, want ErrNaoEncontrado", err)
	}
}

func TestJobExecutadoEMarcar(t *testing.T) {
	s := abrirTeste(t)
	ctx := t.Context()

	ok, err := jobExecutado(ctx, s.db, "gerar_ocorrencias", "2026-09-28")
	if err != nil {
		t.Fatalf("jobExecutado: %v", err)
	}
	if ok {
		t.Fatalf("job não deveria estar marcado ainda")
	}

	if err := jobMarcar(ctx, s.db, "gerar_ocorrencias", "2026-09-28", "2026-09-28T10:00:00-03:00"); err != nil {
		t.Fatalf("jobMarcar: %v", err)
	}
	ok, err = jobExecutado(ctx, s.db, "gerar_ocorrencias", "2026-09-28")
	if err != nil || !ok {
		t.Fatalf("job deveria estar marcado: ok=%v err=%v", ok, err)
	}
}

func TestWithTxExpostoRollback(t *testing.T) {
	s := abrirTeste(t)
	itemID := criarItemTeste(t, s)

	err := s.WithTx(t.Context(), func(tx *Tx) error {
		dia, _ := domain.ParseDia("2026-09-28")
		if _, _, err := tx.OcorrenciaInserir(t.Context(), domain.Ocorrencia{
			ItemID: itemID, Inicio: &dia, Fim: &dia, Meta: 1, Status: domain.StatusAberta,
		}); err != nil {
			return err
		}
		return errRollbackTeste
	})
	if err == nil {
		t.Fatal("esperava erro propagado de WithTx")
	}

	ocs, err := ocorrenciasPorItem(t.Context(), s.db, itemID)
	if err != nil {
		t.Fatalf("ocorrenciasPorItem: %v", err)
	}
	if len(ocs) != 0 {
		t.Fatalf("rollback não desfez o insert dentro de WithTx: %d ocorrencias", len(ocs))
	}
}

// --- auxiliares de teste ---

var errRollbackTeste = errRollback{}

type errRollback struct{}

func (errRollback) Error() string { return "rollback proposital" }

func criarItemTeste(t *testing.T, s *Store) int64 {
	t.Helper()
	agora := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	id, err := itemInserir(t.Context(), s.db, domain.Item{
		Tipo: domain.TipoHabito, Titulo: "hábito de teste", Regra: &domain.Regra{Tipo: domain.RegraDiario},
		Estado: domain.EstadoAtivo, CriadoEm: agora, AtualizadoEm: agora,
	})
	if err != nil {
		t.Fatalf("criarItemTeste: %v", err)
	}
	return id
}

func isErrNaoEncontrado(err error) bool {
	_, ok := err.(ErrNaoEncontrado)
	return ok
}
