-- Fase 3: listas de compras (CONTRATO-FASE3.md §4), schema copiado literalmente.

CREATE TABLE lista (
  id        INTEGER PRIMARY KEY,
  nome      TEXT NOT NULL CHECK (length(trim(nome)) BETWEEN 1 AND 40),
  ordem     INTEGER NOT NULL DEFAULT 0,
  arquivada INTEGER NOT NULL DEFAULT 0 CHECK (arquivada IN (0,1)),
  lembrete  TEXT,                 -- JSON {"regra":{...},"horarios":["09:00"]} ou NULL
  criado_em TEXT NOT NULL
);

CREATE TABLE secao (
  id       INTEGER PRIMARY KEY,
  lista_id INTEGER NOT NULL REFERENCES lista(id),
  nome     TEXT NOT NULL CHECK (length(trim(nome)) BETWEEN 1 AND 40),
  ordem    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE item_compra (
  id          INTEGER PRIMARY KEY,
  lista_id    INTEGER NOT NULL REFERENCES lista(id),
  secao_id    INTEGER NOT NULL REFERENCES secao(id),
  nome        TEXT NOT NULL CHECK (length(trim(nome)) BETWEEN 1 AND 60),
  nome_chave  TEXT NOT NULL,
  quantidade  TEXT,
  nota        TEXT,
  comprado    INTEGER NOT NULL DEFAULT 0 CHECK (comprado IN (0,1)),
  comprado_em TEXT,
  origem      TEXT NOT NULL DEFAULT 'manual' CHECK (origem IN ('manual','frequente','receita')),
  receita_id  INTEGER,            -- sem FK até a fase 4
  criado_em   TEXT NOT NULL
);
-- regra 1: no máximo um pendente por nome_chave na lista
CREATE UNIQUE INDEX item_compra_pendente ON item_compra(lista_id, nome_chave) WHERE comprado = 0;
CREATE INDEX item_compra_lista ON item_compra(lista_id, comprado);

CREATE TABLE compra_historico (
  lista_id          INTEGER NOT NULL REFERENCES lista(id),
  nome_chave        TEXT NOT NULL,
  nome              TEXT NOT NULL,
  secao_id          INTEGER REFERENCES secao(id),
  ultima_quantidade TEXT,
  vezes             INTEGER NOT NULL DEFAULT 0,
  ultima_em         TEXT NOT NULL,
  PRIMARY KEY (lista_id, nome_chave)
);

-- notificacao passa a aceitar lista no lugar de item (reconstrução — SQLite não altera NOT NULL/CHECK)
--
-- DECISÃO: sem PRAGMA foreign_keys=OFF aqui. Nada no schema tem FK
-- apontando PARA notificacao (só ela aponta para fora, a item/ocorrencia),
-- então DROP TABLE notificacao não deixa referência solta; e a
-- reconstrução copia item_id/ocorrencia_id que já existiam (validados pelo
-- FK da tabela antiga) e deixa lista_id NULL nas linhas antigas — FK nunca
-- reprova valor NULL. RENAME também não mexe em FK de terceiros, porque
-- não existem. Deixar foreign_keys ligado o tempo todo é mais simples e
-- ainda protege a própria migração de um erro de digitação nos SELECT/INSERT.
CREATE TABLE notificacao_nova (
  id             INTEGER PRIMARY KEY,
  item_id        INTEGER REFERENCES item(id),
  lista_id       INTEGER REFERENCES lista(id),
  ocorrencia_id  INTEGER REFERENCES ocorrencia(id),
  tipo           TEXT NOT NULL DEFAULT 'lembrete' CHECK (tipo IN ('lembrete','cobranca','lista')),
  agendada_para  TEXT NOT NULL,
  status         TEXT NOT NULL DEFAULT 'pendente'
                 CHECK (status IN ('pendente','enviada','falhou','expirada','cancelada')),
  enviada_em     TEXT,
  recebida_em    TEXT,
  acao           TEXT CHECK (acao IN ('feito','adiar','abrir')),
  acao_em        TEXT,
  detalhe        TEXT,
  CHECK ((item_id IS NULL) <> (lista_id IS NULL))
);
INSERT INTO notificacao_nova (id, item_id, ocorrencia_id, tipo, agendada_para, status, enviada_em,
  recebida_em, acao, acao_em, detalhe)
  SELECT id, item_id, ocorrencia_id, tipo, agendada_para, status, enviada_em, recebida_em, acao, acao_em, detalhe
  FROM notificacao;
DROP TABLE notificacao;
ALTER TABLE notificacao_nova RENAME TO notificacao;
CREATE INDEX notificacao_fila ON notificacao(status, agendada_para);
-- UNIQUE com NULL não deduplica no SQLite: um índice parcial para cada dono
CREATE UNIQUE INDEX notificacao_item_unica  ON notificacao(item_id, agendada_para, tipo)  WHERE item_id IS NOT NULL;
CREATE UNIQUE INDEX notificacao_lista_unica ON notificacao(lista_id, agendada_para, tipo) WHERE lista_id IS NOT NULL;

-- Seed: lista "Mercado" com as 10 seções-padrão de §2.2, nesta ordem.
--
-- DECISÃO: criado_em fixo (data desta migração) em vez de vir do clock
-- injetado — a migração é texto SQL estático aplicado uma vez só
-- (schema_version evita repetição); não há como injetar clock.Clock aqui
-- sem reescrever o mecanismo de migração inteiro por causa de um único
-- timestamp de seed. A regra "nenhum time.Now() fora de clock/main.go" é
-- sobre código Go, não sobre uma migração SQL versionada.
INSERT INTO lista (nome, ordem, arquivada, criado_em) VALUES ('Mercado', 0, 0, '2026-09-26T00:00:00-03:00');
INSERT INTO secao (lista_id, nome, ordem)
  SELECT id, 'Hortifrúti', 0 FROM lista WHERE nome = 'Mercado';
INSERT INTO secao (lista_id, nome, ordem)
  SELECT id, 'Padaria', 1 FROM lista WHERE nome = 'Mercado';
INSERT INTO secao (lista_id, nome, ordem)
  SELECT id, 'Açougue e peixaria', 2 FROM lista WHERE nome = 'Mercado';
INSERT INTO secao (lista_id, nome, ordem)
  SELECT id, 'Frios e laticínios', 3 FROM lista WHERE nome = 'Mercado';
INSERT INTO secao (lista_id, nome, ordem)
  SELECT id, 'Mercearia', 4 FROM lista WHERE nome = 'Mercado';
INSERT INTO secao (lista_id, nome, ordem)
  SELECT id, 'Bebidas', 5 FROM lista WHERE nome = 'Mercado';
INSERT INTO secao (lista_id, nome, ordem)
  SELECT id, 'Congelados', 6 FROM lista WHERE nome = 'Mercado';
INSERT INTO secao (lista_id, nome, ordem)
  SELECT id, 'Limpeza', 7 FROM lista WHERE nome = 'Mercado';
INSERT INTO secao (lista_id, nome, ordem)
  SELECT id, 'Higiene', 8 FROM lista WHERE nome = 'Mercado';
INSERT INTO secao (lista_id, nome, ordem)
  SELECT id, 'Outros', 9 FROM lista WHERE nome = 'Mercado';
