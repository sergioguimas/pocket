package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"pocket/internal/clock"
	"pocket/internal/domain"
	"pocket/internal/push"
	"pocket/internal/store"
)

// Nomes dos jobs diários de operação — mesmo mecanismo (nome, dia) das
// ocorrências (internal/app/jobs.go), tabela `job` (nome, data_ref).
const (
	nomeJobRenovarCert = "renovar_cert"
	nomeJobAlertaCert  = "alerta_cert"
	nomeJobBackup      = "backup"
)

// timeoutRenovarCert é o prazo do "$POCKET_ACME --cron --home <dir>" (CONTRATO §7).
const timeoutRenovarCert = 10 * time.Minute

// backoffRenovarCert e tentativasMaxRenovarCert limitam quantas vezes
// renovar_cert tenta de novo no mesmo dia depois de falhar (DECISÃO Opus,
// etapa 5, revisão do coordenador): sem isto, um tick de 30s tentando de
// novo a cada falha faria ~120 tentativas/hora — cada uma cria um TXT na
// Hostinger e um pedido de validação na CA. A Let's Encrypt limita 5
// validações falhas/hora por conta+hostname; o Pocket se auto-bloquearia
// bem antes de o problema real (Hostinger/CA fora do ar) se resolver.
const (
	backoffRenovarCert       = time.Hour
	tentativasMaxRenovarCert = 4
)

// limiarAlertaCert é o "< 20 dias" de §7 que dispara o push de alerta.
const limiarAlertaCert = 20 * 24 * time.Hour

// backupManterN é quantos backups diários o job mantém (CONTRATO §7).
const backupManterN = 7

// Jobs reúne as tarefas diárias de operação da fase 1/etapa 5: renovação do
// certificado, alerta de validade e backup do banco. Cada uma é idempotente
// por (nome, dia), como o job gerar_ocorrencias da etapa 3.
type Jobs struct {
	st     *store.Store
	exec   Executor
	pusher push.Pusher
	cert   *CertLoader
	clock  clock.Clock
	loc    *time.Location

	acme      string // caminho do acme.sh; vazio = renovação desligada (config.go já normaliza isso)
	backupDir string // POCKET_DB dir + "/backup" (CONTRATO §7: relativo ao POCKET_DB)

	// mu impede duas execuções de RodarJobsDiarios em paralelo — o acme.sh
	// pode levar até 10min, e rodar dois de uma vez não tem sentido (o
	// segundo só reencontraria o job já marcado e devolveria cedo mesmo
	// assim, mas evita dois processos acme.sh concorrentes no mesmo diretório).
	mu sync.Mutex

	// Estado do backoff de renovar_cert (DECISÃO acima) — só em memória:
	// um restart zera as tentativas do dia, o que é aceitável (o pior caso é
	// "mais uma tentativa hoje", nunca menos seguro que o limite).
	renovarMu              sync.Mutex
	renovarDia             string // dataRef do dia sendo contado; muda -> zera tentativas
	renovarTentativas      int
	renovarUltimaTentativa time.Time
	renovarLimiteLogado    bool // já logou "limite atingido" hoje? evita logar a cada tick
}

// NewJobs cria Jobs. dbPath é o mesmo caminho de config.Config.DB — o
// diretório de backup é relativo a ele (CONTRATO §7). cert pode ser nil em
// testes que não exercitam alerta_cert.
func NewJobs(st *store.Store, exec Executor, pusher push.Pusher, cert *CertLoader, c clock.Clock, loc *time.Location, acme, dbPath string) *Jobs {
	return &Jobs{
		st: st, exec: exec, pusher: pusher, cert: cert, clock: c, loc: loc,
		acme:      acme,
		backupDir: filepath.Join(filepath.Dir(dbPath), "backup"),
	}
}

// Tick dispara RodarJobsDiarios numa goroutine própria, sem bloquear o
// chamador.
//
// DECISÃO (Opus, etapa 5): renovar_cert pode levar até 10 min (timeout) —
// rodá-lo síncrono dentro de scheduler.Tick travaria o envio de
// notificações pendentes pelo mesmo tempo (o scheduler só tem uma
// goroutine). TryLock garante no máximo uma execução por vez: se já tem uma
// rodando (ex.: ticks de 30s durante os 10 min do acme.sh), os ticks
// seguintes só a ignoram — a goroutine em voo continua, e o próximo tick
// depois que ela terminar dispara de novo (idempotente: (nome, dia) já
// marcado faz o trabalho de verdade só rodar 1×/dia). A goroutine herda o
// ctx do chamador (o ctx do processo, cancelado no shutdown) — um shutdown
// no meio do acme.sh cancela o exec em vez de deixá-lo órfão.
func (j *Jobs) Tick(ctx context.Context) {
	if !j.mu.TryLock() {
		return
	}
	go func() {
		defer j.mu.Unlock()
		if err := j.RodarJobsDiarios(ctx); err != nil {
			slog.Error("rodar jobs de operação", "erro", err)
		}
	}()
}

// RodarJobsDiarios roda os três jobs de operação. Cada um decide sozinho se
// já rodou hoje (via a tabela job); um erro num job é logado e não impede os
// outros dois (são independentes entre si).
func (j *Jobs) RodarJobsDiarios(ctx context.Context) error {
	hoje := domain.Hoje(j.clock, j.loc)
	dataRef := string(hoje)

	if err := j.renovarCert(ctx, dataRef); err != nil {
		slog.Error("job renovar_cert", "erro", err)
	}
	if err := j.alertaCert(ctx, dataRef); err != nil {
		slog.Error("job alerta_cert", "erro", err)
	}
	if err := j.backup(ctx, dataRef); err != nil {
		slog.Error("job backup", "erro", err)
	}
	return nil
}

// marcarJob grava (nome, dataRef) na tabela job — só depois de sucesso
// (mesmo padrão de app.RodarJobsDiarios): se o job falhar no meio, a
// próxima chamada tenta de novo.
func (j *Jobs) marcarJob(ctx context.Context, nome, dataRef string) error {
	return j.st.WithTx(ctx, func(tx *store.Tx) error {
		return tx.JobMarcar(ctx, nome, dataRef, domain.FormatInstante(j.clock.Now().In(j.loc)))
	})
}

// renovarCert roda "$POCKET_ACME --cron --home <dir do acme>" (CONTRATO
// §7), no máximo 1×/dia em caso de sucesso. POCKET_ACME vazio desliga a
// renovação — nem chega a checar o job (0 chamadas ao Executor, nunca).
//
// Falha: tenta de novo no máximo 1×/hora (backoffRenovarCert) e no máximo
// tentativasMaxRenovarCert vezes por dia — ver a DECISÃO na constante.
func (j *Jobs) renovarCert(ctx context.Context, dataRef string) error {
	if j.acme == "" {
		return nil
	}

	ja, err := j.st.JobExecutado(ctx, nomeJobRenovarCert, dataRef)
	if err != nil {
		return err
	}
	if ja {
		return nil
	}

	if !j.podeTentarRenovarCert(dataRef) {
		return nil
	}

	home := filepath.Dir(j.acme)
	saida, err := j.exec.Executar(ctx, timeoutRenovarCert, j.acme, "--cron", "--home", home)
	if err != nil {
		return fmt.Errorf("acme.sh --cron --home %s: %w (saída: %s)", home, err, resumoSaida(saida))
	}
	slog.Info("renovar_cert rodou", "home", home, "saida", resumoSaida(saida))

	// Força o certLoader a checar o arquivo agora, em vez de esperar a
	// próxima requisição TLS — assim alertaCert (chamado logo depois, na
	// mesma RodarJobsDiarios) já vê a validade nova se o acme.sh renovou.
	if j.cert != nil {
		if _, err := j.cert.Get(nil); err != nil {
			slog.Warn("recarregar certificado após renovar_cert", "erro", err)
		}
	}

	return j.marcarJob(ctx, nomeJobRenovarCert, dataRef)
}

// podeTentarRenovarCert aplica o backoff (1×/hora, 4×/dia) sobre o estado em
// memória de Jobs. dataRef diferente do dia contado zera o contador (dia
// novo, cota nova). Chamado só quando o job ainda não teve sucesso hoje.
func (j *Jobs) podeTentarRenovarCert(dataRef string) bool {
	agora := j.clock.Now().In(j.loc)

	j.renovarMu.Lock()
	defer j.renovarMu.Unlock()

	if j.renovarDia != dataRef {
		j.renovarDia = dataRef
		j.renovarTentativas = 0
		j.renovarUltimaTentativa = time.Time{}
		j.renovarLimiteLogado = false
	}

	if j.renovarTentativas >= tentativasMaxRenovarCert {
		if !j.renovarLimiteLogado {
			j.renovarLimiteLogado = true
			slog.Warn("renovar_cert: limite de tentativas do dia atingido, tentando de novo amanhã",
				"tentativas", j.renovarTentativas)
		}
		return false
	}

	if !j.renovarUltimaTentativa.IsZero() && agora.Sub(j.renovarUltimaTentativa) < backoffRenovarCert {
		return false
	}

	j.renovarTentativas++
	j.renovarUltimaTentativa = agora
	return true
}

// resumoSaida corta a saída do acme.sh nas últimas linhas — "loga a saída
// resumida, sem segredos" (CONTRATO §7). DECISÃO (Opus, etapa 5): confia
// que o acme.sh não imprime segredos em modo normal (sem --debug, que este
// job não passa) — dns_hostinger lê a API key das variáveis de ambiente,
// não a ecoa no log; resumir para as últimas linhas é só para não inundar o
// log num dia de erro verboso.
func resumoSaida(saida string) string {
	const maxLinhas = 20
	linhas := strings.Split(strings.TrimRight(saida, "\n"), "\n")
	if len(linhas) > maxLinhas {
		linhas = linhas[len(linhas)-maxLinhas:]
	}
	return strings.Join(linhas, " | ")
}

// alertaPayload é o payload do push de alerta de certificado — mesmo
// formato de campos do payload de lembrete (scheduler) e do teste
// (app.TestarPush), para o sw.js não precisar de um caso especial:
// {notificacao_id, item_id, title, body}. notificacao_id/item_id ficam 0
// (CONTRATO §7: "sem linha em notificacao").
type alertaPayload struct {
	NotificacaoID int64  `json:"notificacao_id"`
	ItemID        int64  `json:"item_id"`
	Title         string `json:"title"`
	Body          string `json:"body"`
}

// alertaCert manda 1 push/dia, direto pelo Pusher (sem linha em
// notificacao — lá item_id é obrigatório), quando o certificado vence em
// menos de 20 dias. A unicidade diária vem do job (nome "alerta_cert", dia).
func (j *Jobs) alertaCert(ctx context.Context, dataRef string) error {
	if j.cert == nil {
		return nil
	}
	validade, err := j.cert.Validade()
	if err != nil {
		// certificado ainda não carregado (boot muito cedo) — nada a alertar.
		return nil
	}

	agora := j.clock.Now().In(j.loc)
	restante := validade.Sub(agora)
	if restante >= limiarAlertaCert {
		return nil
	}
	dias := int(restante.Hours() / 24)
	if dias < 0 {
		dias = 0
	}

	ja, err := j.st.JobExecutado(ctx, nomeJobAlertaCert, dataRef)
	if err != nil {
		return err
	}
	if ja {
		return nil
	}
	if j.pusher == nil {
		return nil
	}

	// DECISÃO (Opus, etapa 5): título carrega o texto literal do contrato
	// ("Pocket: certificado vence em N dias"), corpo carrega a dica — mesmo
	// split título curto/corpo com a ação de scheduler.montarPayload.
	payload, err := json.Marshal(alertaPayload{
		Title: fmt.Sprintf("Pocket: certificado vence em %d dias", dias),
		Body:  "confira o log do renovar_cert",
	})
	if err != nil {
		return err
	}

	subs, err := j.st.PushSubListar(ctx)
	if err != nil {
		return err
	}
	for _, sub := range subs {
		status, errEnv := j.pusher.Enviar(push.Subscription{Endpoint: sub.Endpoint, P256dh: sub.P256dh, Auth: sub.Auth}, payload)
		switch {
		case errEnv != nil:
			slog.Error("enviar alerta de certificado", "endpoint", sub.Endpoint, "erro", errEnv)
		case status == http.StatusNotFound || status == http.StatusGone:
			if err := j.st.PushSubRemover(ctx, sub.Endpoint); err != nil {
				slog.Error("remover inscrição push morta (alerta cert)", "endpoint", sub.Endpoint, "erro", err)
			}
		}
	}

	return j.marcarJob(ctx, nomeJobAlertaCert, dataRef)
}

// backup roda "VACUUM INTO data/backup/pocket-YYYY-MM-DD.db" (CONTRATO §7),
// no máximo 1×/dia, e mantém os 7 mais recentes.
func (j *Jobs) backup(ctx context.Context, dataRef string) error {
	ja, err := j.st.JobExecutado(ctx, nomeJobBackup, dataRef)
	if err != nil {
		return err
	}
	if ja {
		return nil
	}

	if err := os.MkdirAll(j.backupDir, 0o700); err != nil {
		return fmt.Errorf("criar diretório de backup (%s): %w", j.backupDir, err)
	}

	caminho := filepath.Join(j.backupDir, fmt.Sprintf("pocket-%s.db", dataRef))
	if _, err := os.Stat(caminho); err == nil {
		// Arquivo do dia já existe (CONTRATO §7: "não refaz") — só marca o
		// job, sem rodar VACUUM INTO de novo.
		return j.marcarJob(ctx, nomeJobBackup, dataRef)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checar backup existente (%s): %w", caminho, err)
	}

	if err := j.st.VacuumInto(ctx, caminho); err != nil {
		return fmt.Errorf("VACUUM INTO %s: %w", caminho, err)
	}

	if err := rotacionarBackups(j.backupDir, backupManterN); err != nil {
		// O backup do dia já foi feito com sucesso — uma falha ao apagar os
		// antigos não deve impedir marcar o job (senão o backup de hoje
		// seria refeito amanhã em vão, já que o arquivo já existe).
		slog.Error("rotacionar backups", "erro", err)
	}

	return j.marcarJob(ctx, nomeJobBackup, dataRef)
}

// rotacionarBackups mantém os `manter` arquivos "pocket-YYYY-MM-DD.db" mais
// recentes em dir, apagando os demais. Ordenação lexicográfica de string já
// é cronológica (mesmo truque de domain.Dia — nomes são YYYY-MM-DD).
func rotacionarBackups(dir string, manter int) error {
	entradas, err := filepath.Glob(filepath.Join(dir, "pocket-*.db"))
	if err != nil {
		return fmt.Errorf("listar backups: %w", err)
	}
	sort.Strings(entradas)
	if len(entradas) <= manter {
		return nil
	}
	for _, antigo := range entradas[:len(entradas)-manter] {
		if err := os.Remove(antigo); err != nil {
			return fmt.Errorf("apagar backup antigo (%s): %w", antigo, err)
		}
	}
	return nil
}
