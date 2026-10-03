// Package domain é o núcleo puro do Pocket (CONTRATO.md §3): regras de
// frequência, ocorrências, métricas e horários de lembrete. Nenhuma função
// aqui faz I/O — só cálculo a partir dos dados que o store passa. "Agora" só
// entra via clock.Clock (usado só em datas.go, para Hoje); todo o resto
// recebe a data/hora já pronta de quem chama.
package domain

import "time"

// DECISÃO (Opus, contrato §11/decisão 1): os tipos abaixo espelham
// literalmente o schema de store/migrations/0001_init.sql — nenhum campo
// aqui que não exista lá.

// Tipos de item (schema §4, coluna item.tipo).
const (
	TipoHabito      = "habito"
	TipoCompromisso = "compromisso"
)

// Estados de item (schema §4, coluna item.estado).
const (
	EstadoAtivo     = "ativo"
	EstadoPausado   = "pausado"
	EstadoArquivado = "arquivado"
	EstadoConcluido = "concluido"
)

// Status de ocorrência (schema §4, coluna ocorrencia.status).
const (
	StatusAberta   = "aberta"
	StatusCumprida = "cumprida"
	StatusPerdida  = "perdida"
	StatusPulada   = "pulada"
)

// Origens de registro (schema §4, coluna registro.origem).
const (
	OrigemApp         = "app"
	OrigemNotificacao = "notificacao"
)

// Item é um hábito ou compromisso (§3.1). Regra é não-nil só para hábito;
// Data/Hora só fazem sentido para compromisso (CHECK do schema).
type Item struct {
	ID           int64
	Tipo         string
	Titulo       string
	Notas        string
	Regra        *Regra // NOT NULL só para hábito
	Data         *Dia   // compromisso: data opcional
	Hora         string // compromisso: "HH:MM" ou "" (sem hora)
	Horarios     []string
	Estado       string
	Cor          string
	Ordem        int
	CriadoEm     time.Time
	AtualizadoEm time.Time
}

// EstaAtivo diz se o item está no estado que gera ocorrências/lembretes.
func (i Item) EstaAtivo() bool { return i.Estado == EstadoAtivo }

// Ocorrencia é a unidade "devia ter feito isto neste intervalo" (§3.3).
// Inicio/Fim são nil só para compromisso sem data; Fim também é nil para a
// ocorrência aberta de um hábito intervalo (vence quando for feito).
type Ocorrencia struct {
	ID     int64
	ItemID int64
	Inicio *Dia
	Fim    *Dia
	Meta   int
	Status string
}

// Registro é um toque em "feito" (§3.4).
type Registro struct {
	ID     int64
	ItemID int64
	Em     time.Time
	Valor  *float64 // fase 2; sempre nil na fase 1
	Nota   string
	Origem string
}
