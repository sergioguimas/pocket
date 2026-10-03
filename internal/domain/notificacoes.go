package domain

// DECISÃO (Opus, etapa 4/contrato §11/decisão 1): os tipos abaixo espelham
// literalmente o schema de store/migrations/0001_init.sql (tabelas
// notificacao e push_sub) — mesmo padrão de tipos.go.

// Tipos de notificação (schema, coluna notificacao.tipo): "lembrete" é o
// único usado na fase 1; "cobranca" fica reservado para a fase 2 (§3.5);
// "lista" é o lembrete de lista de compras (fase 3, CONTRATO-FASE3 §2.6 —
// geração/envio ficam para F3-2, mas o tipo já existe no schema desde 0002).
const (
	TipoNotifLembrete = "lembrete"
	TipoNotifCobranca = "cobranca"
	TipoNotifLista    = "lista"
)

// Status de notificação (schema, coluna notificacao.status).
const (
	StatusNotifPendente  = "pendente"
	StatusNotifEnviada   = "enviada"
	StatusNotifFalhou    = "falhou"
	StatusNotifExpirada  = "expirada"
	StatusNotifCancelada = "cancelada"
)

// Ações que o service worker reporta numa notificação (schema, coluna
// notificacao.acao; §3.5).
const (
	AcaoFeito = "feito"
	AcaoAdiar = "adiar"
	AcaoAbrir = "abrir"
)

// Notificacao é um disparo de lembrete agendado (§3.5) — criado antes pelo
// gerador (app), enviado pelo scheduler. OcorrenciaID é nil só depois que a
// ocorrência associada é apagada (Observador desvincula em vez de deixar
// referência solta — notificacao.ocorrencia_id não tem ON DELETE).
//
// DECISÃO (F3-1): desde a migração 0002, exatamente um de ItemID/ListaID é
// não-nil (CHECK do schema) — notificação de item (fase 1) ou de lista
// (fase 3, CONTRATO-FASE3 §2.6). ItemID deixou de ser int64 puro para
// caber os dois casos; todo código da fase 1 que lia n.ItemID direto passa
// a checar o ponteiro.
type Notificacao struct {
	ID           int64
	ItemID       *int64
	ListaID      *int64
	OcorrenciaID *int64
	Tipo         string
	AgendadaPara string // RFC3339 local (FormatInstante/ParseInstante)
	Status       string
	EnviadaEm    *string
	RecebidaEm   *string
	Acao         *string
	AcaoEm       *string
	Detalhe      *string // status HTTP do push service / erro
}

// PushSub é uma inscrição Web Push de um dispositivo (schema push_sub).
type PushSub struct {
	Endpoint  string
	P256dh    string
	Auth      string
	UserAgent string
	CriadoEm  string
	UltimoOk  *string
}
