package ops

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pocket/internal/clock"
	"pocket/internal/domain"
	"pocket/internal/push"
	"pocket/internal/store"
)

// ambiente reúne o que cada teste deste arquivo precisa: banco em arquivo
// (t.TempDir()), relógio falso, executor e pusher falsos (regra da casa: sem
// rede/exec real nos testes).
type ambiente struct {
	st     *store.Store
	fake   *clock.Fake
	loc    *time.Location
	exec   *ExecutorFake
	pusher *push.Fake
	dbPath string
}

func novoAmbiente(t *testing.T, inicio time.Time) *ambiente {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	fake := clock.NewFake(inicio.In(loc))
	dbPath := filepath.Join(t.TempDir(), "pocket.db")
	st, err := store.Open(dbPath, fake)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	return &ambiente{st: st, fake: fake, loc: loc, exec: &ExecutorFake{}, pusher: &push.Fake{}, dbPath: dbPath}
}

// certComValidade cria um CertLoader carregado com um certificado
// autoassinado cujo NotAfter é validade (helper gerarCertTeste vem de
// tls_test.go, mesmo pacote).
func certComValidade(t *testing.T, validade time.Time) *CertLoader {
	t.Helper()
	certPath, keyPath := gerarCertTeste(t, t.TempDir(), validade)
	l := NewCertLoader(certPath, keyPath)
	if _, err := l.Get(nil); err != nil {
		t.Fatalf("carregar certificado de teste: %v", err)
	}
	return l
}

func inscreverPush(t *testing.T, e *ambiente, endpoint string) {
	t.Helper()
	if err := e.st.PushSubUpsert(context.Background(), domain.PushSub{
		Endpoint: endpoint, P256dh: "p256dh-" + endpoint, Auth: "auth-" + endpoint,
		CriadoEm: domain.FormatInstante(e.fake.Now()),
	}); err != nil {
		t.Fatalf("PushSubUpsert: %v", err)
	}
}

// certLonge é uma validade confortavelmente acima do limiar de 20 dias —
// usada nos testes de renovar_cert/backup para o alerta nunca disparar
// junto e confundir a contagem de envios do pusher.
func certLonge(inicio time.Time) time.Time {
	return inicio.Add(60 * 24 * time.Hour)
}

// TestRenovarCertChamaExecutorUmaVezPorDia: o executor roda 1× por dia,
// mesmo chamando RodarJobsDiarios várias vezes no mesmo dia (idempotente por
// (nome, dia)); no dia seguinte, roda de novo.
func TestRenovarCertChamaExecutorUmaVezPorDia(t *testing.T) {
	inicio := time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC)
	e := novoAmbiente(t, inicio)
	cert := certComValidade(t, certLonge(inicio))
	acme := filepath.Join(t.TempDir(), ".acme.sh", "acme.sh")
	j := NewJobs(e.st, e.exec, e.pusher, cert, e.fake, e.loc, acme, e.dbPath)

	if err := j.RodarJobsDiarios(context.Background()); err != nil {
		t.Fatalf("RodarJobsDiarios (1ª vez): %v", err)
	}
	if err := j.RodarJobsDiarios(context.Background()); err != nil {
		t.Fatalf("RodarJobsDiarios (2ª vez, mesmo dia): %v", err)
	}
	if n := e.exec.Contagem(); n != 1 {
		t.Fatalf("chamadas ao executor no mesmo dia = %d, want 1", n)
	}

	chamada := e.exec.Chamadas[0]
	if chamada.Nome != acme {
		t.Errorf("nome do comando = %q, want %q", chamada.Nome, acme)
	}
	wantArgs := []string{"--cron", "--home", filepath.Dir(acme)}
	if len(chamada.Args) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", chamada.Args, wantArgs)
	}
	for i, a := range wantArgs {
		if chamada.Args[i] != a {
			t.Errorf("args[%d] = %q, want %q", i, chamada.Args[i], a)
		}
	}
	if chamada.Timeout != timeoutRenovarCert {
		t.Errorf("timeout = %v, want %v", chamada.Timeout, timeoutRenovarCert)
	}

	e.fake.Advance(24 * time.Hour)
	if err := j.RodarJobsDiarios(context.Background()); err != nil {
		t.Fatalf("RodarJobsDiarios (dia seguinte): %v", err)
	}
	if n := e.exec.Contagem(); n != 2 {
		t.Fatalf("chamadas ao executor após virar o dia = %d, want 2", n)
	}
}

// TestRenovarCertBackoffAposFalha cobre o limite de tentativas do
// coordenador (revisão da etapa 5): falha não pode virar um tick de 30s
// martelando o acme.sh — no máximo 1 tentativa/hora e 4/dia.
func TestRenovarCertBackoffAposFalha(t *testing.T) {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	inicio := time.Date(2026, 9, 28, 0, 0, 0, 0, loc) // 00:00 local, como no pedido da revisão
	e := novoAmbiente(t, inicio)
	cert := certComValidade(t, certLonge(inicio))
	acme := filepath.Join(t.TempDir(), ".acme.sh", "acme.sh")
	j := NewJobs(e.st, e.exec, e.pusher, cert, e.fake, e.loc, acme, e.dbPath)

	e.exec.Resultado = func(nome string, args ...string) (string, error) {
		return "erro simulado", errors.New("acme.sh falhou")
	}

	rodar := func() { _ = j.RodarJobsDiarios(context.Background()) }
	contagem := func() int { return e.exec.Contagem() }

	// 00:00 — 1ª tentativa (falha).
	rodar()
	if n := contagem(); n != 1 {
		t.Fatalf("após 00:00 = %d chamadas, want 1", n)
	}

	// 00:00:30 e 00:30 — dentro da mesma hora de backoff, não tentam de novo.
	e.fake.Advance(30 * time.Second)
	rodar()
	if n := contagem(); n != 1 {
		t.Fatalf("após 00:00:30 = %d chamadas, want 1 (ainda dentro do backoff de 1h)", n)
	}
	e.fake.Advance(29*time.Minute + 30*time.Second) // total: 00:30
	rodar()
	if n := contagem(); n != 1 {
		t.Fatalf("após 00:30 = %d chamadas, want 1 (ainda dentro do backoff de 1h)", n)
	}

	// 01:00 — completou 1h desde a 1ª tentativa: 2ª tentativa (falha).
	e.fake.Advance(30 * time.Minute) // total: 01:00
	rodar()
	if n := contagem(); n != 2 {
		t.Fatalf("após 01:00 = %d chamadas, want 2", n)
	}

	// 02:00 e 03:00 — 3ª e 4ª tentativas (falham).
	e.fake.Advance(time.Hour) // 02:00
	rodar()
	e.fake.Advance(time.Hour) // 03:00
	rodar()
	if n := contagem(); n != 4 {
		t.Fatalf("após 03:00 = %d chamadas, want 4", n)
	}

	// 04:00 — já são 4 tentativas hoje (tentativasMaxRenovarCert): a 5ª hora
	// não tenta mais.
	e.fake.Advance(time.Hour) // 04:00
	rodar()
	if n := contagem(); n != 4 {
		t.Fatalf("após 04:00 (5ª hora) = %d chamadas, want 4 (limite diário atingido)", n)
	}

	// dia seguinte — a cota volta a valer.
	e.fake.Advance(20 * time.Hour) // total: 2026-09-29 00:00
	rodar()
	if n := contagem(); n != 5 {
		t.Fatalf("após virar o dia = %d chamadas, want 5", n)
	}
}

// TestRenovarCertDesligadoComAcmeVazio: POCKET_ACME vazio (config.go já
// normaliza "definida vazia" != "ausente") nunca chama o executor.
func TestRenovarCertDesligadoComAcmeVazio(t *testing.T) {
	inicio := time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC)
	e := novoAmbiente(t, inicio)
	cert := certComValidade(t, certLonge(inicio))
	j := NewJobs(e.st, e.exec, e.pusher, cert, e.fake, e.loc, "", e.dbPath)

	if err := j.RodarJobsDiarios(context.Background()); err != nil {
		t.Fatalf("RodarJobsDiarios: %v", err)
	}
	e.fake.Advance(24 * time.Hour)
	if err := j.RodarJobsDiarios(context.Background()); err != nil {
		t.Fatalf("RodarJobsDiarios (dia seguinte): %v", err)
	}
	if n := e.exec.Contagem(); n != 0 {
		t.Fatalf("chamadas ao executor com POCKET_ACME vazio = %d, want 0", n)
	}
}

// TestAlertaCertSoAbaixoDeVinteDias: cert com validade folgada não dispara
// alerta; abaixo de 20 dias, dispara — 1×/dia.
func TestAlertaCertSoAbaixoDeVinteDias(t *testing.T) {
	inicio := time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC)
	e := novoAmbiente(t, inicio)
	inscreverPush(t, e, "https://push.exemplo/1")

	t.Run("acima do limiar nao alerta", func(t *testing.T) {
		e2 := novoAmbiente(t, inicio)
		inscreverPush(t, e2, "https://push.exemplo/1")
		cert := certComValidade(t, inicio.Add(25*24*time.Hour))
		j := NewJobs(e2.st, e2.exec, e2.pusher, cert, e2.fake, e2.loc, "", e2.dbPath)

		if err := j.RodarJobsDiarios(context.Background()); err != nil {
			t.Fatalf("RodarJobsDiarios: %v", err)
		}
		if n := e2.pusher.Contagem(); n != 0 {
			t.Fatalf("envios com cert válido por 25 dias = %d, want 0", n)
		}
	})

	t.Run("abaixo do limiar alerta uma vez por dia", func(t *testing.T) {
		cert := certComValidade(t, inicio.Add(10*24*time.Hour))
		j := NewJobs(e.st, e.exec, e.pusher, cert, e.fake, e.loc, "", e.dbPath)

		if err := j.RodarJobsDiarios(context.Background()); err != nil {
			t.Fatalf("RodarJobsDiarios (1ª vez): %v", err)
		}
		if err := j.RodarJobsDiarios(context.Background()); err != nil {
			t.Fatalf("RodarJobsDiarios (2ª vez, mesmo dia): %v", err)
		}
		if n := e.pusher.Contagem(); n != 1 {
			t.Fatalf("envios no mesmo dia = %d, want 1", n)
		}

		var p alertaPayload
		if err := json.Unmarshal(e.pusher.Envios[0].Payload, &p); err != nil {
			t.Fatalf("decodificar payload: %v", err)
		}
		if p.Title != "Pocket: certificado vence em 10 dias" {
			t.Errorf("title = %q, want a contagem de dias restantes", p.Title)
		}

		e.fake.Advance(24 * time.Hour)
		if err := j.RodarJobsDiarios(context.Background()); err != nil {
			t.Fatalf("RodarJobsDiarios (dia seguinte): %v", err)
		}
		if n := e.pusher.Contagem(); n != 2 {
			t.Fatalf("envios após virar o dia = %d, want 2", n)
		}
	})
}

// TestBackupMantemSeteEDoDiaNaoRefeito cobre a rotação (mantém 7) e "se o
// arquivo do dia já existir, não refaz".
func TestBackupMantemSeteEDoDiaNaoRefeito(t *testing.T) {
	inicio := time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC)
	e := novoAmbiente(t, inicio)
	cert := certComValidade(t, certLonge(inicio))
	j := NewJobs(e.st, e.exec, e.pusher, cert, e.fake, e.loc, "", e.dbPath)

	// 10 dias seguidos de backup — só os 7 mais recentes devem sobrar.
	for i := 0; i < 10; i++ {
		if err := j.RodarJobsDiarios(context.Background()); err != nil {
			t.Fatalf("RodarJobsDiarios (dia %d): %v", i, err)
		}
		e.fake.Advance(24 * time.Hour)
	}

	entradas, err := filepath.Glob(filepath.Join(j.backupDir, "pocket-*.db"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(entradas) != backupManterN {
		t.Fatalf("backups restantes = %d, want %d (%v)", len(entradas), backupManterN, entradas)
	}

	// o mais antigo dos 10 dias (2026-09-20) deve ter sido apagado; o mais
	// recente antes do loop terminar (2026-09-28, dia 9 do loop) deve existir.
	if _, err := os.Stat(filepath.Join(j.backupDir, "pocket-2026-09-20.db")); !os.IsNotExist(err) {
		t.Errorf("backup do dia 1 (2026-09-20) deveria ter sido apagado, err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(j.backupDir, "pocket-2026-09-29.db")); err != nil {
		t.Errorf("backup do último dia (2026-09-29) deveria existir: %v", err)
	}

	// "backup do dia já existir, não refaz": escreve um marcador no arquivo
	// de hoje (2026-09-29, o último criado pelo loop) e roda o job de novo
	// no mesmo dia — o conteúdo não deve mudar (VACUUM INTO não roda de novo
	// sobre um arquivo já existente).
	caminhoHoje := filepath.Join(j.backupDir, "pocket-2026-09-29.db")
	if err := os.WriteFile(caminhoHoje, []byte("marcador-nao-deveria-sumir"), 0o600); err != nil {
		t.Fatalf("escrever marcador: %v", err)
	}
	if err := j.RodarJobsDiarios(context.Background()); err != nil {
		t.Fatalf("RodarJobsDiarios (mesmo dia, arquivo já existe): %v", err)
	}
	conteudo, err := os.ReadFile(caminhoHoje)
	if err != nil {
		t.Fatalf("ler backup de hoje: %v", err)
	}
	if string(conteudo) != "marcador-nao-deveria-sumir" {
		t.Errorf("conteúdo do backup de hoje mudou — foi refeito apesar de já existir")
	}
}

// TestJobsTickNaoBloqueiaEExecutaUmaVez: Tick dispara a execução em segundo
// plano (não bloqueia o chamador) e o TryLock evita duas execuções
// concorrentes — chamado várias vezes seguidas, o executor só roda 1×.
func TestJobsTickNaoBloqueiaEExecutaUmaVez(t *testing.T) {
	inicio := time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC)
	e := novoAmbiente(t, inicio)
	cert := certComValidade(t, certLonge(inicio))

	// executor lento o bastante para os Ticks concorrentes colidirem antes
	// do primeiro terminar.
	liberar := make(chan struct{})
	e.exec.Resultado = func(nome string, args ...string) (string, error) {
		<-liberar
		return "ok", nil
	}
	j := NewJobs(e.st, e.exec, e.pusher, cert, e.fake, e.loc, "/acme/acme.sh", e.dbPath)

	inicio2 := time.Now()
	j.Tick(context.Background())
	j.Tick(context.Background())
	j.Tick(context.Background())
	if decorrido := time.Since(inicio2); decorrido > time.Second {
		t.Fatalf("Tick bloqueou o chamador por %v — deveria devolver na hora", decorrido)
	}

	close(liberar)
	// dá tempo da goroutine terminar e marcar o job.
	deadline := time.Now().Add(2 * time.Second)
	for e.exec.Contagem() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := e.exec.Contagem(); n != 1 {
		t.Fatalf("chamadas ao executor = %d, want 1 (TryLock deveria ignorar os Ticks concorrentes)", n)
	}
}
