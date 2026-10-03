package store

import (
	"context"
	"database/sql"
	"fmt"

	"pocket/internal/domain"
)

const pushSubColunas = `endpoint, p256dh, auth, user_agent, criado_em, ultimo_ok`

func scanPushSub(row interface{ Scan(...any) error }) (domain.PushSub, error) {
	var (
		sub       domain.PushSub
		userAgent sql.NullString
		ultimoOk  sql.NullString
	)
	if err := row.Scan(&sub.Endpoint, &sub.P256dh, &sub.Auth, &userAgent, &sub.CriadoEm, &ultimoOk); err != nil {
		return domain.PushSub{}, err
	}
	sub.UserAgent = userAgent.String
	if ultimoOk.Valid {
		v := ultimoOk.String
		sub.UltimoOk = &v
	}
	return sub, nil
}

// pushSubUpsert grava sub por endpoint (§5: "upsert por endpoint" — o app
// chama em toda abertura do PWA). Em conflito, mantém criado_em e ultimo_ok
// originais — só endpoint é a chave de verdade de um dispositivo; trocar de
// p256dh/auth (o browser renova a inscrição) não deve resetar o histórico.
func pushSubUpsert(ctx context.Context, q querier, sub domain.PushSub) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO push_sub (endpoint, p256dh, auth, user_agent, criado_em)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(endpoint) DO UPDATE SET p256dh = excluded.p256dh, auth = excluded.auth, user_agent = excluded.user_agent`,
		sub.Endpoint, sub.P256dh, sub.Auth, nullIfVazio(sub.UserAgent), sub.CriadoEm)
	if err != nil {
		return fmt.Errorf("gravar inscrição push (%s): %w", sub.Endpoint, err)
	}
	return nil
}

func pushSubListar(ctx context.Context, q querier) ([]domain.PushSub, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+pushSubColunas+` FROM push_sub ORDER BY criado_em`)
	if err != nil {
		return nil, fmt.Errorf("listar inscrições push: %w", err)
	}
	defer rows.Close()

	var out []domain.PushSub
	for rows.Next() {
		sub, err := scanPushSub(rows)
		if err != nil {
			return nil, fmt.Errorf("ler inscrição push listada: %w", err)
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

// pushSubRemover apaga a inscrição — chamado quando o push service devolve
// 404/410 (endpoint morto, CONTRATO §5/etapa 4).
func pushSubRemover(ctx context.Context, q querier, endpoint string) error {
	_, err := q.ExecContext(ctx, `DELETE FROM push_sub WHERE endpoint = ?`, endpoint)
	if err != nil {
		return fmt.Errorf("remover inscrição push (%s): %w", endpoint, err)
	}
	return nil
}

func pushSubAtualizarUltimoOk(ctx context.Context, q querier, endpoint, ultimoOk string) error {
	_, err := q.ExecContext(ctx, `UPDATE push_sub SET ultimo_ok = ? WHERE endpoint = ?`, ultimoOk, endpoint)
	if err != nil {
		return fmt.Errorf("atualizar ultimo_ok da inscrição push (%s): %w", endpoint, err)
	}
	return nil
}

func (s *Store) PushSubUpsert(ctx context.Context, sub domain.PushSub) error {
	return pushSubUpsert(ctx, s.db, sub)
}

func (s *Store) PushSubListar(ctx context.Context) ([]domain.PushSub, error) {
	return pushSubListar(ctx, s.db)
}

func (s *Store) PushSubRemover(ctx context.Context, endpoint string) error {
	return pushSubRemover(ctx, s.db, endpoint)
}

func (s *Store) PushSubAtualizarUltimoOk(ctx context.Context, endpoint, ultimoOk string) error {
	return pushSubAtualizarUltimoOk(ctx, s.db, endpoint, ultimoOk)
}
