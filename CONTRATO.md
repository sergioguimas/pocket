# Pocket — contrato da fase 1 (núcleo + hábitos, compromissos e lembretes)

**Status:** TO-BE, **fechado em 26/09/2026** (decisões de §11 confirmadas por Sergio). O código em
`servicos/pocket/` hoje é o **spike da fase 0** (descartável) — a fase 1 o substitui por inteiro.
Build conforme [PLANO-DE-BUILD.md](PLANO-DE-BUILD.md).
**Base:** decisões D1–D9 em [README.md](OPERACAO.md); infra validada em [FASE-0-SPIKE.md](FASE-0-SPIKE.md).
**Papel:** app instalado no Poco (PWA) que mostra o que fazer hoje, registra hábitos e compromissos
com um toque — na tela ou direto na notificação — e acompanha a evolução (sequência, taxa, calendário).

Fases seguintes, **fora deste contrato** (ver §10): 1.5 captura rápida por texto/voz (Groq) ·
2 evolução com valor + cobrança · 3 compras · 4 receitas.

---

## 1. Fronteiras

```
 Poco (PWA instalado, Chrome)
   │ HTTPS https://pocket.example.com:9104   (tailnet; cert LE)
   ▼
 svc-pocket (Go, um binário, CGO_ENABLED=0)
   ├─ SPA React (dist/ embutido no binário) + sw.js
   ├─ /api/*  JSON
   ├─ SQLite data/pocket.db
   ├─ scheduler (tick 30 s) ──▶ Web Push (FCM etc.) ──▶ Poco
   └─ 1×/dia: acme.sh --cron (renova cert) · backup do SQLite
```

- **Entrada:** só HTTP do PWA. Sem WhatsApp, sem Telegram, sem LLM nesta fase.
- **Saída:** respostas JSON e notificações Web Push.
- **Não faz:** não escuta em `0.0.0.0` (só no IP da tailnet); não tem login (usuário único, acesso = estar na tailnet — mesmo risco aceito do smaug-dashboard); não chama API paga.
- **No aparelho:** HTTP, SQLite, scheduler. RAM alvo < 40 MB. Node nunca roda no aparelho (D6).

## 2. Stack

| Camada | Escolha | Nota |
|---|---|---|
| Backend | Go 1.27, `net/http` (mux padrão com padrões de método) | layout do Smaug: `internal/{config,domain,store,push,scheduler,httpapi}` |
| Banco | SQLite `modernc.org/sqlite` (puro Go), WAL, `busy_timeout=5000` | migrações embutidas (`store/migrations/*.sql`), como o Smaug |
| Push | `github.com/SherClockHolmes/webpush-go` | validado na fase 0 |
| Front | React + Vite + TypeScript, buildado no PC | `frontend/` → `dist/` **commitado** e embutido via `go:embed` |
| Service worker | `sw.js` escrito à mão (sem Workbox) | push, clique, cache do shell |
| Fuso | `America/Sao_Paulo`, `time/tzdata` embutido | Android não tem `/etc/localtime` (fase 0) |

## 3. Domínio

### 3.1 Item

Tudo o que o Pocket acompanha é um **item**, de um de dois tipos:

| Tipo | O que é | Exemplo |
|---|---|---|
| `habito` | recorrente, com uma **regra de frequência** (§3.2) | "ler", "academia 3×/semana", "regar plantas a cada 3 dias" |
| `compromisso` | acontece uma vez; data e hora **opcionais** | "dentista 14/10 15h", "renovar CNH até 30/10", "comprar presente" (sem data = tarefa solta) |

"Lembrete" **não é tipo**: é uma notificação presa a um item (§3.5). Todo item pode ter zero ou mais horários de lembrete.

Estados do item: `ativo`, `pausado` (hábito: não gera ocorrências, não quebra sequência), `arquivado` (some das listas, histórico preservado). Compromisso cumprido é **concluído** (fica no histórico, some de "Hoje").

### 3.2 Regra de frequência (hábitos)

Guardada como JSON validado em Go (`domain.Regra`), um de quatro formatos:

| `tipo` | Campos | Significado | Ocorrência gerada |
|---|---|---|---|
| `diario` | — | todo dia | 1 por dia, meta 1 |
| `dias_semana` | `dias: [1..7]` (1 = segunda … 7 = domingo) | só nesses dias | 1 por dia marcado, meta 1 |
| `vezes_periodo` | `vezes: 1..31`, `periodo: "semana" \| "mes"` | N vezes no período, em qualquer dia | 1 por período, meta N |
| `intervalo` | `dias: 2..90` | a cada N dias **contados da última vez feita** | 1 aberta por vez (§3.3) |

Validação: `dias_semana` sem dias → erro; `vezes` maior que os dias do período → erro; semana começa na **segunda** (§11).

### 3.3 Ocorrência (D7)

A unidade de "devia ter feito isto neste intervalo". Materializada no banco, com status:

| Campo | Significado |
|---|---|
| `inicio`, `fim` | datas locais (inclusivas). Diário/dias_semana: o próprio dia. Período: seg–dom ou 1º–último do mês. Intervalo: `fim` nulo (aberta até ser feita). Compromisso: `inicio = fim =` data do compromisso, ou ambos nulos se sem data |
| `meta` | quantas marcações cumprem (1, ou N em `vezes_periodo`) |
| `status` | `aberta` · `cumprida` · `perdida` · `pulada` |

Regras:
1. **Geração** (job `gerar_ocorrencias`, no boot e à 00:00): cria as ocorrências do dia e do período corrente para todo hábito ativo; idempotente por `UNIQUE(item_id, inicio)`. Não gera nada antes da data de criação do item nem durante pausa.
2. **Fechamento** (mesmo job): ocorrência com `fim < hoje` ainda `aberta` vira `perdida`.
3. **Cumprida** quando `registros no intervalo ≥ meta`; volta a `aberta`/`perdida` se um registro for desfeito.
4. **Pular** (ex.: doente, viagem): o usuário marca a ocorrência como `pulada` — não conta na taxa nem quebra a sequência. Desfazível.
5. **Intervalo:** ao registrar, a ocorrência aberta vira `cumprida` e nasce a próxima com `inicio = data do registro + N dias`. Antes do `inicio` o hábito aparece como "em dia"; depois, como "atrasado X dias". Nunca vira `perdida`.
6. **Marcação retroativa:** o usuário pode registrar em dia passado (calendário do hábito). A ocorrência daquele dia é recalculada (`perdida → cumprida`). Dia sem ocorrência (ex.: dia não marcado em `dias_semana`) aceita registro como extra, sem criar ocorrência.

### 3.4 Registro

Um toque em "feito" = uma linha em `registro` (`item_id`, `em` timestamp, `nota` opcional, `origem`: `app` \| `notificacao`). **Desfazer** apaga o registro (hard delete — é um toque errado, não histórico). A coluna `valor` existe já (NULL na fase 1) para a evolução da fase 2 (página 142, 3×12).

### 3.5 Lembretes e notificações

- Item tem `horarios` (lista de `HH:MM`). Para hábito, disparam **em cada dia com ocorrência aberta não cumprida** (em `vezes_periodo`, todo dia do período até bater a meta). Para compromisso com hora, o padrão é um lembrete na hora e o usuário pode adicionar antecedências (`-15min`, `-1d`...); sem hora, os horários valem para o dia do compromisso.
- **Precisão de minuto** (fase 0 mediu até 80 s de ponta a ponta com o Poco em Doze). Não prometer segundo.
- Cada disparo é uma linha em `notificacao` **criada antes** (job gera as do dia), com `agendada_para`. O scheduler a cada 30 s envia as vencidas não enviadas — **agenda persistida, nada só em memória** (lição do A5: o hook reinicia o serviço a cada deploy).
- Vencida há mais de **30 min** quando o serviço volta (aparelho desligado, deploy longo) → `expirada`, não envia. Lembrete velho é ruído.
- Se o item já foi cumprido antes do horário → `cancelada`, não envia.
- Ações na notificação (Android mostra até 2): **Feito** (cria registro com `origem = notificacao`) e **Adiar 30 min** (nova notificação +30 min). Tocar no corpo abre o app no item.
- O service worker reporta `recebido` (como no spike) — dá para ver no app se as notificações estão chegando.
- `tipo` da notificação: `lembrete` agora; `cobranca` fica reservado para a fase 2 (coluna já existe).

### 3.6 Métricas (hábitos)

- **Sequência atual:** ocorrências `cumprida` consecutivas até a mais recente fechada; a de hoje/do período corrente conta se já cumprida e **não quebra** se ainda aberta; `pulada` é transparente. Em `vezes_periodo` a unidade é o período ("3 semanas seguidas"). `intervalo` não tem sequência: mostra "em dia" / "atrasado X dias".
- **Melhor sequência** (mesma regra, histórico todo).
- **Taxa 30 dias:** `cumpridas / (cumpridas + perdidas)` entre as ocorrências fechadas nos últimos 30 dias, mais a corrente se já cumprida.
- Calculadas em Go (`domain`) a partir das ocorrências — sem tabela de cache.

## 4. Schema (SQLite, `store/migrations/0001_init.sql`)

```sql
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
```

Datas como TEXT local, sempre formatadas por uma função só em `domain` (nunca `time.Now().String()`).

## 5. API (`/api`, JSON; erro = `{"erro": "mensagem em português"}` + status HTTP)

| Método e rota | Faz |
|---|---|
| `GET /api/hoje` | o que mostrar na tela Hoje (§6): hábitos com ocorrência aberta/cumprida hoje, progresso de período, intervalos vencidos, compromissos de hoje e atrasados, contagem de tarefas sem data |
| `GET /api/itens?tipo=&estado=` | lista, com métricas (§3.6) nos hábitos |
| `POST /api/itens` | cria; valida regra/horários; gera ocorrências e notificações do dia na hora |
| `GET /api/itens/{id}` | detalhe + métricas + últimos 30 registros |
| `PATCH /api/itens/{id}` | edita campos; mudança de regra **não reescreve o passado** (ocorrências fechadas ficam; abertas de hoje em diante são refeitas) |
| `POST /api/itens/{id}/estado` | `{estado}`: pausar, retomar, arquivar, concluir |
| `GET /api/itens/{id}/calendario?de=&ate=` | por dia: status da ocorrência + nº de registros (para o mapa de calor) |
| `POST /api/itens/{id}/registros` | `{em?, nota?}`; `em` omitido = agora. Retorna item atualizado |
| `DELETE /api/registros/{id}` | desfaz |
| `POST /api/ocorrencias/{id}/pular` · `.../despular` | §3.3 regra 4 |
| `GET /api/push/vapid` | chave pública |
| `POST /api/push/subscribe` | upsert por endpoint (o app chama **em toda abertura**, se a permissão estiver concedida — cobre troca de endpoint e a migração do spike) |
| `POST /api/push/teste` | push imediato "teste" |
| `POST /api/notificacoes/{id}/{recebida\|feito\|adiar\|abrir}` | chamado pelo service worker |
| `GET /health` | `{ok, versao, tick_em, cert_expira_em, subs}` — sem `/api`, para a dashboard/monitoramento |

## 6. Telas (fase 1)

Mobile-first, uma coluna, tema escuro (paleta do spike como ponto de partida — `impeccable` na revisão de UI). Navegação inferior com 3 abas + botão de adicionar.

1. **Hoje** — o coração. Blocos: *Hábitos de hoje* (toque = feito, com desfazer por 5 s; chip "2/3 esta semana"; "atrasado 2 dias" em intervalo), *Compromissos* (hoje e atrasados, com hora), *Sem data* (contador que leva à aba Compromissos). Estado vazio útil ("nada para hoje — criar hábito").
2. **Hábitos** — lista com sequência atual e taxa 30 d. Detalhe: calendário de ~12 semanas (mapa de calor; tocar num dia passado marca/desmarca retroativo, §3.3 regra 6), sequência atual/melhor, taxa, botões pausar/arquivar, editar.
3. **Compromissos** — próximos por data, atrasados em destaque, "sem data" como lista de tarefas; concluir com um toque.
4. **Criar/editar** (folha modal): tipo → título → para hábito, seletor de frequência com os 4 formatos em linguagem natural ("Todo dia", "Em dias da semana", "X vezes por semana/mês", "A cada N dias"); para compromisso, data/hora opcionais e antecedências; horários de lembrete.
5. **Ajustes** (ícone no topo): estado das notificações (permissão, inscrito, último recebido), botão de teste, validade do certificado, versão.

Service worker: cacheia o shell (HTML/JS/CSS) para abrir instantâneo; dados sempre da rede. Sem rede: aviso "sem conexão com o redmilab" em vez de tela quebrada. Fila offline de registros é backlog.

## 7. Operação

- **Serviço:** `svc-pocket` já registrado em `scripts/services.sh`; comando `cd $HOMELAB/servicos/pocket && ./bin/pocket`. Porta **9104** no IP da tailnet (443 exige root — fase 0).
- **Renovação do certificado (obrigatório — janela da CA abre em 24/11/2026):** job diário `renovar_cert` roda `~/.acme.sh/acme.sh --cron --home ~/.acme.sh` (timeout 10 min), loga o resultado; o `--install-cert` já gravado copia para `secrets/tls/` e o `certLoader` (do spike) recarrega sem reiniciar. `GET /health` expõe a validade; **faltando < 20 dias, o próprio Pocket manda um push de alerta** (uma vez por dia).
- **Backup:** job diário `VACUUM INTO data/backup/pocket-YYYY-MM-DD.db`, mantém 7. Protege de bug/corrupção, não da perda do aparelho (buraco de backup do redmilab segue aberto).
- **Migração do spike:** reaproveita `secrets/vapid.json` (mesmo formato) para não invalidar a inscrição do Poco; `secrets/subs.json` e `eventos.json`/`shares.json` são descartados (o app reinscreve sozinho ao abrir). Remover o código do spike.
- **Restart a cada deploy é normal** (hook). Nada de estado só em memória: fila, jobs e ocorrências vêm do banco no boot.

## 8. Configuração (`.env` na pasta, fora do git; `.env.example` no repo)

| Variável | Default | |
|---|---|---|
| `POCKET_LISTEN` | `100.64.0.10:9104` | recusa `0.0.0.0` e vazio |
| `POCKET_DB` | `data/pocket.db` | |
| `POCKET_SECRETS` | `secrets` | `tls/`, `vapid.json` |
| `POCKET_TZ` | `America/Sao_Paulo` | |
| `POCKET_ACME` | `~/.acme.sh/acme.sh` | vazio desliga a renovação (dev no PC) |
| `POCKET_PUSH_SUBJECT` | `https://pocket.example.com` | exigido pelo VAPID |

No PC (dev): `POCKET_LISTEN=127.0.0.1:9104` com cert autoassinado; o front roda em `vite dev` com proxy para a API.

## 9. Critérios de aceite (marcar no Poco)

| # | Critério |
|---|---|
| C1 | Criar um hábito de cada formato (§3.2) e um compromisso com e sem data; todos aparecem certo em Hoje |
| C2 | Marcar e desfazer em Hoje atualiza sequência e taxa |
| C3 | `vezes_periodo` 3×/semana: chip mostra 1/3, 2/3; ao chegar em 3 some dos lembretes do resto da semana |
| C4 | `intervalo` 3 dias: após marcar, some de Hoje e volta no 3º dia; sem marcar, mostra "atrasado 1 dia" |
| C5 | Dia de `diario` não marcado vira `perdida` após 00:00 e quebra a sequência; `pulada` não quebra |
| C6 | Marcação retroativa no calendário recupera a sequência |
| C7 | Lembrete chega no horário (± 2 min) com o app fechado; **Feito** na notificação registra sem abrir o app |
| C8 | **Adiar 30 min** reenvia 30 min depois |
| C9 | Hábito já cumprido não recebe o lembrete do horário seguinte |
| C10 | Deploy (restart) com lembrete pendente para daqui a 5 min: o lembrete chega mesmo assim |
| C11 | Serviço parado por 1 h: ao voltar, lembretes vencidos há > 30 min ficam `expirada` e **não** chegam em rajada |
| C12 | `GET /health` mostra validade do cert; `acme.sh --cron` rodou (log) sem erro |
| C13 | App abre sem rede com o aviso de "sem conexão", não tela branca |
| C14 | Testes Go de `domain` cobrindo: geração de ocorrências dos 4 formatos (incl. virada de mês/semana e mês de 28–31 dias), fechamento, sequência, taxa, intervalo |

## 10. Fora do escopo — backlog

- **Fase 1.5 — captura rápida (decidida em 26/09, contrato próprio):** campo de texto + botão de microfone em Hoje.
  Áudio gravado no PWA (MediaRecorder) → Groq `whisper-large-v3-turbo`; texto → Groq `openai/gpt-oss-120b`
  (ou `-20b`) com saída estruturada. Entende *criar item* ("dentista dia 14 às 15h"), *registrar feito*
  ("fiz academia") e, na fase 2, *registrar valor* ("li até a página 142"). **O LLM só propõe:** abre o
  formulário de §6.4 preenchido (ou um cartão "registrar feito em X?"), nada é gravado sem o toque do
  usuário. Provedor atrás de interface (padrão `llm.Provider` do Smaug) — trocar Groq por Gemini é config.
  Tier gratuito do Groq em 26/09/2026: whisper 2.000 req/dia e 8 h de áudio/dia; gpt-oss 1.000 req/dia,
  200 mil tokens/dia — sobra para uso pessoal, mas pode mudar.
- **Fase 2:** `valor` nos registros (evolução: página, episódio, séries×reps) e gráficos; **cobrança** (notificação `tipo = cobranca` quando a ocorrência está prestes a virar perdida — em `vezes_periodo`, quando os dias restantes = marcações faltando).
- **Fase 3:** listas de compras. **Fase 4:** receitas (share target, yt-dlp, Gemini com vídeo, normalizar URL de rastreio).
- Horário silencioso, fila offline de registros, múltiplos usuários, exportar dados.

## 11. Decisões confirmadas (26/09/2026)

| # | Decisão | Por quê | Alternativa descartada |
|---|---|---|---|
| A1 | **Semana começa na segunda** (para `vezes_periodo` semanal) | rotina de treino/semana útil costuma pensar seg–dom | domingo (calendário brasileiro) |
| A2 | **`intervalo` conta da última vez feita**, não de uma data fixa | "regar a cada 3 dias": se atrasou, o próximo conta de quando regou | ancorado na data de criação (dias fixos: 1, 4, 7…) |
| A3 | **Lembrete vencido há > 30 min é descartado** | rajada de lembretes velhos após queda é pior que perder um | enviar tudo com atraso |
| A4 | **Sem LLM na fase 1** — criação por formulário; **LLM (Groq, tier gratuito) na fase 1.5**, só preenchendo o formulário | o formulário precisa existir antes (o LLM o preenche); fase 1 fica sem integração externa; custo não é mais argumento (Groq gratuito) | LLM já na fase 1; LLM gravando direto sem confirmação |
| A5 | **"Feito" na notificação registra sem abrir o app**; tocar no corpo abre | é o ganho da fase 0 (validado no A3) | sempre abrir o app para confirmar |
| A6 | **Desfazer é hard delete do registro** | toque errado não é histórico | soft delete com auditoria |
