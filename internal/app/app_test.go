package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pocket/internal/clock"
	"pocket/internal/domain"
	"pocket/internal/store"
)

// segunda28 é uma segunda-feira fixa usada nos testes (§12 do pedido: datas
// fixas, 2026-09-28 é segunda).
var segunda28 = time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)

func novaAppTeste(t *testing.T, inicio time.Time) (*App, *clock.Fake) {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	dir := t.TempDir()
	fake := clock.NewFake(inicio.In(loc))
	st, err := store.Open(filepath.Join(dir, "pocket.db"), fake)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return New(st, fake, loc, nil), fake
}

func TestCriarItemHabitoDiarioGeraOcorrenciaDeHoje(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()

	item, err := a.CriarItem(ctx, ItemEntrada{
		Tipo: domain.TipoHabito, Titulo: "ler", Regra: []byte(`{"tipo":"diario"}`),
	})
	if err != nil {
		t.Fatalf("CriarItem: %v", err)
	}

	hoje, err := a.Hoje(ctx)
	if err != nil {
		t.Fatalf("Hoje: %v", err)
	}
	if len(hoje.Habitos) != 1 || hoje.Habitos[0].Item.ID != item.ID {
		t.Fatalf("Hoje.Habitos = %+v, want 1 hábito (%d)", hoje.Habitos, item.ID)
	}
	if hoje.Habitos[0].Ocorrencia.Status != domain.StatusAberta {
		t.Fatalf("status = %q, want aberta", hoje.Habitos[0].Ocorrencia.Status)
	}
}

func TestRegistrarEDesfazerDiario(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()

	item, err := a.CriarItem(ctx, ItemEntrada{
		Tipo: domain.TipoHabito, Titulo: "ler", Regra: []byte(`{"tipo":"diario"}`),
	})
	if err != nil {
		t.Fatalf("CriarItem: %v", err)
	}

	cm, reg, err := a.Registrar(ctx, item.ID, RegistroEntrada{Origem: domain.OrigemApp})
	if err != nil {
		t.Fatalf("Registrar: %v", err)
	}
	if cm.Metricas == nil || cm.Metricas.SequenciaAtual != 1 {
		t.Fatalf("metricas = %+v, want sequencia 1", cm.Metricas)
	}

	// a ocorrência do dia virou cumprida (OcorrenciaAberta é nil agora —
	// só reporta ocorrência ainda "aberta", e esta não é mais).
	ocs, err := a.st.OcorrenciasPorItem(ctx, item.ID)
	if err != nil || len(ocs) != 1 || ocs[0].Status != domain.StatusCumprida {
		t.Fatalf("ocorrencias = %+v, err = %v, want 1 cumprida", ocs, err)
	}
	detalhe, err := a.DetalheItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("DetalheItem: %v", err)
	}
	if detalhe.OcorrenciaAberta != nil {
		t.Fatalf("ocorrencia aberta = %+v, want nil (já cumprida)", detalhe.OcorrenciaAberta)
	}

	cm2, err := a.DesfazerRegistro(ctx, reg.ID)
	if err != nil {
		t.Fatalf("DesfazerRegistro: %v", err)
	}
	if cm2.Metricas.SequenciaAtual != 0 {
		t.Fatalf("sequencia após desfazer = %d, want 0", cm2.Metricas.SequenciaAtual)
	}
}

func TestVezesPeriodoFeitosDeMeta(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()

	item, err := a.CriarItem(ctx, ItemEntrada{
		Tipo: domain.TipoHabito, Titulo: "academia",
		Regra: []byte(`{"tipo":"vezes_periodo","vezes":3,"periodo":"semana"}`),
	})
	if err != nil {
		t.Fatalf("CriarItem: %v", err)
	}

	for i, wantFeitos := range []int{1, 2, 3} {
		_, _, err := a.Registrar(ctx, item.ID, RegistroEntrada{})
		if err != nil {
			t.Fatalf("Registrar %d: %v", i, err)
		}
		hoje, err := a.Hoje(ctx)
		if err != nil {
			t.Fatalf("Hoje %d: %v", i, err)
		}
		if len(hoje.Habitos) != 1 {
			t.Fatalf("Hoje.Habitos (%d) = %+v", i, hoje.Habitos)
		}
		if hoje.Habitos[0].Feitos != wantFeitos {
			t.Fatalf("feitos (%d) = %d, want %d", i, hoje.Habitos[0].Feitos, wantFeitos)
		}
		wantStatus := domain.StatusAberta
		if wantFeitos == 3 {
			wantStatus = domain.StatusCumprida
		}
		if hoje.Habitos[0].Ocorrencia.Status != wantStatus {
			t.Fatalf("status (%d) = %q, want %q", i, hoje.Habitos[0].Ocorrencia.Status, wantStatus)
		}
	}
}

func TestIntervaloRegistrarAvancaEDesfazerReabre(t *testing.T) {
	a, fake := novaAppTeste(t, segunda28)
	ctx := context.Background()

	item, err := a.CriarItem(ctx, ItemEntrada{
		Tipo: domain.TipoHabito, Titulo: "regar plantas",
		Regra: []byte(`{"tipo":"intervalo","dias":3}`),
	})
	if err != nil {
		t.Fatalf("CriarItem: %v", err)
	}

	_, reg, err := a.Registrar(ctx, item.ID, RegistroEntrada{})
	if err != nil {
		t.Fatalf("Registrar: %v", err)
	}

	detalhe, err := a.DetalheItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("DetalheItem: %v", err)
	}
	if detalhe.OcorrenciaAberta == nil {
		t.Fatal("esperava ocorrencia aberta (a próxima, em dia)")
	}
	wantInicio := domain.Dia("2026-10-01") // 28/09 + 3 dias
	if *detalhe.OcorrenciaAberta.Inicio != wantInicio {
		t.Fatalf("inicio da próxima = %s, want %s", *detalhe.OcorrenciaAberta.Inicio, wantInicio)
	}

	// avança o relógio para o dia em que a próxima vence e confirma "hoje"
	// não mostra o hábito ainda (só quando inicio <= hoje).
	fake.Advance(2 * 24 * time.Hour) // 30/09: ainda "em dia"
	hoje, err := a.Hoje(ctx)
	if err != nil {
		t.Fatalf("Hoje: %v", err)
	}
	if len(hoje.Habitos) != 0 {
		t.Fatalf("Hoje antes do vencimento = %+v, want vazio", hoje.Habitos)
	}

	fake.Advance(24 * time.Hour) // 01/10: vence
	hoje, err = a.Hoje(ctx)
	if err != nil {
		t.Fatalf("Hoje: %v", err)
	}
	if len(hoje.Habitos) != 1 {
		t.Fatalf("Hoje no vencimento = %+v, want 1", hoje.Habitos)
	}

	// desfazer o primeiro registro deve reabrir a cumprida original e
	// apagar a próxima (sem registros).
	cm, err := a.DesfazerRegistro(ctx, reg.ID)
	if err != nil {
		t.Fatalf("DesfazerRegistro: %v", err)
	}
	detalhe2, err := a.DetalheItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("DetalheItem após desfazer: %v", err)
	}
	if detalhe2.OcorrenciaAberta == nil || *detalhe2.OcorrenciaAberta.Inicio != "2026-09-28" {
		t.Fatalf("ocorrencia aberta após desfazer = %+v, want inicio 2026-09-28", detalhe2.OcorrenciaAberta)
	}
	_ = cm
}

func TestMudarEstadoHabitoPausarPulaAbertasEArquivarBloqueiaTransicaoInvalida(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()

	item, err := a.CriarItem(ctx, ItemEntrada{
		Tipo: domain.TipoHabito, Titulo: "ler", Regra: []byte(`{"tipo":"diario"}`),
	})
	if err != nil {
		t.Fatalf("CriarItem: %v", err)
	}

	pausado, err := a.MudarEstado(ctx, item.ID, domain.EstadoPausado)
	if err != nil {
		t.Fatalf("MudarEstado pausar: %v", err)
	}
	if pausado.Estado != domain.EstadoPausado {
		t.Fatalf("estado = %q, want pausado", pausado.Estado)
	}

	detalhe, err := a.DetalheItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("DetalheItem: %v", err)
	}
	if len(detalhe.Registros) != 0 {
		t.Fatalf("registros = %+v, want vazio", detalhe.Registros)
	}
	// a ocorrência de hoje devia ter virado pulada ao pausar.
	ocs, err := a.st.OcorrenciasPorItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("OcorrenciasPorItem: %v", err)
	}
	if len(ocs) != 1 || ocs[0].Status != domain.StatusPulada {
		t.Fatalf("ocorrencias = %+v, want 1 pulada", ocs)
	}

	_, err = a.MudarEstado(ctx, item.ID, domain.EstadoPausado)
	if err == nil {
		t.Fatal("pausado -> pausado deveria ser 409")
	}
	if _, ok := err.(ErrConflito); !ok {
		t.Fatalf("erro = %T, want ErrConflito", err)
	}
}

func TestEditarNotaRegistro(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()
	item, err := a.CriarItem(ctx, ItemEntrada{
		Tipo: domain.TipoHabito, Titulo: "devocional", Regra: []byte(`{"tipo":"diario"}`),
	})
	if err != nil {
		t.Fatalf("CriarItem: %v", err)
	}
	cm, reg, err := a.Registrar(ctx, item.ID, RegistroEntrada{Origem: domain.OrigemApp})
	if err != nil {
		t.Fatalf("Registrar: %v", err)
	}
	seqAntes := cm.Metricas.SequenciaAtual

	got, err := a.EditarNotaRegistro(ctx, reg.ID, "  Salmo 23  ")
	if err != nil || got.Nota != "Salmo 23" {
		t.Fatalf("EditarNota = %+v, %v; want nota aparada", got, err)
	}
	// apagar a nota
	if got, err = a.EditarNotaRegistro(ctx, reg.ID, ""); err != nil || got.Nota != "" {
		t.Fatalf("apagar nota = %+v, %v", got, err)
	}
	// longa demais → 400; inexistente → 404
	if _, err := a.EditarNotaRegistro(ctx, reg.ID, strings.Repeat("á", notaMax+1)); !errors.As(err, new(ErrValidacao)) {
		t.Fatalf("nota longa: err = %v, want validação", err)
	}
	if _, err := a.EditarNotaRegistro(ctx, 9999, "x"); !errors.As(err, new(ErrNaoEncontrado)) {
		t.Fatalf("registro inexistente: err = %v, want não encontrado", err)
	}
	// nota não mexe em métrica
	det, err := a.DetalheItem(ctx, item.ID)
	if err != nil || det.Metricas.SequenciaAtual != seqAntes {
		t.Fatalf("sequência mudou: %+v, %v", det, err)
	}
}
