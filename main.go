// Comando pocket: hábitos, compromissos e lembretes com Web Push
// (CONTRATO.md) — PWA instalado no Poco. A fase 1 substitui por inteiro o
// spike da fase 0 (FASE-0-SPIKE.md); certLoader e o formato de
// secrets/vapid.json são os únicos pedaços preservados (ver internal/ops).
package main

import (
	"context"
	"crypto/tls"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"pocket/internal/app"
	"pocket/internal/captura"
	"pocket/internal/clock"
	"pocket/internal/config"
	"pocket/internal/httpapi"
	"pocket/internal/ops"
	"pocket/internal/push"
	"pocket/internal/scheduler"
	"pocket/internal/store"

	"github.com/joho/godotenv"
)

// versao é preenchida no build: go build -ldflags "-X main.versao=$(git rev-parse --short HEAD)".
var versao = "dev"

// distFS embute o build do front (servicos/pocket/frontend/ → dist/,
// commitado — CONTRATO §2/§6). O diretivo fica aqui, não em
// internal/httpapi, porque go:embed só aceita caminhos dentro da árvore do
// próprio pacote e dist/ é vizinho deste arquivo, não de internal/httpapi.
//
//go:embed all:dist
var distFS embed.FS

func main() {
	// DECISÃO: texto (não JSON) em stdout — o contrato só pede "slog em
	// stdout"; texto é mais fácil de ler direto no log do tmux/journal.
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("encerrando com erro", "erro", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	// .env na própria pasta do serviço, como o Smaug (CONTRATO §8).
	// Ausente = ok, segue com as variáveis do ambiente real.
	_ = godotenv.Load()

	cfg, err := config.LoadConfigFrom(func(k string) (string, bool) { return os.LookupEnv(k) })
	if err != nil {
		return err
	}
	// Android não tem /etc/localtime: sem isto o horário das linhas de log
	// (slog usa time.Local) sai em UTC. O domínio já usa cfg.Location.
	time.Local = cfg.Location

	clk := clock.Real{}

	st, err := store.Open(cfg.DB, clk)
	if err != nil {
		return err
	}
	defer st.Close()

	certPath := filepath.Join(cfg.Secrets, "tls", "fullchain.pem")
	keyPath := filepath.Join(cfg.Secrets, "tls", "key.pem")
	certLoader := ops.NewCertLoader(certPath, keyPath)
	if _, err := certLoader.Get(nil); err != nil {
		return err
	}

	// DECISÃO: log 1x no boot se a renovação está desligada (CONTRATO §7/
	// etapa 5) — o job em si (internal/ops.Jobs) nem checa a tabela job
	// quando cfg.ACME == "", então não haveria outro lugar natural pra isto.
	if cfg.ACME == "" {
		logger.Info("renovação de certificado desligada (POCKET_ACME vazio)")
	}

	// VAPID: lido de secrets/vapid.json no formato do spike (gera se
	// ausente) — preserva a inscrição já feita no Poco (CONTRATO §7).
	vapidKeys, err := push.CarregarOuGerarVAPID(cfg.Secrets)
	if err != nil {
		return fmt.Errorf("carregar/gerar VAPID: %w", err)
	}
	pusher := push.NewWebPush(vapidKeys, cfg.PushSubject)

	obs := app.NovoObservadorNotificacoes(clk, cfg.Location)
	ap := app.New(st, clk, cfg.Location, obs)
	ap.SetPush(pusher, vapidKeys.Public)

	// Captura por voz/texto (fase 1.5, CONTRATO-FASE15.md §7): só liga com
	// GROQ_API_KEY preenchida — vazia é o default (V4: reusa a chave do
	// Smaug, a cota é por organização) e, nesse caso, GET /api/captura
	// devolve ativa:false sem SetCaptura ter sido chamado.
	if cfg.GroqAPIKey != "" {
		transcritor := captura.NovoGroqTranscritor(cfg.GroqAPIKey, cfg.TranscricaoModelo)
		transcritor.URL = cfg.GroqURL + "/audio/transcriptions"
		interpretador := captura.NovoGroqInterpretador(cfg.GroqAPIKey, cfg.CapturaModelo)
		interpretador.URL = cfg.GroqURL + "/chat/completions"
		ap.SetCaptura(transcritor, interpretador, cfg.CapturaModelo)
	} else {
		logger.Info("captura por voz/texto desligada (GROQ_API_KEY vazia)")
	}

	// DECISÃO (Opus, decisão 11): erro aqui é falha de banco — melhor
	// encerrar do que subir servindo /api/hoje quebrado. O scheduler (etapa
	// 4) roda o mesmo job a cada tick, mas o boot precisa de um também: sem
	// isto, um serviço que nunca recebe um tick (crash antes do primeiro)
	// subiria sem as ocorrências/notificações de hoje.
	if err := ap.RodarJobsDiarios(context.Background()); err != nil {
		return fmt.Errorf("rodar jobs diários no boot: %w", err)
	}

	api := httpapi.New(st, ap, versao)

	// SPA embutida (etapa 6): fs.Sub reduz distFS à raiz de dist/, para
	// index.html ficar em "index.html" e não em "dist/index.html".
	dist, err := fs.Sub(distFS, "dist")
	if err != nil {
		return fmt.Errorf("abrir dist embutido: %w", err)
	}
	api.SetEstatico(dist)

	// Scheduler: tick de 30s (e um no boot) lendo o banco — nada de timer
	// por item em memória (lição do A5). Sobe numa goroutine própria, presa
	// a ctx; schedulerFim fecha quando Executar retorna, para o shutdown
	// poder esperar por ela (encerramento limpo).
	sch := scheduler.New(st, ap, pusher, clk, cfg.Location)

	// Jobs de operação (etapa 5: renovar_cert, alerta_cert, backup) — o
	// scheduler os dispara a cada tick sem bloquear (ops.Jobs.Tick roda em
	// goroutine própria; ver a DECISÃO lá sobre por que renovar_cert não
	// pode travar o envio de notificações).
	opsJobs := ops.NewJobs(st, ops.ExecutorReal{}, pusher, certLoader, clk, cfg.Location, cfg.ACME, cfg.DB)
	sch.SetOpsJobs(opsJobs)

	// GET /health completo (etapa 5): validade do cert + último tick do
	// scheduler, além de {ok, versao, subs} que já existiam.
	api.SetSaude(certLoader, sch)

	srv := &http.Server{
		Handler:           api,
		TLSConfig:         &tls.Config{GetCertificate: certLoader.Get, MinVersion: tls.VersionTLS12},
		ReadHeaderTimeout: 10 * time.Second,
	}

	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	schedulerFim := make(chan struct{})
	go func() {
		defer close(schedulerFim)
		sch.Executar(ctx)
	}()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ServeTLS(ln, "", "")
	}()
	logger.Info("pocket no ar", "endereco", cfg.Listen, "versao", versao, "tz", cfg.TZ)

	select {
	case <-ctx.Done():
		logger.Info("sinal recebido, encerrando")
		// DECISÃO: 10s de prazo para o Shutdown — o contrato não fixa um
		// valor; tempo generoso o bastante para requisições em voo sem
		// segurar o restart do deploy.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		<-schedulerFim // ctx já está Done; o scheduler termina o tick em voo e sai.
		return nil
	case err := <-errCh:
		stop() // também encerra o scheduler, preso ao mesmo ctx.
		<-schedulerFim
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
