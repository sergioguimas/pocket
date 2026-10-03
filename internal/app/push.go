package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"pocket/internal/domain"
	"pocket/internal/push"
)

// VapidPublica devolve a chave pública VAPID (GET /api/push/vapid). Vazia
// até SetPush ser chamado (main.go faz isso antes de aceitar requisições).
func (a *App) VapidPublica() string { return a.vapidPublic }

// PushSubEntrada é o corpo de POST /api/push/subscribe (§5: campos de
// PushSubscription.toJSON() do browser — endpoint + keys.p256dh/auth).
type PushSubEntrada struct {
	Endpoint  string
	P256dh    string
	Auth      string
	UserAgent string
}

// InscreverPush faz upsert por endpoint (§5: "o app chama em toda abertura,
// se a permissão estiver concedida — cobre troca de endpoint e a migração
// do spike").
func (a *App) InscreverPush(ctx context.Context, in PushSubEntrada) error {
	if in.Endpoint == "" || in.P256dh == "" || in.Auth == "" {
		return erroValidacao("inscrição push incompleta (endpoint/p256dh/auth)")
	}
	sub := domain.PushSub{
		Endpoint: in.Endpoint, P256dh: in.P256dh, Auth: in.Auth, UserAgent: in.UserAgent,
		CriadoEm: domain.FormatInstante(a.agora()),
	}
	return a.st.PushSubUpsert(ctx, sub)
}

// testePayload é o corpo de POST /api/push/teste ({notificacao_id, item_id,
// title, body} — CONTRATO §5/etapa 4; notificacao_id e item_id ficam 0: um
// teste não é uma notificação de verdade, não tem linha em `notificacao`).
type testePayload struct {
	NotificacaoID int64  `json:"notificacao_id"`
	ItemID        int64  `json:"item_id"`
	Title         string `json:"title"`
	Body          string `json:"body"`
}

// TestarPush manda um push "teste" imediato para todas as inscrições (§5) —
// usado pela tela de Ajustes para confirmar que o push está funcionando.
// Devolve quantas inscrições receberam com sucesso; remove as que
// devolverem 404/410 (mesma regra do scheduler).
func (a *App) TestarPush(ctx context.Context) (enviados int, err error) {
	if a.pusher == nil {
		return 0, fmt.Errorf("push não configurado")
	}
	subs, err := a.st.PushSubListar(ctx)
	if err != nil {
		return 0, err
	}

	payload, err := json.Marshal(testePayload{Title: "Pocket — teste", Body: "chegou às " + a.agora().Format("15:04:05")})
	if err != nil {
		return 0, err
	}

	agora := domain.FormatInstante(a.agora())
	for _, s := range subs {
		status, errEnv := a.pusher.Enviar(push.Subscription{Endpoint: s.Endpoint, P256dh: s.P256dh, Auth: s.Auth}, payload)
		if errEnv != nil {
			continue
		}
		if status == http.StatusNotFound || status == http.StatusGone {
			_ = a.st.PushSubRemover(ctx, s.Endpoint)
			continue
		}
		_ = a.st.PushSubAtualizarUltimoOk(ctx, s.Endpoint, agora)
		enviados++
	}
	return enviados, nil
}

// AcaoNotificacao aplica a ação do service worker numa notificação (§3.5,
// §5): "recebida" só marca que chegou; "abrir" só marca a ação (o clique já
// abriu o app no browser); "feito" registra (origem notificacao — mesmo
// caminho de POST /registros); "adiar" agenda uma nova notificação +30 min.
//
// DECISÃO (Opus, etapa 4): "feito"/"adiar" fazem duas escritas (Registrar
// ou a nova notificação, depois marcar a ação) em transações separadas —
// não uma só. Registrar já abre sua própria transação (é um caso de uso
// completo, com o Observador junto); aninhar transações não é suportado
// pelo database/sql. O risco (marcar a ação falhar depois de Registrar/
// inserir ter funcionado) é aceito: o efeito que importa (o registro, a
// notificação adiada) já aconteceu.
func (a *App) AcaoNotificacao(ctx context.Context, notificacaoID int64, acao string) error {
	switch acao {
	case "recebida":
		return a.marcarRecebida(ctx, notificacaoID)
	case domain.AcaoAbrir:
		return a.marcarAcaoNotificacao(ctx, notificacaoID, domain.AcaoAbrir)
	case domain.AcaoFeito:
		return a.acaoFeito(ctx, notificacaoID)
	case domain.AcaoAdiar:
		return a.acaoAdiar(ctx, notificacaoID)
	default:
		return erroValidacao("ação de notificação inválida (%q)", acao)
	}
}

func (a *App) buscarNotificacao(ctx context.Context, id int64) (domain.Notificacao, error) {
	n, err := a.st.NotificacaoBuscar(ctx, id)
	if err != nil {
		return domain.Notificacao{}, mapearErroStore(err, fmt.Sprintf("notificação %d não encontrada", id))
	}
	return n, nil
}

func (a *App) marcarRecebida(ctx context.Context, id int64) error {
	if _, err := a.buscarNotificacao(ctx, id); err != nil {
		return err
	}
	return a.st.NotificacaoMarcarRecebida(ctx, id, domain.FormatInstante(a.agora()))
}

func (a *App) marcarAcaoNotificacao(ctx context.Context, id int64, acao string) error {
	if _, err := a.buscarNotificacao(ctx, id); err != nil {
		return err
	}
	return a.st.NotificacaoMarcarAcao(ctx, id, acao, domain.FormatInstante(a.agora()))
}

// notificacaoAindaAcionavel checa se o item da notificação continua ativo
// e, se ela está ligada a uma ocorrência, essa ocorrência continua aberta.
//
// DECISÃO (F3-1, bug de produção corrigido): compromisso "C10 teste" foi
// concluído às 11:31:47 (item.estado=concluido, ocorrência cumprida) e às
// 11:31:49 o usuário tocou "Adiar 30 min" na notificação que já estava na
// tela do celular desde antes — AcaoNotificacao("adiar") não checava nada
// disso e criava uma notificação nova pendente para um item já concluído.
// acaoFeito/acaoAdiar chamam isto antes de agir; o scheduler (ver
// scheduler.itemAindaAtivo) faz a mesma checagem de novo no momento do
// ENVIO, como segunda camada de defesa contra qualquer corrida.
func (a *App) notificacaoAindaAcionavel(ctx context.Context, n domain.Notificacao) (bool, error) {
	if n.ItemID == nil {
		return true, nil // notificação de lista — sem "feito"/"adiar" (ver acaoFeito).
	}
	item, err := a.st.ItemBuscar(ctx, *n.ItemID)
	if err != nil {
		return false, mapearErroStore(err, fmt.Sprintf("item %d não encontrado", *n.ItemID))
	}
	if item.Estado != domain.EstadoAtivo {
		return false, nil
	}
	if n.OcorrenciaID != nil {
		oc, err := a.st.OcorrenciaBuscar(ctx, *n.OcorrenciaID)
		if err != nil {
			return false, nil // ocorrência sumiu — trata como não mais acionável, não como erro da request.
		}
		if oc.Status != domain.StatusAberta {
			return false, nil
		}
	}
	return true, nil
}

// acaoFeito é o "Feito" da notificação (decisão A5: registra sem abrir o
// app) — o mesmo Registrar de POST /registros, origem notificacao.
//
// DECISÃO (F3-1): lembrete de lista (CONTRATO-FASE3 §2.6: "sem ações de
// Feito/Adiar; tocar abre o app na lista") não tem ItemID — 409, mesmo
// vocabulário de erro do resto do app.
func (a *App) acaoFeito(ctx context.Context, id int64) error {
	n, err := a.buscarNotificacao(ctx, id)
	if err != nil {
		return err
	}
	if n.ItemID == nil {
		return erroConflito("lembrete de lista não tem essa ação")
	}

	acionavel, err := a.notificacaoAindaAcionavel(ctx, n)
	if err != nil {
		return err
	}
	if !acionavel {
		// grava a ação mesmo assim (o toque aconteceu, só não faz mais
		// sentido registrar de novo) e devolve 409 — ver a DECISÃO acima.
		if err := a.marcarAcaoNotificacao(ctx, id, domain.AcaoFeito); err != nil {
			return err
		}
		return erroConflito("item já concluído")
	}

	if _, _, err := a.Registrar(ctx, *n.ItemID, RegistroEntrada{Origem: domain.OrigemNotificacao}); err != nil {
		return err
	}
	return a.marcarAcaoNotificacao(ctx, id, domain.AcaoFeito)
}

// acaoAdiar cria uma nova notificação +30 min (§3.5) — idempotente por
// UNIQUE(item_id, agendada_para, tipo), como o gerador. Lembrete de lista
// (F3-1, ver acaoFeito) também não tem "adiar".
func (a *App) acaoAdiar(ctx context.Context, id int64) error {
	n, err := a.buscarNotificacao(ctx, id)
	if err != nil {
		return err
	}
	if n.ItemID == nil {
		return erroConflito("lembrete de lista não tem essa ação")
	}

	acionavel, err := a.notificacaoAindaAcionavel(ctx, n)
	if err != nil {
		return err
	}
	if !acionavel {
		// grava a ação mesmo assim, sem criar a notificação nova (ver a
		// DECISÃO em notificacaoAindaAcionavel).
		if err := a.marcarAcaoNotificacao(ctx, id, domain.AcaoAdiar); err != nil {
			return err
		}
		return erroConflito("item já concluído")
	}

	nova := domain.Notificacao{
		ItemID: n.ItemID, OcorrenciaID: n.OcorrenciaID, Tipo: domain.TipoNotifLembrete,
		AgendadaPara: domain.FormatInstante(a.agora().Add(30 * time.Minute)),
		Status:       domain.StatusNotifPendente,
	}
	if _, _, err := a.st.NotificacaoInserir(ctx, nova); err != nil {
		return err
	}
	return a.marcarAcaoNotificacao(ctx, id, domain.AcaoAdiar)
}
