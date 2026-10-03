package scheduler

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"pocket/internal/app"
	"pocket/internal/clock"
	"pocket/internal/domain"
	"pocket/internal/push"
	"pocket/internal/store"
)

// segunda28 é uma segunda-feira fixa (mesma data usada em app_test.go) às
// 10:00, em loc.
func segunda28(loc *time.Location) time.Time {
	return time.Date(2026, 9, 28, 10, 0, 0, 0, loc)
}

// ambiente reúne o que cada teste precisa: banco em arquivo (não memória —
// C10 reabre o mesmo arquivo simulando um restart), App já com o
// Observador de notificações, relógio e Pusher falsos, e o Scheduler.
type ambiente struct {
	dbPath string
	loc    *time.Location
	st     *store.Store
	ap     *app.App
	fake   *clock.Fake
	pusher *push.Fake
	sch    *Scheduler
}

func novoAmbiente(t *testing.T, dbPath string, inicio time.Time) *ambiente {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	fake := clock.NewFake(inicio.In(loc))
	st, err := store.Open(dbPath, fake)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	obs := app.NovoObservadorNotificacoes(fake, loc)
	ap := app.New(st, fake, loc, obs)
	pusher := &push.Fake{}
	ap.SetPush(pusher, "chave-publica-teste")

	if err := ap.RodarJobsDiarios(context.Background()); err != nil {
		t.Fatalf("RodarJobsDiarios: %v", err)
	}

	sch := New(st, ap, pusher, fake, loc)
	return &ambiente{dbPath: dbPath, loc: loc, st: st, ap: ap, fake: fake, pusher: pusher, sch: sch}
}

func criarHabitoDiario(t *testing.T, e *ambiente, titulo, horario string) domain.Item {
	t.Helper()
	item, err := e.ap.CriarItem(context.Background(), app.ItemEntrada{
		Tipo: domain.TipoHabito, Titulo: titulo,
		Regra: []byte(`{"tipo":"diario"}`), Horarios: []string{horario},
	})
	if err != nil {
		t.Fatalf("CriarItem: %v", err)
	}
	return item
}

func inscreverPush(t *testing.T, e *ambiente, endpoint string) {
	t.Helper()
	if err := e.ap.InscreverPush(context.Background(), app.PushSubEntrada{
		Endpoint: endpoint, P256dh: "p256dh-" + endpoint, Auth: "auth-" + endpoint,
	}); err != nil {
		t.Fatalf("InscreverPush: %v", err)
	}
}

// TestTickEnviaNoMinutoCerto é C7: lembrete chega no horário (± tolerância
// aqui é exata, já que o relógio é falso) com o app "fechado" — ninguém
// além do tick manda o push.
func TestTickEnviaNoMinutoCerto(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	e := novoAmbiente(t, dbPath, segunda28(mustLoc(t)))
	inscreverPush(t, e, "https://push.exemplo/1")
	criarHabitoDiario(t, e, "ler", "10:05")

	e.sch.Tick(context.Background()) // antes do horário: nada a enviar
	if n := e.pusher.Contagem(); n != 0 {
		t.Fatalf("envios antes do horário = %d, want 0", n)
	}

	e.fake.Advance(5 * time.Minute) // 10:05
	e.sch.Tick(context.Background())

	if n := e.pusher.Contagem(); n != 1 {
		t.Fatalf("envios às 10:05 = %d, want 1", n)
	}
}

// TestAdiarReenviaTrintaMinutosDepois é C8.
func TestAdiarReenviaTrintaMinutosDepois(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	e := novoAmbiente(t, dbPath, segunda28(mustLoc(t)))
	inscreverPush(t, e, "https://push.exemplo/1")
	item := criarHabitoDiario(t, e, "ler", "10:05")

	e.fake.Advance(5 * time.Minute)
	e.sch.Tick(context.Background())
	if n := e.pusher.Contagem(); n != 1 {
		t.Fatalf("envios às 10:05 = %d, want 1", n)
	}

	notifs, err := e.st.NotificacoesPendentesAte(context.Background(), domain.FormatInstante(e.fake.Now()))
	if err != nil {
		t.Fatalf("NotificacoesPendentesAte: %v", err)
	}
	if len(notifs) != 0 {
		t.Fatalf("ainda há pendentes = %+v, want nenhuma (já enviada)", notifs)
	}

	// Acha o id da notificação enviada pelo item (via calendário/consulta
	// direta não existe — usa a última notificação do próprio pusher: o
	// payload carrega notificacao_id).
	notifID := ultimoNotificacaoID(t, e)

	if err := e.ap.AcaoNotificacao(context.Background(), notifID, domain.AcaoAdiar); err != nil {
		t.Fatalf("AcaoNotificacao adiar: %v", err)
	}

	e.fake.Advance(29 * time.Minute)
	e.sch.Tick(context.Background())
	if n := e.pusher.Contagem(); n != 1 {
		t.Fatalf("envios antes dos 30min = %d, want 1 (ainda só o primeiro)", n)
	}

	e.fake.Advance(time.Minute) // completa os 30min do adiamento
	e.sch.Tick(context.Background())
	if n := e.pusher.Contagem(); n != 2 {
		t.Fatalf("envios após adiar 30min = %d, want 2", n)
	}
	_ = item
}

// TestOcorrenciaCumpridaCancelaProximoLembrete é C9: hábito já cumprido não
// recebe o lembrete do horário seguinte.
func TestOcorrenciaCumpridaCancelaProximoLembrete(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	e := novoAmbiente(t, dbPath, segunda28(mustLoc(t)))
	inscreverPush(t, e, "https://push.exemplo/1")
	item := criarHabitoDiario(t, e, "ler", "10:05")

	// marca feito antes do horário do lembrete — a ocorrência vira cumprida
	// e o Observador (OcorrenciaMudou) cancela a notificação pendente.
	if _, _, err := e.ap.Registrar(context.Background(), item.ID, app.RegistroEntrada{}); err != nil {
		t.Fatalf("Registrar: %v", err)
	}

	e.fake.Advance(5 * time.Minute) // 10:05, horário do lembrete cancelado
	e.sch.Tick(context.Background())

	if n := e.pusher.Contagem(); n != 0 {
		t.Fatalf("envios após cumprir antes do horário = %d, want 0", n)
	}
}

// TestNovoAppSchedulerSobreMesmoBancoEnviaLembretePendente é C10: deploy
// (restart) com lembrete pendente para daqui a alguns minutos — o lembrete
// chega mesmo assim, porque a fila está inteira no banco, não em memória.
func TestNovoAppSchedulerSobreMesmoBancoEnviaLembretePendente(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	inicio := segunda28(mustLoc(t))

	e1 := novoAmbiente(t, dbPath, inicio)
	inscreverPush(t, e1, "https://push.exemplo/1")
	criarHabitoDiario(t, e1, "ler", "10:05") // 5 min à frente
	if err := e1.st.Close(); err != nil {
		t.Fatalf("fechar store 1: %v", err)
	}

	// "restart": novo Store/App/Scheduler sobre o mesmo arquivo, relógio
	// novo já em 10:05 (como se o processo tivesse ficado fora até lá).
	e2 := novoAmbiente(t, dbPath, inicio.Add(5*time.Minute))
	e2.sch.Tick(context.Background())

	if n := e2.pusher.Contagem(); n != 1 {
		t.Fatalf("envios após restart no horário do lembrete = %d, want 1", n)
	}
}

// TestRelogioSaltaUmaHoraExpiraSemEnviar é C11: serviço parado por 1h — ao
// voltar, lembretes vencidos há mais de 30min ficam "expirada" e não
// chegam em rajada.
func TestRelogioSaltaUmaHoraExpiraSemEnviar(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	e := novoAmbiente(t, dbPath, segunda28(mustLoc(t)))
	inscreverPush(t, e, "https://push.exemplo/1")
	criarHabitoDiario(t, e, "ler", "10:05")

	e.fake.Advance(time.Hour) // 11:00 — 55 min depois do horário do lembrete
	e.sch.Tick(context.Background())

	if n := e.pusher.Contagem(); n != 0 {
		t.Fatalf("envios após salto de 1h = %d, want 0 (deveria expirar, não enviar)", n)
	}
}

// TestPush410RemoveInscricao: o scheduler remove a inscrição quando o
// serviço de push devolve 410 (Gone).
func TestPush410RemoveInscricao(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	e := novoAmbiente(t, dbPath, segunda28(mustLoc(t)))
	endpoint := "https://push.exemplo/morta"
	inscreverPush(t, e, endpoint)
	e.pusher.Resultado = func(sub push.Subscription, payload []byte) (int, error) {
		return http.StatusGone, nil
	}
	criarHabitoDiario(t, e, "ler", "10:05")

	e.fake.Advance(5 * time.Minute)
	e.sch.Tick(context.Background())

	subs, err := e.st.PushSubListar(context.Background())
	if err != nil {
		t.Fatalf("PushSubListar: %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("inscrições restantes = %+v, want nenhuma (410 remove)", subs)
	}
}

// TestTickConcorrenteNaoEnviaDuasVezes: dois ticks concorrentes sobre a
// mesma notificação vencida só mandam um push — a reivindicação
// (UPDATE ... WHERE status='pendente') é o que garante isso no banco.
func TestTickConcorrenteNaoEnviaDuasVezes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	e := novoAmbiente(t, dbPath, segunda28(mustLoc(t)))
	inscreverPush(t, e, "https://push.exemplo/1")
	criarHabitoDiario(t, e, "ler", "10:05")
	e.fake.Advance(5 * time.Minute)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.sch.Tick(context.Background())
		}()
	}
	wg.Wait()

	if n := e.pusher.Contagem(); n != 1 {
		t.Fatalf("envios com 5 ticks concorrentes = %d, want 1", n)
	}
}

// TestNotificacoesAntecedenciaCompromisso é o fluxo ponta a ponta de um
// compromisso com antecedências (§3.5): D às 08:00, compromisso em D+1
// 15:00 com horarios ["-1d","-15m"]. O job de D só deve gerar o lembrete
// que cai em D (a antecedência -1d, D 15:00) — não a de D+1 14:45 (-15m,
// que só existe quando "hoje" for D+1) nem a da própria hora (D+1 15:00).
// Só quando o dia vira e o job de D+1 roda é que os outros dois nascem.
func TestNotificacoesAntecedenciaCompromisso(t *testing.T) {
	loc := mustLoc(t)
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	d0800 := time.Date(2026, 9, 28, 8, 0, 0, 0, loc)
	dQuinze := time.Date(2026, 9, 28, 15, 0, 0, 0, loc)
	d1Quinze := time.Date(2026, 9, 29, 15, 0, 0, 0, loc)
	d1MenosQuinze := time.Date(2026, 9, 29, 14, 45, 0, 0, loc)

	e := novoAmbiente(t, dbPath, d0800)
	inscreverPush(t, e, "https://push.exemplo/1")

	item, err := e.ap.CriarItem(context.Background(), app.ItemEntrada{
		Tipo: domain.TipoCompromisso, Titulo: "dentista",
		Data: "2026-09-29", Hora: "15:00", Horarios: []string{"-1d", "-15m"},
	})
	if err != nil {
		t.Fatalf("CriarItem: %v", err)
	}

	// logo após criar (job de D + ItemMudou já rodaram): só a antecedência
	// -1d (D 15:00) deve estar pendente; a -15m (D+1 14:45) ainda não.
	pendentes := pendentesDoItem(t, e, item.ID, d1Quinze.Add(time.Minute))
	if !contemAgendada(pendentes, dQuinze) {
		t.Fatalf("faltou o lembrete de D 15:00 (-1d) entre as pendentes: %+v", pendentes)
	}
	if contemAgendada(pendentes, d1MenosQuinze) {
		t.Fatalf("lembrete de D+1 14:45 (-15m) já existe antes do job de D+1 rodar: %+v", pendentes)
	}

	// D 15:00: envia a antecedência -1d.
	e.fake.Set(dQuinze)
	e.sch.Tick(context.Background())
	if n := e.pusher.Contagem(); n != 1 {
		t.Fatalf("envios em D 15:00 = %d, want 1", n)
	}

	// D+1 00:05: o dia virou — o job diário (rodado pelo Tick) gera a hora
	// e a -15m de D+1. Nada vencido ainda, nenhum envio novo.
	e.fake.Set(time.Date(2026, 9, 29, 0, 5, 0, 0, loc))
	e.sch.Tick(context.Background())
	if n := e.pusher.Contagem(); n != 1 {
		t.Fatalf("envios logo após virar o dia = %d, want 1 (nada vencido ainda)", n)
	}

	// D+1 14:45: envia a antecedência -15m (a da hora, D+1 15:00, ainda não
	// venceu).
	e.fake.Set(d1MenosQuinze)
	e.sch.Tick(context.Background())
	if n := e.pusher.Contagem(); n != 2 {
		t.Fatalf("envios em D+1 14:45 = %d, want 2", n)
	}

	// D+1 15:00: envia o lembrete da própria hora.
	e.fake.Set(d1Quinze)
	e.sch.Tick(context.Background())
	if n := e.pusher.Contagem(); n != 3 {
		t.Fatalf("envios em D+1 15:00 = %d, want 3", n)
	}
}

// TestConcluirCompromissoCancelaPendentesDoDia: registrar (concluir) o
// compromisso antes do horário cancela as notificações ainda pendentes —
// nada é enviado depois (mesmo caminho de C9, aplicado a compromisso).
func TestConcluirCompromissoCancelaPendentesDoDia(t *testing.T) {
	loc := mustLoc(t)
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	d0800 := time.Date(2026, 9, 28, 8, 0, 0, 0, loc)

	e := novoAmbiente(t, dbPath, d0800)
	inscreverPush(t, e, "https://push.exemplo/1")

	item, err := e.ap.CriarItem(context.Background(), app.ItemEntrada{
		Tipo: domain.TipoCompromisso, Titulo: "dentista",
		Data: "2026-09-29", Hora: "15:00", Horarios: []string{"-1d", "-15m"},
	})
	if err != nil {
		t.Fatalf("CriarItem: %v", err)
	}

	// D 15:00: envia a antecedência -1d.
	e.fake.Set(time.Date(2026, 9, 28, 15, 0, 0, 0, loc))
	e.sch.Tick(context.Background())
	if n := e.pusher.Contagem(); n != 1 {
		t.Fatalf("envios em D 15:00 = %d, want 1", n)
	}

	// D+1 00:05: o job diário gera a hora e a -15m de D+1.
	e.fake.Set(time.Date(2026, 9, 29, 0, 5, 0, 0, loc))
	e.sch.Tick(context.Background())

	// D+1 10:00: usuário conclui o compromisso antes do horário — a
	// ocorrência vira cumprida, o item vira concluido; o Observador cancela
	// os pendentes de hoje.
	e.fake.Set(time.Date(2026, 9, 29, 10, 0, 0, 0, loc))
	if _, _, err := e.ap.Registrar(context.Background(), item.ID, app.RegistroEntrada{}); err != nil {
		t.Fatalf("Registrar: %v", err)
	}

	pendentes := pendentesDoItem(t, e, item.ID, time.Date(2026, 9, 29, 23, 59, 0, 0, loc))
	if len(pendentes) != 0 {
		t.Fatalf("ainda há pendente do item concluído: %+v", pendentes)
	}

	// D+1 14:45 e D+1 15:00: nada é enviado (só o envio de D 15:00 continua).
	e.fake.Set(time.Date(2026, 9, 29, 14, 45, 0, 0, loc))
	e.sch.Tick(context.Background())
	e.fake.Set(time.Date(2026, 9, 29, 15, 0, 0, 0, loc))
	e.sch.Tick(context.Background())
	if n := e.pusher.Contagem(); n != 1 {
		t.Fatalf("envios totais = %d, want 1 (nada depois de concluir)", n)
	}
}

// pendentesDoItem lê as notificações "pendente" (agendada_para <= ate) e
// filtra pelas de item — os testes deste pacote não têm um
// NotificacoesPorItem dedicado, e não precisam de mais que isso.
func pendentesDoItem(t *testing.T, e *ambiente, itemID int64, ate time.Time) []domain.Notificacao {
	t.Helper()
	todas, err := e.st.NotificacoesPendentesAte(context.Background(), domain.FormatInstante(ate))
	if err != nil {
		t.Fatalf("NotificacoesPendentesAte: %v", err)
	}
	var out []domain.Notificacao
	for _, n := range todas {
		if n.ItemID != nil && *n.ItemID == itemID {
			out = append(out, n)
		}
	}
	return out
}

func contemAgendada(ns []domain.Notificacao, quando time.Time) bool {
	alvo := domain.FormatInstante(quando)
	for _, n := range ns {
		if n.AgendadaPara == alvo {
			return true
		}
	}
	return false
}

func mustLoc(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	return loc
}

// opsJobsFake é o OpsJobs falso dos testes deste arquivo (etapa 5) — só
// conta quantas vezes Tick foi chamado, sem executar nada de verdade.
type opsJobsFake struct {
	mu       sync.Mutex
	chamadas int
}

func (f *opsJobsFake) Tick(context.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chamadas++
}

func (f *opsJobsFake) Contagem() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.chamadas
}

// TestUltimoTickAtualizaACadaTick: GET /health (etapa 5) usa UltimoTick
// para "tick_em" — precisa refletir o relógio no momento de cada Tick.
func TestUltimoTickAtualizaACadaTick(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	e := novoAmbiente(t, dbPath, segunda28(mustLoc(t)))

	if !e.sch.UltimoTick().IsZero() {
		t.Fatalf("UltimoTick antes do 1º Tick = %v, want zero", e.sch.UltimoTick())
	}

	e.sch.Tick(context.Background())
	primeiro := e.sch.UltimoTick()
	if !primeiro.Equal(e.fake.Now()) {
		t.Fatalf("UltimoTick após 1º Tick = %v, want %v", primeiro, e.fake.Now())
	}

	e.fake.Advance(time.Minute)
	e.sch.Tick(context.Background())
	segundo := e.sch.UltimoTick()
	if !segundo.Equal(e.fake.Now()) || segundo.Equal(primeiro) {
		t.Fatalf("UltimoTick após 2º Tick = %v, want %v (diferente do 1º)", segundo, e.fake.Now())
	}
}

// TestSchedulerDisparaOpsJobsACadaTickSemBloquear: scheduler.Tick chama
// OpsJobs.Tick (etapa 5) sem travar — o Fake aqui é síncrono (não simula a
// goroutine de ops.Jobs de verdade), então basta checar que foi chamado.
func TestSchedulerDisparaOpsJobsACadaTickSemBloquear(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	e := novoAmbiente(t, dbPath, segunda28(mustLoc(t)))
	ops := &opsJobsFake{}
	e.sch.SetOpsJobs(ops)

	e.sch.Tick(context.Background())
	e.sch.Tick(context.Background())

	if n := ops.Contagem(); n != 2 {
		t.Fatalf("chamadas a OpsJobs.Tick = %d, want 2 (uma por Tick do scheduler)", n)
	}
}

// ultimoNotificacaoID lê o payload do último envio registrado pelo Fake e
// devolve o notificacao_id (os testes deste pacote não têm acesso a um
// NotificacoesPorItem — o payload já carrega o id, é o jeito mais direto).
func ultimoNotificacaoID(t *testing.T, e *ambiente) int64 {
	t.Helper()
	e.pusher.Resultado = nil
	if len(e.pusher.Envios) == 0 {
		t.Fatalf("nenhum envio registrado")
	}
	var p payload
	if err := json.Unmarshal(e.pusher.Envios[len(e.pusher.Envios)-1].Payload, &p); err != nil {
		t.Fatalf("decodificar payload: %v", err)
	}
	return p.NotificacaoID
}
