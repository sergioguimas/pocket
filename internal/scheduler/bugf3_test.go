package scheduler

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"pocket/internal/app"
	"pocket/internal/domain"
	"pocket/internal/store"
)

// forcarEstadoItem muda item.Estado diretamente pelo store, sem passar por
// app.MudarEstado — simula a corrida de produção que motivou este teste: o
// item deixou de estar ativo (ou a ocorrência ligada deixou de estar
// aberta) por um caminho que não passou pelo Observador de notificações,
// então a notificação já agendada continua "pendente" no banco até o
// scheduler tentar mandá-la.
func forcarEstadoItem(t *testing.T, e *ambiente, itemID int64, estado string) {
	t.Helper()
	err := e.st.WithTx(context.Background(), func(tx *store.Tx) error {
		it, err := tx.ItemBuscar(context.Background(), itemID)
		if err != nil {
			return err
		}
		it.Estado = estado
		return tx.ItemAtualizar(context.Background(), it)
	})
	if err != nil {
		t.Fatalf("forcarEstadoItem(%d, %q): %v", itemID, estado, err)
	}
}

// TestSchedulerCancelaLembreteDeItemConcluidoNoEnvio é a segunda camada de
// defesa do bug de produção corrigido em F3-1 (a primeira é
// app.notificacaoAindaAcionavel, exercida em internal/app/push_test.go):
// mesmo que uma notificação continue "pendente" para um item que já não
// está mais ativo (corrida — o item mudou de estado por fora do
// Observador), o scheduler não deve mandar o push no momento do envio —
// só cancelar.
func TestSchedulerCancelaLembreteDeItemConcluidoNoEnvio(t *testing.T) {
	loc := mustLoc(t)
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	d := time.Date(2026, 9, 28, 8, 0, 0, 0, loc)
	e := novoAmbiente(t, dbPath, d)
	inscreverPush(t, e, "https://push.exemplo/1")

	item, err := e.ap.CriarItem(context.Background(), app.ItemEntrada{
		Tipo: domain.TipoCompromisso, Titulo: "C10 teste",
		Data: "2026-09-28", Hora: "11:35",
	})
	if err != nil {
		t.Fatalf("CriarItem: %v", err)
	}

	pendentesAntes := pendentesDoItem(t, e, item.ID, d.Add(24*time.Hour))
	if len(pendentesAntes) != 1 {
		t.Fatalf("pendentes após criar = %+v, want 1", pendentesAntes)
	}
	notifID := pendentesAntes[0].ID

	// corrida: item concluído sem passar pelo Observador (a notificação
	// das 11:35 continua "pendente" no banco).
	forcarEstadoItem(t, e, item.ID, domain.EstadoConcluido)

	e.fake.Set(time.Date(2026, 9, 28, 11, 35, 0, 0, loc))
	e.sch.Tick(context.Background())

	if n := e.pusher.Contagem(); n != 0 {
		t.Fatalf("pusher recebeu %d envios, want 0 (item concluído deveria só cancelar no envio)", n)
	}

	n, err := e.st.NotificacaoBuscar(context.Background(), notifID)
	if err != nil {
		t.Fatalf("NotificacaoBuscar: %v", err)
	}
	if n.Status != domain.StatusNotifCancelada {
		t.Fatalf("notificacao.Status = %q, want %q", n.Status, domain.StatusNotifCancelada)
	}
}

// TestSchedulerCancelaLembreteDeHabitoPausadoNoEnvio é o mesmo teste para
// hábito pausado (em vez de compromisso concluído) — outro jeito de o item
// deixar de estar "ativo" entre o agendamento e o envio.
func TestSchedulerCancelaLembreteDeHabitoPausadoNoEnvio(t *testing.T) {
	loc := mustLoc(t)
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	d := time.Date(2026, 9, 28, 8, 0, 0, 0, loc)
	e := novoAmbiente(t, dbPath, d)
	inscreverPush(t, e, "https://push.exemplo/1")

	item := criarHabitoDiario(t, e, "ler", "10:05")

	pendentesAntes := pendentesDoItem(t, e, item.ID, d.Add(24*time.Hour))
	if len(pendentesAntes) != 1 {
		t.Fatalf("pendentes após criar = %+v, want 1", pendentesAntes)
	}
	notifID := pendentesAntes[0].ID

	// corrida: hábito pausado sem passar pelo Observador.
	forcarEstadoItem(t, e, item.ID, domain.EstadoPausado)

	e.fake.Set(time.Date(2026, 9, 28, 10, 5, 0, 0, loc))
	e.sch.Tick(context.Background())

	if n := e.pusher.Contagem(); n != 0 {
		t.Fatalf("pusher recebeu %d envios, want 0 (hábito pausado deveria só cancelar no envio)", n)
	}

	n, err := e.st.NotificacaoBuscar(context.Background(), notifID)
	if err != nil {
		t.Fatalf("NotificacaoBuscar: %v", err)
	}
	if n.Status != domain.StatusNotifCancelada {
		t.Fatalf("notificacao.Status = %q, want %q", n.Status, domain.StatusNotifCancelada)
	}
}
