// Package scheduler envia os lembretes agendados (CONTRATO §3.5, §5) — um
// tick de 30s lendo o banco, nunca um timer por item em memória (lição do
// A5: o hook reinicia o serviço a cada deploy; a fila precisa sobreviver
// nisso, e ela sobrevive porque está inteira no banco).
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"pocket/internal/app"
	"pocket/internal/clock"
	"pocket/internal/domain"
	"pocket/internal/push"
	"pocket/internal/store"
)

// intervaloTick é a cadência do tick (§1: "scheduler (tick 30 s)").
const intervaloTick = 30 * time.Second

// tolerancia é quanto atraso uma notificação "pendente" ainda pode ter e
// ser enviada — vencida há mais que isso vira "expirada" sem envio
// (decisão A3: rajada de lembretes velhos é pior que perder um).
const tolerancia = 30 * time.Minute

// OpsJobs roda as tarefas diárias de operação (etapa 5: renovar_cert,
// alerta_cert, backup — internal/ops.Jobs) sem bloquear quem chama Tick —
// ver a DECISÃO em ops.Jobs.Tick sobre por que isto não pode ser síncrono
// aqui dentro (renovar_cert pode levar até 10 min).
type OpsJobs interface {
	Tick(ctx context.Context)
}

// Scheduler envia as notificações pendentes vencidas e roda os jobs diários
// (via a.RodarJobsDiarios, chamado a cada tick — idempotente, então virar o
// dia é só a próxima chamada encontrar o job ainda não executado).
type Scheduler struct {
	st     *store.Store
	ap     *app.App
	pusher push.Pusher
	clock  clock.Clock
	loc    *time.Location
	ops    OpsJobs // etapa 5; nil = nenhum job de operação (testes que não precisam)

	mu         sync.Mutex
	ultimoTick time.Time
}

// New cria o Scheduler.
func New(st *store.Store, ap *app.App, pusher push.Pusher, c clock.Clock, loc *time.Location) *Scheduler {
	return &Scheduler{st: st, ap: ap, pusher: pusher, clock: c, loc: loc}
}

// SetOpsJobs injeta os jobs de operação (etapa 5) — setter em vez de mais um
// parâmetro em New, mesmo padrão de App.SetPush: New(...) já é chamado sem
// isto em todo o pacote scheduler e em todos os testes das etapas 1–4.
func (s *Scheduler) SetOpsJobs(o OpsJobs) {
	s.ops = o
}

// UltimoTick devolve o instante do último Tick executado (GET /health,
// CONTRATO §5/etapa 5: "tick_em"). Zero-value antes do primeiro tick.
func (s *Scheduler) UltimoTick() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ultimoTick
}

// Executar roda um tick imediato (§9 C10: "deploy com lembrete pendente
// para daqui a 5 min: o lembrete chega mesmo assim" — precisa de um tick no
// boot, não só depois de 30s) e depois um tick a cada 30s, até ctx ser
// cancelado (main.go encerra isto no shutdown).
func (s *Scheduler) Executar(ctx context.Context) {
	s.Tick(ctx)

	ticker := time.NewTicker(intervaloTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Tick(ctx)
		}
	}
}

// Tick roda os jobs diários (idempotente — barato quando já rodaram hoje) e
// processa a fila de notificações pendentes. Exportado para os testes
// chamarem diretamente (inclusive de goroutines concorrentes, para checar
// que um tick não manda a mesma notificação duas vezes).
func (s *Scheduler) Tick(ctx context.Context) {
	s.mu.Lock()
	s.ultimoTick = s.clock.Now()
	s.mu.Unlock()

	if err := s.ap.RodarJobsDiarios(ctx); err != nil {
		slog.Error("rodar jobs diários no tick", "erro", err)
	}
	// ops.Jobs.Tick não bloqueia (dispara em goroutine própria) — ver
	// OpsJobs acima. Chamado antes de processarPendentes só para não
	// atrasar mais ainda o envio caso algum dia isto deixe de ser
	// assíncrono; a ordem em si não importa hoje.
	if s.ops != nil {
		s.ops.Tick(ctx)
	}
	if err := s.processarPendentes(ctx); err != nil {
		slog.Error("processar notificações pendentes", "erro", err)
	}
}

func (s *Scheduler) processarPendentes(ctx context.Context) error {
	agora := s.clock.Now().In(s.loc)
	pendentes, err := s.st.NotificacoesPendentesAte(ctx, domain.FormatInstante(agora))
	if err != nil {
		return err
	}

	for _, n := range pendentes {
		agendada, err := domain.ParseInstante(n.AgendadaPara)
		if err != nil {
			slog.Error("agendada_para inválida", "notificacao_id", n.ID, "valor", n.AgendadaPara, "erro", err)
			continue
		}
		if agora.Sub(agendada) > tolerancia {
			s.expirar(ctx, n.ID)
			continue
		}
		s.enviar(ctx, n, agora)
	}
	return nil
}

// expirar reivindica n como "expirada" (só se ainda "pendente" — mesma
// proteção contra tick concorrente de enviar). Vencida há mais de 30 min:
// não envia (decisão A3).
func (s *Scheduler) expirar(ctx context.Context, notificacaoID int64) {
	claimed, err := s.st.NotificacaoMarcarExpirada(ctx, notificacaoID)
	if err != nil {
		slog.Error("marcar notificação expirada", "notificacao_id", notificacaoID, "erro", err)
		return
	}
	if claimed {
		slog.Info("notificação expirada (vencida há mais de 30 min)", "notificacao_id", notificacaoID)
	}
}

// enviar reivindica n como "enviada" (o UPDATE ... WHERE status='pendente'
// é o que impede dois ticks concorrentes de mandarem o mesmo push — só um
// deles reivindica a linha) e só então chama o Pusher para cada inscrição.
func (s *Scheduler) enviar(ctx context.Context, n domain.Notificacao, agora time.Time) {
	claimed, err := s.st.NotificacaoMarcarEnviada(ctx, n.ID, domain.FormatInstante(agora))
	if err != nil {
		slog.Error("reivindicar notificação para envio", "notificacao_id", n.ID, "erro", err)
		return
	}
	if !claimed {
		return // outro tick já reivindicou esta notificação.
	}

	payload, err := s.montarPayloadOuCancelar(ctx, n)
	if err != nil {
		_ = s.st.NotificacaoMarcarFalhou(ctx, n.ID, "payload: "+err.Error())
		return
	}
	if payload == nil {
		return // já cancelada (item/ocorrência não mais válida, ou lista vazia/arquivada — ver montarPayloadOuCancelar).
	}

	subs, err := s.st.PushSubListar(ctx)
	if err != nil {
		_ = s.st.NotificacaoMarcarFalhou(ctx, n.ID, "listar inscrições: "+err.Error())
		return
	}
	if len(subs) == 0 {
		_ = s.st.NotificacaoMarcarFalhou(ctx, n.ID, "nenhuma inscrição push")
		return
	}

	agoraStr := domain.FormatInstante(agora)
	var ultimoErro string
	for _, sub := range subs {
		status, errEnv := s.pusher.Enviar(push.Subscription{Endpoint: sub.Endpoint, P256dh: sub.P256dh, Auth: sub.Auth}, payload)
		switch {
		case errEnv != nil:
			ultimoErro = errEnv.Error()
		case status == http.StatusNotFound || status == http.StatusGone:
			// inscrição morta — remove (§5/etapa 4).
			if err := s.st.PushSubRemover(ctx, sub.Endpoint); err != nil {
				slog.Error("remover inscrição push morta", "endpoint", sub.Endpoint, "erro", err)
			}
		default:
			if err := s.st.PushSubAtualizarUltimoOk(ctx, sub.Endpoint, agoraStr); err != nil {
				slog.Error("atualizar ultimo_ok da inscrição", "endpoint", sub.Endpoint, "erro", err)
			}
		}
	}
	if ultimoErro != "" {
		_ = s.st.NotificacaoMarcarFalhou(ctx, n.ID, ultimoErro)
	}
}

// montarPayloadOuCancelar decide, para n já reivindicada como "enviada", se
// o push deve mesmo ser mandado: monta o payload (item ou lista, conforme
// n.ItemID/n.ListaID) ou cancela a notificação (sobrescrevendo o status já
// reivindicado para "cancelada") quando o motivo do lembrete não existe
// mais. payload==nil,err==nil significa "já cancelada, não envie nada"; um
// err não-nil é repassado para o chamador marcar "falhou".
func (s *Scheduler) montarPayloadOuCancelar(ctx context.Context, n domain.Notificacao) ([]byte, error) {
	switch {
	case n.ItemID != nil:
		return s.montarPayloadItemOuCancelar(ctx, n)
	case n.ListaID != nil:
		return s.montarPayloadListaOuCancelar(ctx, n)
	default:
		// nunca deveria acontecer (CHECK do schema garante um dos dois) —
		// defensivo, mesmo tratamento de "sem motivo para existir".
		s.cancelar(ctx, n.ID, "notificação sem item nem lista")
		return nil, nil
	}
}

func (s *Scheduler) montarPayloadItemOuCancelar(ctx context.Context, n domain.Notificacao) ([]byte, error) {
	item, err := s.st.ItemBuscar(ctx, *n.ItemID)
	if err != nil {
		return nil, fmt.Errorf("item não encontrado: %w", err)
	}

	// DECISÃO (F3-1, bug de produção corrigido): segunda camada de defesa —
	// mesma checagem de app.notificacaoAindaAcionavel, agora no momento do
	// ENVIO (não só quando o usuário toca "Feito"/"Adiar"). Cobre a corrida
	// em que o item foi concluído/pausado/arquivado (ou a ocorrência ligada
	// deixou de estar aberta) depois que a notificação foi agendada mas
	// antes do tick que ia mandar o push — cancela em vez de mandar um
	// lembrete que não faz mais sentido.
	if !s.itemAindaAtivo(ctx, item, n) {
		s.cancelar(ctx, n.ID, "item não mais ativo no envio")
		return nil, nil
	}

	b, err := json.Marshal(payload{
		NotificacaoID: n.ID, ItemID: item.ID, Tipo: domain.TipoNotifLembrete,
		Title: "Pocket — " + item.Titulo,
		Body:  "toque em Feito, ou adie 30 min",
	})
	return b, err
}

// montarPayloadListaOuCancelar é o envio do lembrete de lista (CONTRATO-
// FASE3 §2.6/§5, F3-2): conta os pendentes NO MOMENTO DO ENVIO (não na
// geração) — lista arquivada ou zero pendentes cancela (K7: "não incomoda
// com lista vazia"); senão monta o payload com o resumo por seção
// (domain.MontarResumoCompras).
func (s *Scheduler) montarPayloadListaOuCancelar(ctx context.Context, n domain.Notificacao) ([]byte, error) {
	lista, err := s.st.ListaBuscar(ctx, *n.ListaID)
	if err != nil {
		return nil, fmt.Errorf("lista não encontrada: %w", err)
	}
	if lista.Arquivada {
		s.cancelar(ctx, n.ID, "lista arquivada no envio")
		return nil, nil
	}

	itens, err := s.st.ItensPorLista(ctx, lista.ID)
	if err != nil {
		return nil, err
	}
	secoes, err := s.st.SecoesPorLista(ctx, lista.ID)
	if err != nil {
		return nil, err
	}
	nomePorSecao := make(map[int64]string, len(secoes))
	for _, sc := range secoes {
		nomePorSecao[sc.ID] = sc.Nome
	}
	contagem := make(map[int64]int, len(secoes))
	total := 0
	for _, it := range itens {
		if it.Comprado {
			continue
		}
		contagem[it.SecaoID]++
		total++
	}
	if total == 0 {
		s.cancelar(ctx, n.ID, "lista vazia no envio")
		return nil, nil
	}

	secPend := make([]domain.SecaoPendencia, 0, len(contagem))
	for secaoID, n := range contagem {
		secPend = append(secPend, domain.SecaoPendencia{Nome: nomePorSecao[secaoID], Pendentes: n})
	}

	b, err := json.Marshal(listaPayload{
		NotificacaoID: n.ID, ListaID: lista.ID, Tipo: domain.TipoNotifLista,
		Title: "Compras — " + lista.Nome,
		Body:  domain.MontarResumoCompras(secPend),
	})
	return b, err
}

// cancelar sobrescreve n (já reivindicada como "enviada") para "cancelada"
// — usado quando o motivo do lembrete (item ativo, ocorrência aberta,
// lista com pendentes) deixou de existir entre o agendamento e o envio.
func (s *Scheduler) cancelar(ctx context.Context, notificacaoID int64, motivo string) {
	if err := s.st.NotificacaoMarcarCanceladaClaimed(ctx, notificacaoID); err != nil {
		slog.Error("cancelar notificação no envio", "notificacao_id", notificacaoID, "motivo", motivo, "erro", err)
	}
}

// itemAindaAtivo checa se item continua ativo e, se n está ligada a uma
// ocorrência, se ela continua aberta — mesma regra de
// app.notificacaoAindaAcionavel (ver a DECISÃO em montarPayloadItemOuCancelar).
func (s *Scheduler) itemAindaAtivo(ctx context.Context, item domain.Item, n domain.Notificacao) bool {
	if item.Estado != domain.EstadoAtivo {
		return false
	}
	if n.OcorrenciaID != nil {
		oc, err := s.st.OcorrenciaBuscar(ctx, *n.OcorrenciaID)
		if err != nil {
			return false // ocorrência sumiu — trata como não mais válida.
		}
		if oc.Status != domain.StatusAberta {
			return false
		}
	}
	return true
}

// payload é o corpo do push de item (CONTRATO §5/etapa 4: "Payload JSON:
// {notificacao_id, item_id, title, body}").
//
// DECISÃO (F3-2): Tipo é um campo novo ("lembrete") — aditivo e compatível
// com o sw.js atual (etapa 6), que já ignora campos desconhecidos; o
// contrato só passa a exigi-lo para o sw.js decidir não mostrar ações
// quando tipo="lista" (§5).
type payload struct {
	NotificacaoID int64  `json:"notificacao_id"`
	ItemID        int64  `json:"item_id"`
	Tipo          string `json:"tipo"`
	Title         string `json:"title"`
	Body          string `json:"body"`
}

// listaPayload é o corpo do push de lembrete de lista (F3-2, CONTRATO-
// FASE3 §5): "tipo" (lista) e "lista_id" no lugar de "item_id" — o sw.js
// não mostra ações quando tipo="lista" e abre "/?lista=<id>" no toque
// (fica para a F3-3, front).
type listaPayload struct {
	NotificacaoID int64  `json:"notificacao_id"`
	ListaID       int64  `json:"lista_id"`
	Tipo          string `json:"tipo"`
	Title         string `json:"title"`
	Body          string `json:"body"`
}
