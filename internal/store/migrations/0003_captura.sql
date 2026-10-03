-- Fase 1.5: captura rápida por voz e texto (CONTRATO-FASE15.md §5), schema copiado literalmente.

CREATE TABLE captura (
  id          INTEGER PRIMARY KEY,
  em          TEXT NOT NULL,                 -- RFC3339
  origem      TEXT NOT NULL,                 -- 'voz' | 'texto'
  texto       TEXT NOT NULL,                 -- transcrição ou texto digitado
  resposta    TEXT,                          -- JSON devolvido ao front (NULL se erro)
  erro        TEXT,
  modelo      TEXT NOT NULL,
  latencia_ms INTEGER NOT NULL,
  desfecho    TEXT                           -- NULL | confirmada | ajustada | descartada
);
CREATE INDEX captura_em ON captura(em);
