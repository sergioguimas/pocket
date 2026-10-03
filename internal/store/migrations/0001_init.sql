-- Schema inicial do Pocket (CONTRATO.md §4), copiado literalmente.

CREATE TABLE item (
  id            INTEGER PRIMARY KEY,
  tipo          TEXT NOT NULL CHECK (tipo IN ('habito','compromisso')),
  titulo        TEXT NOT NULL CHECK (length(trim(titulo)) BETWEEN 1 AND 120),
  notas         TEXT,
  regra         TEXT,            -- JSON (§3.2); NOT NULL p/ habito, NULL p/ compromisso
  data          TEXT,            -- compromisso: 'YYYY-MM-DD' ou NULL
  hora          TEXT,            -- compromisso: 'HH:MM' ou NULL
  horarios      TEXT NOT NULL DEFAULT '[]',  -- JSON ["07:30","21:00"] ou antecedências ["-15m","-1d"]
  estado        TEXT NOT NULL DEFAULT 'ativo' CHECK (estado IN ('ativo','pausado','arquivado','concluido')),
  cor           TEXT,            -- token do front, opcional
  ordem         INTEGER NOT NULL DEFAULT 0,
  criado_em     TEXT NOT NULL,   -- RFC3339 local
  atualizado_em TEXT NOT NULL,
  CHECK ((tipo = 'habito') = (regra IS NOT NULL))
);

CREATE TABLE ocorrencia (
  id      INTEGER PRIMARY KEY,
  item_id INTEGER NOT NULL REFERENCES item(id),
  inicio  TEXT,                  -- 'YYYY-MM-DD'; NULL só p/ compromisso sem data
  fim     TEXT,                  -- NULL: intervalo aberto / compromisso sem data
  meta    INTEGER NOT NULL DEFAULT 1 CHECK (meta >= 1),
  status  TEXT NOT NULL DEFAULT 'aberta' CHECK (status IN ('aberta','cumprida','perdida','pulada')),
  UNIQUE (item_id, inicio)
);
CREATE INDEX ocorrencia_aberta ON ocorrencia(status, fim);

CREATE TABLE registro (
  id        INTEGER PRIMARY KEY,
  item_id   INTEGER NOT NULL REFERENCES item(id),
  em        TEXT NOT NULL,       -- RFC3339 local; o dia local decide a ocorrência
  valor     REAL,                -- fase 2
  nota      TEXT,
  origem    TEXT NOT NULL CHECK (origem IN ('app','notificacao'))
);
CREATE INDEX registro_item_em ON registro(item_id, em);

CREATE TABLE notificacao (
  id             INTEGER PRIMARY KEY,
  item_id        INTEGER NOT NULL REFERENCES item(id),
  ocorrencia_id  INTEGER REFERENCES ocorrencia(id),
  tipo           TEXT NOT NULL DEFAULT 'lembrete' CHECK (tipo IN ('lembrete','cobranca')),
  agendada_para  TEXT NOT NULL,
  status         TEXT NOT NULL DEFAULT 'pendente'
                 CHECK (status IN ('pendente','enviada','falhou','expirada','cancelada')),
  enviada_em     TEXT,
  recebida_em    TEXT,
  acao           TEXT CHECK (acao IN ('feito','adiar','abrir')),
  acao_em        TEXT,
  detalhe        TEXT,           -- status HTTP do push service / erro
  UNIQUE (item_id, agendada_para, tipo)
);
CREATE INDEX notificacao_fila ON notificacao(status, agendada_para);

CREATE TABLE push_sub (
  endpoint    TEXT PRIMARY KEY,
  p256dh      TEXT NOT NULL,
  auth        TEXT NOT NULL,
  user_agent  TEXT,
  criado_em   TEXT NOT NULL,
  ultimo_ok   TEXT
);

CREATE TABLE job (                -- mesmo padrão do Smaug: execução idempotente por dia
  nome     TEXT NOT NULL,
  data_ref TEXT NOT NULL,
  em       TEXT NOT NULL,
  PRIMARY KEY (nome, data_ref)
);
