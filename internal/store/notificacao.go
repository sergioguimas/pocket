package store

import (
	"context"
	"database/sql"
	"fmt"

	"pocket/internal/domain"
)

const notificacaoColunas = `id, item_id, lista_id, ocorrencia_id, tipo, agendada_para, status, enviada_em, recebida_em, acao, acao_em, detalhe`

func scanNotificacao(row interface{ Scan(...any) error }) (domain.Notificacao, error) {
	var (
		n            domain.Notificacao
		itemID       sql.NullInt64
		listaID      sql.NullInt64
		ocorrenciaID sql.NullInt64
		enviadaEm    sql.NullString
		recebidaEm   sql.NullString
		acao         sql.NullString
		acaoEm       sql.NullString
		detalhe      sql.NullString
	)
	if err := row.Scan(&n.ID, &itemID, &listaID, &ocorrenciaID, &n.Tipo, &n.AgendadaPara, &n.Status,
		&enviadaEm, &recebidaEm, &acao, &acaoEm, &detalhe); err != nil {
		return domain.Notificacao{}, err
	}
	if itemID.Valid {
		v := itemID.Int64
		n.ItemID = &v
	}
	if listaID.Valid {
		v := listaID.Int64
		n.ListaID = &v
	}
	if ocorrenciaID.Valid {
		v := ocorrenciaID.Int64
		n.OcorrenciaID = &v
	}
	if enviadaEm.Valid {
		v := enviadaEm.String
		n.EnviadaEm = &v
	}
	if recebidaEm.Valid {
		v := recebidaEm.String
		n.RecebidaEm = &v
	}
	if acao.Valid {
		v := acao.String
		n.Acao = &v
	}
	if acaoEm.Valid {
		v := acaoEm.String
		n.AcaoEm = &v
	}
	if detalhe.Valid {
		v := detalhe.String
		n.Detalhe = &v
	}
	return n, nil
}

func notificacaoBuscar(ctx context.Context, q querier, id int64) (domain.Notificacao, error) {
	row := q.QueryRowContext(ctx, `SELECT `+notificacaoColunas+` FROM notificacao WHERE id = ?`, id)
	n, err := scanNotificacao(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Notificacao{}, ErrNaoEncontrado{Entidade: "notificacao", ID: id}
		}
		return domain.Notificacao{}, fmt.Errorf("buscar notificacao %d: %w", id, err)
	}
	return n, nil
}

func notificacaoListar(ctx context.Context, q querier, query string, args ...any) ([]domain.Notificacao, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+notificacaoColunas+` FROM notificacao WHERE `+query, args...)
	if err != nil {
		return nil, fmt.Errorf("listar notificacoes: %w", err)
	}
	defer rows.Close()

	var out []domain.Notificacao
	for rows.Next() {
		n, err := scanNotificacao(rows)
		if err != nil {
			return nil, fmt.Errorf("ler notificacao listada: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// notificacoesPendentesDoDia lê as "pendente" de item cuja agendada_para cai
// no dia local dado (comparação por prefixo — agendada_para é RFC3339 e o
// dia é sempre os 10 primeiros caracteres, 'YYYY-MM-DD'). Usada pelo
// gerador (app) para saber o que já existe hoje antes de cancelar/inserir.
func notificacoesPendentesDoDia(ctx context.Context, q querier, itemID int64, dia string) ([]domain.Notificacao, error) {
	return notificacaoListar(ctx, q,
		`item_id = ? AND status = 'pendente' AND substr(agendada_para, 1, 10) = ? ORDER BY agendada_para`,
		itemID, dia)
}

// notificacoesListaPendentesDoDia é notificacoesPendentesDoDia (acima),
// mas para lista (F3-2, §2.6) — usada pelo gerador do lembrete de lista.
func notificacoesListaPendentesDoDia(ctx context.Context, q querier, listaID int64, dia string) ([]domain.Notificacao, error) {
	return notificacaoListar(ctx, q,
		`lista_id = ? AND status = 'pendente' AND substr(agendada_para, 1, 10) = ? ORDER BY agendada_para`,
		listaID, dia)
}

// notificacoesPendentesAte lê todas as "pendente" com agendada_para <= ate
// (RFC3339), em ordem — o que o scheduler processa a cada tick.
func notificacoesPendentesAte(ctx context.Context, q querier, ate string) ([]domain.Notificacao, error) {
	return notificacaoListar(ctx, q,
		`status = 'pendente' AND agendada_para <= ? ORDER BY agendada_para`, ate)
}

// notificacaoInserir grava n. inserted=false quando o índice único do dono
// (item ou lista — dois índices parciais desde a migração 0002, ver
// notificacao_item_unica/notificacao_lista_unica) já tinha uma linha
// (idempotência do gerador, mesmo padrão de ocorrenciaInserir). O upsert
// encadeado (dois ON CONFLICT — SQLite 3.35+) cobre os dois índices num
// INSERT só; só um deles pode disparar por vez, já que item_id e lista_id
// nunca são ambos não-nulos na mesma linha (CHECK do schema).
func notificacaoInserir(ctx context.Context, q querier, n domain.Notificacao) (id int64, inserted bool, err error) {
	row := q.QueryRowContext(ctx, `
		INSERT INTO notificacao (item_id, lista_id, ocorrencia_id, tipo, agendada_para, status)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(item_id, agendada_para, tipo) WHERE item_id IS NOT NULL DO NOTHING
		ON CONFLICT(lista_id, agendada_para, tipo) WHERE lista_id IS NOT NULL DO NOTHING
		RETURNING id`,
		ptrParaColuna(n.ItemID), ptrParaColuna(n.ListaID), ocorrenciaIDParaColuna(n.OcorrenciaID),
		n.Tipo, n.AgendadaPara, n.Status)
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("inserir notificacao (item %v, lista %v): %w", n.ItemID, n.ListaID, err)
	}
	return id, true, nil
}

// notificacaoCancelar marca id como "cancelada" — só se ainda "pendente"
// (uma já enviada/expirada não deve ser reescrita por engano).
func notificacaoCancelar(ctx context.Context, q querier, id int64) error {
	_, err := q.ExecContext(ctx, `UPDATE notificacao SET status = 'cancelada' WHERE id = ? AND status = 'pendente'`, id)
	if err != nil {
		return fmt.Errorf("cancelar notificacao %d: %w", id, err)
	}
	return nil
}

// notificacoesCancelarPorOcorrencia cancela todas as "pendente" ligadas a
// ocorrenciaID (§3.5: ocorrência cumprida cancela os lembretes que ainda
// não dispararam — C9; também usada por OcorrenciaApagando).
func notificacoesCancelarPorOcorrencia(ctx context.Context, q querier, ocorrenciaID int64) error {
	_, err := q.ExecContext(ctx, `UPDATE notificacao SET status = 'cancelada' WHERE ocorrencia_id = ? AND status = 'pendente'`, ocorrenciaID)
	if err != nil {
		return fmt.Errorf("cancelar notificacoes da ocorrencia %d: %w", ocorrenciaID, err)
	}
	return nil
}

// notificacoesDesvincularOcorrencia zera ocorrencia_id nas notificações
// ligadas a ocorrenciaID — chamado ANTES do DELETE da ocorrência (ATENÇÃO em
// app.go: a coluna não tem ON DELETE), para o histórico (já enviadas etc.)
// não ficar com uma referência solta.
func notificacoesDesvincularOcorrencia(ctx context.Context, q querier, ocorrenciaID int64) error {
	_, err := q.ExecContext(ctx, `UPDATE notificacao SET ocorrencia_id = NULL WHERE ocorrencia_id = ?`, ocorrenciaID)
	if err != nil {
		return fmt.Errorf("desvincular notificacoes da ocorrencia %d: %w", ocorrenciaID, err)
	}
	return nil
}

// notificacaoMarcarEnviada reivindica id (só se ainda "pendente" — a
// checagem WHERE status='pendente' é o que impede um tick concorrente de
// enviar duas vezes, decisão etapa 4) e grava enviada_em. claimed=false
// quando outro tick já reivindicou (ou a notificação não existe/não está
// mais pendente).
func notificacaoMarcarEnviada(ctx context.Context, q querier, id int64, enviadaEm string) (claimed bool, err error) {
	res, err := q.ExecContext(ctx, `UPDATE notificacao SET status = 'enviada', enviada_em = ? WHERE id = ? AND status = 'pendente'`, enviadaEm, id)
	if err != nil {
		return false, fmt.Errorf("marcar notificacao %d enviada: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("confirmar notificacao %d enviada: %w", id, err)
	}
	return n > 0, nil
}

// notificacaoMarcarCanceladaClaimed marca id como "cancelada"
// incondicionalmente (sem WHERE status='pendente') — diferente de
// notificacaoCancelar, que só cancela uma ainda pendente. Usada pelo
// scheduler no momento do envio, DEPOIS de já ter reivindicado a linha
// como "enviada" (NotificacaoMarcarEnviada) mas ANTES de mandar o push de
// verdade: o item (ou a ocorrência ligada) deixou de justificar o
// lembrete entre o agendamento e o envio (bug de produção corrigido em
// F3-1 — ver a DECISÃO em app.notificacaoAindaAcionavel e em
// scheduler.itemAindaAtivo).
func notificacaoMarcarCanceladaClaimed(ctx context.Context, q querier, id int64) error {
	_, err := q.ExecContext(ctx, `UPDATE notificacao SET status = 'cancelada' WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("cancelar notificacao %d (já reivindicada): %w", id, err)
	}
	return nil
}

// notificacaoMarcarFalhou grava o motivo da falha de envio (detalhe = erro
// do Pusher ou status HTTP) — chamada depois de notificacaoMarcarEnviada já
// ter reivindicado a linha, então o WHERE aqui é só defensivo (sem guard de
// status: uma falha sempre sobrescreve).
func notificacaoMarcarFalhou(ctx context.Context, q querier, id int64, detalhe string) error {
	_, err := q.ExecContext(ctx, `UPDATE notificacao SET status = 'falhou', detalhe = ? WHERE id = ?`, detalhe, id)
	if err != nil {
		return fmt.Errorf("marcar notificacao %d falhou: %w", id, err)
	}
	return nil
}

// notificacaoMarcarExpirada reivindica id como "expirada" (só se ainda
// "pendente" — mesma proteção de concorrência de notificacaoMarcarEnviada;
// decisão A3: vencida há mais de 30 min não é enviada).
func notificacaoMarcarExpirada(ctx context.Context, q querier, id int64) (claimed bool, err error) {
	res, err := q.ExecContext(ctx, `UPDATE notificacao SET status = 'expirada' WHERE id = ? AND status = 'pendente'`, id)
	if err != nil {
		return false, fmt.Errorf("marcar notificacao %d expirada: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("confirmar notificacao %d expirada: %w", id, err)
	}
	return n > 0, nil
}

// notificacaoMarcarRecebida grava recebida_em (relatado pelo service worker
// — §3.5: "dá para ver no app se as notificações estão chegando").
func notificacaoMarcarRecebida(ctx context.Context, q querier, id int64, recebidaEm string) error {
	_, err := q.ExecContext(ctx, `UPDATE notificacao SET recebida_em = ? WHERE id = ?`, recebidaEm, id)
	if err != nil {
		return fmt.Errorf("marcar notificacao %d recebida: %w", id, err)
	}
	return nil
}

// notificacaoMarcarAcao grava a ação (feito/adiar/abrir) e quando ela
// aconteceu.
func notificacaoMarcarAcao(ctx context.Context, q querier, id int64, acao, acaoEm string) error {
	_, err := q.ExecContext(ctx, `UPDATE notificacao SET acao = ?, acao_em = ? WHERE id = ?`, acao, acaoEm, id)
	if err != nil {
		return fmt.Errorf("marcar acao da notificacao %d: %w", id, err)
	}
	return nil
}

func ocorrenciaIDParaColuna(id *int64) any {
	if id == nil {
		return nil
	}
	return *id
}

// ptrParaColuna converte um ponteiro (nil ou não) num argumento de driver
// SQL — mesma ideia de ocorrenciaIDParaColuna, mas genérico (F3-1:
// notificacao.item_id/lista_id passam a ser ambos opcionais).
func ptrParaColuna[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

// Store — leituras/escritas diretas (fora de transação): o scheduler e os
// casos de uso de push (app/push.go) não precisam da atomicidade de uma Tx
// multi-linha — cada operação aqui é uma linha só.

func (s *Store) NotificacaoBuscar(ctx context.Context, id int64) (domain.Notificacao, error) {
	return notificacaoBuscar(ctx, s.db, id)
}

func (s *Store) NotificacoesPendentesAte(ctx context.Context, ate string) ([]domain.Notificacao, error) {
	return notificacoesPendentesAte(ctx, s.db, ate)
}

func (s *Store) NotificacaoInserir(ctx context.Context, n domain.Notificacao) (int64, bool, error) {
	return notificacaoInserir(ctx, s.db, n)
}

func (s *Store) NotificacaoMarcarEnviada(ctx context.Context, id int64, enviadaEm string) (bool, error) {
	return notificacaoMarcarEnviada(ctx, s.db, id, enviadaEm)
}

func (s *Store) NotificacaoMarcarFalhou(ctx context.Context, id int64, detalhe string) error {
	return notificacaoMarcarFalhou(ctx, s.db, id, detalhe)
}

func (s *Store) NotificacaoMarcarCanceladaClaimed(ctx context.Context, id int64) error {
	return notificacaoMarcarCanceladaClaimed(ctx, s.db, id)
}

func (s *Store) NotificacaoMarcarExpirada(ctx context.Context, id int64) (bool, error) {
	return notificacaoMarcarExpirada(ctx, s.db, id)
}

func (s *Store) NotificacaoMarcarRecebida(ctx context.Context, id int64, recebidaEm string) error {
	return notificacaoMarcarRecebida(ctx, s.db, id, recebidaEm)
}

func (s *Store) NotificacaoMarcarAcao(ctx context.Context, id int64, acao, acaoEm string) error {
	return notificacaoMarcarAcao(ctx, s.db, id, acao, acaoEm)
}

// Tx — usadas pelo Observador (internal/app/observador.go), dentro da
// mesma transação do caso de uso que mudou item/ocorrência.

func (t *Tx) NotificacoesPendentesDoDia(ctx context.Context, itemID int64, dia string) ([]domain.Notificacao, error) {
	return notificacoesPendentesDoDia(ctx, t.tx, itemID, dia)
}

func (t *Tx) NotificacoesListaPendentesDoDia(ctx context.Context, listaID int64, dia string) ([]domain.Notificacao, error) {
	return notificacoesListaPendentesDoDia(ctx, t.tx, listaID, dia)
}

func (t *Tx) NotificacaoInserir(ctx context.Context, n domain.Notificacao) (int64, bool, error) {
	return notificacaoInserir(ctx, t.tx, n)
}

func (t *Tx) NotificacaoCancelar(ctx context.Context, id int64) error {
	return notificacaoCancelar(ctx, t.tx, id)
}

func (t *Tx) NotificacoesCancelarPorOcorrencia(ctx context.Context, ocorrenciaID int64) error {
	return notificacoesCancelarPorOcorrencia(ctx, t.tx, ocorrenciaID)
}

func (t *Tx) NotificacoesDesvincularOcorrencia(ctx context.Context, ocorrenciaID int64) error {
	return notificacoesDesvincularOcorrencia(ctx, t.tx, ocorrenciaID)
}
