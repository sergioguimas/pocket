# Pocket — plano de build da fase 1

Como transformar [CONTRATO.md](CONTRATO.md) em `bin/pocket` rodando no redmilab e instalado no Poco.
Padrão da casa: **Opus planeja, orquestra e revisa; Sonnet escreve o grosso do código**, uma etapa por
vez, via subagentes (mesmo modelo do plano do Smaug).

> "Fase" no contrato = fase do produto (1, 1.5, 2…). Aqui, as partes do build da fase 1 se chamam **etapas**.

---

## 1. Regras do build

1. **Branch `bot/pocket`** a partir de `master`. Um commit por etapa: `pocket: etapa N — <o quê>`.
   O spike continua rodando em `master` até o aceite — **nunca** dar `git push celular master` no meio
   do build (o hook publica e reinicia o `svc-pocket`). Merge e deploy só no fim (§6).
2. **Onde roda:** etapas 1–5 (Go) podem rodar **no aparelho** (sessão `pocket` do Remote Control,
   `svc-claude-pocket`, que já nasce em `servicos/pocket/`) ou no PC. Etapas 6–7 (front) **só no PC** —
   Node nunca roda no aparelho (D6). Se rodar no aparelho: `git push celular bot/pocket` para o PC
   buscar; no PC, `git fetch celular && git checkout bot/pocket`.
3. **Sessão principal em Opus**; cada etapa vai para um subagente **Sonnet** (`Agent`, `model: "sonnet"`)
   com o prompt de §4. O Opus lê o diff, roda `go vet ./... && go test ./...`, aplica o checklist (§5),
   corrige o que for pequeno, devolve ao Sonnet o que for estrutural, e só então commita.
4. **Uma etapa por vez, na ordem.** Não avançar com teste vermelho ou `vet` sujo.
5. **O contrato manda.** Decisão que o contrato não cobre: o Opus decide, anota em `OPERACAO.md` →
   "Decisões durante o build" e segue. Decisão que **contraria** o contrato: para e pergunta ao Sergio.
6. **Tempo é injetado.** Todo "agora" e "hoje" vem de um `clock.Clock`; testes usam relógio falso.
   Nenhum `time.Now()` fora de `internal/clock` e `main.go`.
7. **Sem rede nos testes.** Web Push e `acme.sh` só por interface + fake.
8. **Ambiente no aparelho:** Termux, `CGO_ENABLED=0`, Go 1.27, sem `/tmp` (usar `t.TempDir()`),
   uma sessão de Claude por vez (RAM).

## 2. Prompt de abertura (colar na sessão Opus)

```
Você está em ~/projetos/redmilab (ou Documents/Projetos/redmilab no PC). Vamos construir a fase 1 do
servicos/pocket/ seguindo servicos/pocket/CONTRATO.md e servicos/pocket/PLANO-DE-BUILD.md. Leia os
dois inteiros antes de qualquer coisa, e também: servicos/pocket/main.go (spike: certLoader e formato
do vapid.json a reaproveitar), servicos/smaug/internal/config e servicos/smaug/internal/store (padrões
de config por lookup e de migrações embutidas), scripts/services.sh (entrada pocket já existe).

Regras: branch bot/pocket; uma etapa por vez conforme PLANO-DE-BUILD §3; cada etapa delegada a um
subagente Sonnet (Agent, model "sonnet") com o cabeçalho comum + o prompt da etapa em §4; você (Opus)
revisa com o checklist de §5, roda go vet e go test, corrige o pequeno, devolve o estrutural, commita
"pocket: etapa N — ...". Não dê push em master. Decisões fora do contrato vão em README.md →
"Decisões durante o build". Ao fim de cada etapa, me diga em 3 linhas o que entrou e o que ficou.

Comece pela etapa 1.
```

## 3. Etapas

| # | Entrega | Onde | Prova |
|---|---|---|---|
| 1 | Esqueleto que sobe: config, clock, TLS com recarga, SQLite + migração `0001`, `/health` mínimo, spike removido | aparelho ou PC | `go run .` sobe com cert; `curl /health`; testes de config e migração |
| 2 | Domínio puro: regras de frequência, datas, geração/fechamento de ocorrências, métricas, horários | aparelho ou PC | testes table-driven de C14 |
| 3 | Repositórios + serviço de aplicação + jobs de ocorrência + API de §5 (menos push) | aparelho ou PC | testes `httptest` ponta a ponta com banco em `t.TempDir()` |
| 4 | Web Push + agenda de notificações + scheduler + endpoints do service worker | aparelho ou PC | testes com relógio e pusher falsos: C7–C11 simulados |
| 5 | Operação: renovação do cert, alerta de validade, backup, `/health` completo | aparelho ou PC | testes com executor falso; `/health` com `cert_expira_em` |
| 6 | Front: React + Vite, telas de §6, `sw.js`, manifest, `dist/` embutido | **PC** | `npm run build`; `go build`; navegar no Chrome do PC com cert autoassinado |
| 7 | Fechamento: README, decisões, `CHECKLIST-ACEITE.md` | PC | revisão |

## 4. Prompts por etapa (para o subagente Sonnet)

Cabeçalho comum (o Opus prefixa):

```
Você está implementando a fase 1 do serviço Go em servicos/pocket/ do repo redmilab (branch bot/pocket).
Contrato: servicos/pocket/CONTRATO.md — leia as seções que a etapa cita antes de escrever. Padrões da
casa: config por lookup como servicos/smaug/internal/config; migrações embutidas como
servicos/smaug/internal/store; comentários curtos em português; sem frameworks web; puro Go
(CGO_ENABLED=0), Go 1.27; SQLite via modernc.org/sqlite. Layout: main.go + internal/{config,clock,
domain,store,app,push,scheduler,ops,httpapi}. Todo "agora" vem de clock.Clock (nunca time.Now() fora
de internal/clock e main.go). Datas locais como TEXT 'YYYY-MM-DD', formatadas só por funções de
internal/domain. Testes table-driven junto com o código, sem rede, banco em t.TempDir(). Não invente
campos que o contrato não tem; se faltar algo, escolha o mais simples, marque `// DECISÃO:` e liste no
fim. Termine com `go vet ./... && go test ./...` verdes e um resumo de 5 linhas: arquivos, decisões,
o que ficou de fora.
```

**Etapa 1**
```
Etapa 1 — esqueleto. Leia CONTRATO §1, §2, §4, §7 (migração do spike), §8.
Apague o código do spike (main.go, web/) — antes, leia-o: o certLoader (recarga do cert por mtime) e o
formato de secrets/vapid.json ({"Public","Private"}) devem ser preservados. Crie: go.mod (module pocket;
deps: modernc.org/sqlite, webpush-go já presente), main.go (wiring, slog em stdout, signal.NotifyContext,
encerramento limpo), internal/config (Config + LoadConfigFrom(lookup) com as variáveis de §8; recusa
POCKET_LISTEN vazio ou 0.0.0.0/::; POCKET_TZ carrega com time/tzdata embutido), internal/clock (Clock
interface, Real, Fake avançável), internal/ops/tls.go (certLoader do spike + função que devolve a
validade do cert), internal/store (Open com PRAGMAs WAL, foreign_keys, busy_timeout=5000; migrações
embutidas com schema_version; 0001_init.sql = schema de CONTRATO §4 literalmente), internal/httpapi
(Server com GET /health → {ok, versao}; resto 404 por ora). .env.example com §8. Testes: config
(defaults, listen proibido, TZ inválida), migração aplica 1× e é idempotente, CHECKs do schema
(habito sem regra falha; compromisso com regra falha).
```

**Etapa 2**
```
Etapa 2 — domínio puro. Leia CONTRATO §3 inteiro e §9 C3–C6, C14. Nada de I/O nesta etapa.
internal/domain:
- datas.go: Dia (tipo 'YYYY-MM-DD'), Hoje(clock, loc), InicioSemana (segunda, decisão A1), InicioMes/FimMes,
  AddDias; parse/format.
- regra.go: Regra (discriminada por tipo: diario, dias_semana{dias}, vezes_periodo{vezes,periodo},
  intervalo{dias}); ParseRegra(json) com validação de §3.2 (inclui vezes > dias do período → erro);
  mensagens de erro em português.
- ocorrencias.go: GerarOcorrencias(item, ocorrenciasExistentes, ate Dia) → novas (idempotente: nunca
  duplica inicio; respeita criado_em e pausa); para intervalo, a próxima nasce de ultimoRegistro+N
  (decisão A2); FecharVencidas(ocorrencias, hoje) → as que viram perdida (intervalo nunca vira perdida);
  StatusApos(ocorrencia, nRegistros) aplicando §3.3 regras 3–4; OcorrenciaDoDia(item, dia) para
  marcação retroativa (regra 6: dia sem ocorrência = registro extra).
- metricas.go: SequenciaAtual, MelhorSequencia, Taxa30d, EstadoIntervalo (em dia / atrasado N dias)
  exatamente como §3.6 (pulada transparente; a corrente aberta não quebra).
- horarios.go: parse de "HH:MM" e antecedências ("-15m", "-2h", "-1d"); AgendasDoDia(item, dia,
  ocorrenciaAberta) → []time.Time conforme §3.5 (hábito: só em dia com ocorrência aberta não cumprida;
  vezes_periodo: todo dia do período até bater a meta; compromisso: hora - antecedências).
Testes (C14): os 4 formatos gerando ocorrências por 60 dias atravessando fim de mês (fev com 28 e 29,
meses de 30 e 31) e virada de semana; idempotência; pausa; fechamento; sequência com pulada e com
período corrente aberto; taxa; intervalo atrasado; antecedências de compromisso.
```

**Etapa 3**
```
Etapa 3 — banco, aplicação e API. Leia CONTRATO §3, §4, §5 (menos as linhas de push e notificações),
§9 C1–C6.
internal/store: repositórios de item, ocorrencia, registro e job (Executado(nome, dataRef)/Marcar).
Escritas de múltiplas linhas sempre em transação.
internal/app: casos de uso que juntam domain + store — CriarItem (valida, grava, gera ocorrências até
hoje/fim do período), EditarItem (mudança de regra não reescreve ocorrências fechadas; refaz abertas de
hoje em diante), MudarEstado, Registrar (acha/decide a ocorrência pelo dia local do `em`, recalcula
status; compromisso cumprido → estado concluido), DesfazerRegistro (hard delete, decisão A6; recalcula),
Pular/Despular, Hoje (monta a resposta de GET /api/hoje), Calendario, ListarComMetricas. Deixe um
gancho `aoMudarOcorrencia` (interface vazia por ora) que a etapa 4 usará para gerar/cancelar
notificações.
Job gerar_ocorrencias (gera + fecha), idempotente por (nome, dia): roda no boot e na virada do dia
(o scheduler de verdade vem na etapa 4 — por ora exponha `app.RodarJobsDiarios(ctx)` e chame no boot).
internal/httpapi: rotas de §5 exceto /api/push/* e /api/notificacoes/*; JSON de erro {"erro": "..."}
com 400/404/409/500; validação no app, não no handler. Testes httptest ponta a ponta: C1 (criar os 4
formatos + compromisso com e sem data e ver /api/hoje), C2, C3 (1/3, 2/3, 3/3), C4, C5 (avançar o
relógio falso sobre 00:00), C6 (retroativo recupera sequência).
```

**Etapa 4**
```
Etapa 4 — notificações. Leia CONTRATO §3.5 inteiro, §5 (push e notificações), §7 (migração do spike),
§9 C7–C11.
internal/push: interface Pusher{Enviar(sub, payload) (status int, err)}; implementação webpush-go com
Urgency high, TTL 3600, subject de POCKET_PUSH_SUBJECT; VAPID lido de secrets/vapid.json no formato do
spike (gera se ausente); Fake que grava envios. Payload JSON: {notificacao_id, item_id, title, body}.
internal/app: gerar notificações do dia (job diário + ao criar/editar item + ao abrir ocorrência nova),
idempotente por UNIQUE(item_id, agendada_para, tipo); cancelar pendentes quando a ocorrência é cumprida
(C9); Adiar = nova notificação +30 min; Feito = Registrar com origem notificacao.
internal/scheduler: tick de 30 s (e um tick no boot): envia `pendente` com agendada_para ≤ agora;
vencida há > 30 min → expirada, sem enviar (decisão A3); roda os jobs diários quando o dia virar;
push 404/410 remove a push_sub; outro erro → notificacao falhou com detalhe. Nada de timer por item em
memória — só o tick lendo o banco (lição do A5).
httpapi: GET /api/push/vapid, POST /api/push/subscribe (upsert por endpoint), POST /api/push/teste,
POST /api/notificacoes/{id}/{recebida|feito|adiar|abrir}.
Testes com clock e pusher falsos: C7 (envia no minuto certo), C8 (adiar), C9 (cumprido cancela o
próximo), C10 (novo Server/scheduler sobre o mesmo banco com lembrete a 5 min → envia), C11 (relógio
salta 1 h → expiradas, nenhuma enviada), 410 remove a inscrição, tick concorrente não envia duas vezes.
```

**Etapa 5**
```
Etapa 5 — operação. Leia CONTRATO §7 e §9 C12.
internal/ops: interface Executor (exec com timeout; Fake); job renovar_cert diário que roda
"$POCKET_ACME --cron --home <dir do acme>" (POCKET_ACME vazio = desligado, log uma vez), loga saída
resumida; alerta de validade: se o cert vence em < 20 dias, 1 push por dia enviado direto pelo
Pusher, sem linha em `notificacao` (lá item_id é obrigatório) — a unicidade diária vem do job
(nome "alerta_cert", dia); backup diário `VACUUM INTO data/backup/pocket-YYYY-MM-DD.db` mantendo 7.
Jobs registrados no mesmo mecanismo (nome, dia) da etapa 3. GET /health completo: {ok, versao,
tick_em, cert_expira_em, subs}. Testes: executor falso chamado 1×/dia; alerta só abaixo de 20 dias e
1×/dia; rotação do backup mantém 7.
```

**Etapa 6** (no PC)
```
Etapa 6 — front. Leia CONTRATO §5 e §6 inteiros; veja servicos/smaug-dashboard/frontend (padrão Vite
da casa) e o index.html/sw.js do spike no histórico (git show master:servicos/pocket/web/sw.js) para o
fluxo de push que já funcionou no Poco.
servicos/pocket/frontend/: Vite + React + TypeScript, sem biblioteca de UI; cliente tipado das rotas
de §5; telas Hoje, Hábitos (lista + detalhe com calendário de 12 semanas e marcação retroativa),
Compromissos, Criar/editar (folha modal com o seletor de frequência em linguagem natural), Ajustes.
Mobile-first, tema escuro, alvos de toque ≥ 44 px, desfazer por 5 s após marcar. public/sw.js à mão:
cache do shell, push → showNotification com ações Feito/Adiar 30 min, reporta recebida,
notificationclick → feito/adiar/abrir. public/manifest.webmanifest SEM share_target (fase 4).
Toda abertura do app: se Notification.permission === "granted", garante a inscrição e faz
POST /api/push/subscribe. Sem rede: aviso "sem conexão com o redmilab".
Build: `npm run build` → servicos/pocket/dist/ (COMMITADO). Go: `//go:embed all:dist`, fallback de
SPA para index.html, sw.js e manifest com Cache-Control no-cache e Content-Type corretos.
Dev: vite dev com proxy /api → https://127.0.0.1:9104 (cert autoassinado no PC).
```

**Etapa 7** (no PC)
```
Etapa 7 — fechamento. README.md: trocar o topo por "como operar" (subir, .env, logs, testes, build do
front no PC), manter Decisões D1–D9 e adicionar "Decisões durante o build" consolidando todos os
// DECISÃO: do código. CHECKLIST-ACEITE.md com C1–C14 do CONTRATO §9 como caixas. go vet limpo.
```

## 5. Checklist de revisão (sessão Opus, por etapa)

- [ ] O diff só toca `servicos/pocket/` (e nada de `scripts/services.sh` — a entrada `pocket` já existe).
- [ ] Nenhum `time.Now()` fora de `internal/clock` e `main.go`; "hoje" sempre no fuso de `POCKET_TZ`.
- [ ] Datas no banco só como `'YYYY-MM-DD'`/RFC3339 local produzidos por `domain` — grep por `.Format(` fora de `domain`.
- [ ] Nenhum estado que importe vive só em memória: fila de notificações, jobs e ocorrências saem do banco no boot.
- [ ] Geração de ocorrências e notificações idempotente (UNIQUE + `INSERT … ON CONFLICT DO NOTHING`); rodar o job 2× não duplica.
- [ ] Toda escrita de várias linhas em transação; registrar/desfazer recalcula a ocorrência na mesma transação.
- [ ] Mudança de regra não altera ocorrências fechadas.
- [ ] Scheduler: vencida > 30 min vira `expirada`; ocorrência cumprida cancela pendentes; 404/410 remove inscrição.
- [ ] `POCKET_LISTEN` nunca `0.0.0.0`; nenhum segredo (`vapid.json`, `tls/`) no git; `data/` e `secrets/` ignorados.
- [ ] `vapid.json` do spike continua válido (mesmo formato) — a inscrição do Poco sobrevive ao deploy.
- [ ] Erros da API em português, `{"erro": …}`, sem stack trace.
- [ ] Testes table-driven, sem rede, relógio falso; `go vet` limpo.
- [ ] (Etapa 6) `dist/` commitado e embutido; `sw.js` sem cache; nenhum `share_target`; toque de "feito" com desfazer.

## 6. Depois do build (no PC)

1. Revisão final do Opus sobre `master..bot/pocket`.
2. `git checkout master && git merge --no-ff bot/pocket`, `git push celular master` → o hook compila e
   reinicia o `svc-pocket` (o spike sai do ar aqui).
3. Abrir o app no Poco: ele reinscreve sozinho. Marcar `CHECKLIST-ACEITE.md` (C1–C14) no aparelho.
4. Medir RAM (`ps -o rss=`) e anotar no README. Confirmar no log que o `renovar_cert` rodou.
5. Atualizar o resumo do README da raiz do redmilab (estado dos serviços) e avisar que o bloco precisa ser recolado no Claude.ai.
6. Próximo: contrato da fase 1.5 (captura rápida com Groq).
