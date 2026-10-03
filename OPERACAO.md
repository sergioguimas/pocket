# pocket — assistente pessoal (hábitos, tarefas, evolução, compras, receitas)

**Status:** fase 1 **no ar desde 26/09/2026** (merge `bcac8ab`; etapas 1–7 do
[PLANO-DE-BUILD.md](PLANO-DE-BUILD.md)); aguardando aceite no Poco
([CHECKLIST-ACEITE.md](CHECKLIST-ACEITE.md)). Fase 0: [FASE-0-SPIKE.md](FASE-0-SPIKE.md).
Contrato: [CONTRATO.md](CONTRATO.md) · API: [API.md](API.md) · Fase 3 (compras): [CONTRATO-FASE3.md](CONTRATO-FASE3.md),
em build (branch `bot/pocket-f3`; F3-1 commitada `454ac41` — migração, domínio, store, app e API
exceto lembrete de lista; F3-2 pronta — geração/envio do lembrete de lista no job diário/scheduler;
F3-3 pronta — 4ª aba, tela de compras, ajustes da lista, Hoje, fila offline em IndexedDB, `sw.js`
para `tipo = lista`; falta F3-4, ensaio da migração no aparelho + checklist K1–K10).

## Como operar

- **Endereço:** https://pocket.example.com:9104 (só na tailnet; 443 exige root).
- **Serviço:** `svc-pocket` (`scripts/services.sh`) → `cd $HOMELAB/servicos/pocket && ./bin/pocket`.
  O hook compila e reinicia a cada push em `master` que toque `servicos/pocket/` — nada de estado
  só em memória, então restart é seguro.
- **Configuração:** `.env` na pasta (fora do git), variáveis em CONTRATO §8. Defaults servem no aparelho.
- **Dados:** `data/pocket.db` (SQLite) e `data/backup/` (7 dias, `VACUUM INTO` diário).
  **Segredos:** `secrets/tls/` (cert do acme.sh) e `secrets/vapid.json` (não apagar: invalida a
  inscrição de notificação do Poco).
- **Logs:** `$HOMELAB/logs/pocket.log`. **Saúde:** `curl -s https://pocket.example.com:9104/health`
  (`ok=false` se o cert vence em < 7 dias ou o scheduler parou há > 5 min).
- **Certificado:** renovado pelo próprio serviço 1×/dia via `acme.sh --cron` (backoff de 1 h, até
  4 tentativas/dia); alerta por push quando faltam < 20 dias.
- **Testes:** `go test ./...` nesta pasta. **Front:** ver "Front (dev e build)" no fim — o build é
  feito no PC e `dist/` é commitado.

Nome: o *Pocket* de Tower of God — o objeto que faz tudo. Um app só, com módulos por dentro.

## O que é

Um PWA instalado no celular pessoal (Poco M6 Pro), servido pelo redmilab na tailnet:

| Módulo | Faz |
|---|---|
| Hábitos / tarefas / lembretes | rotinas fixas e compromissos pontuais, com notificação e cobrança |
| Evolução | hábito com valor registrado (página 142, ep. 7, 3×12 supino) e histórico |
| Compras | lista, marcar no mercado, resumo |
| Receitas | compartilhar reel/vídeo do Instagram/YouTube → extrai ingredientes e preparo → manda ingredientes pra lista |

## Decisões

| # | Decisão | Por quê | Alternativas descartadas |
|---|---|---|---|
| D1 | **Um app, módulos internos** | hábito, tarefa, lembrete e evolução são a mesma entidade (item + agenda + registros); receita → lista de compras é o maior ganho e só existe junto | vários apps: triplicaria scheduler/histórico e a integração receita→compras nunca seria feita |
| D2 | **Smaug fica separado e intocado** | está pronto e em aceite; Pocket não usa WhatsApp, então não há conflito | adicionar os módulos ao Smaug: roteamento de intenção ficaria ambíguo ("o que é o quê") |
| D3 | **Interface = PWA (tela primeiro)** | o uso real é visual: gráfico de evolução, riscar lista no mercado, navegar receitas | WhatsApp: conflita com o Smaug no mesmo chat (webhooks em fan-out) e é ruim pra consulta |
| D4 | **Notificação via Web Push; share target pra receitas** — ✅ confirmado na fase 0 | um ícone, uma interface; "Compartilhar → Pocket" no Instagram é o melhor fluxo de captura | Telegram: mais simples e com push garantido, mas vira segunda interface. **Plano B** se o push falhar no Poco (fase 0 decide) |
| D5 | **HTTPS por subdomínio `pocket.example.com` → IP tailnet do redmilab, cert Let's Encrypt via `acme.sh` DNS-01 (`dns_hostinger`)** | PWA/push/share target exigem contexto seguro; o Tailscale do aparelho é o app da Play Store (sem `tailscale cert`) | CA própria instalada no celular: frágil; Tailscale Serve/cert: indisponível no app Android |
| D6 | **Stack = a do smaug-dashboard**: Go + SQLite no aparelho, React + Vite buildado no PC, `dist/` commitado | reaproveita padrão já validado; Node nunca roda no aparelho | — |
| D7 | **Ocorrências com status no schema desde a fase 1** | cobrança ("não marcou o treino hoje") vira só um job sobre ocorrências pendentes vencidas | derivar cobrança depois: migração dolorosa |
| D8 | **Receitas: metadados primeiro, vídeo pro Gemini depois, Groq/Whisper só de reserva** | quantidades costumam estar na legenda ou escritas na tela — transcrição de áudio sozinha perde isso | só yt-dlp + Whisper: perde texto na tela e legenda |
| D9 | **Instagram via `yt-dlp` com cookies de conta secundária** | muitos reels exigem login | cookies da conta principal: risco de bloqueio |

## Ordem

0. **Spike de infra** — [FASE-0-SPIKE.md](FASE-0-SPIKE.md). Decide D4 (PWA vs. Telegram).
1. Núcleo (itens, agendas, ocorrências, registros) + hábitos/tarefas/lembretes + push.
1.5. Captura rápida por texto/voz (Groq gratuito: Whisper + gpt-oss) — o LLM só preenche o formulário. Contrato: [CONTRATO-FASE15.md](CONTRATO-FASE15.md) (fechado 28/09).
2. Evolução + cobrança.
3. Compras.
4. Receitas + share target + receita → lista.

## Riscos conhecidos

- **HyperOS (Poco) mata apps em segundo plano** — o push depende do Chrome sem restrição de bateria. É exatamente o que a fase 0 mede.
- **Instagram + yt-dlp** é o ponto mais frágil do projeto; por isso receitas é a última fase.
- **Cert de 90 dias** depende da renovação do `acme.sh` rodar — e nada no aparelho tem autostart garantido (Termux:Boot não dispara). Renovação precisa ser verificável na dashboard.
- Nome de serviço: `svc-pocket` (reservado). `svc-claude-pocket` é o Remote Control, não este app.

## Decisões durante o build

Decisões que o [CONTRATO.md](CONTRATO.md) não cobria, tomadas na sessão Opus durante o build da fase 1
(o código marca as locais com `// DECISÃO:`).

| Etapa | Decisão | Por quê |
|---|---|---|
| 1 | `.env` carregado com `godotenv` no boot (ausente = só ambiente real) | mesmo padrão do Smaug; §8 fala em `.env` na pasta |
| 1 | `POCKET_ACME` **definida vazia** desliga a renovação; **ausente** = default `~/.acme.sh/acme.sh` (`~` expandido por `HOME`) | §8: "vazio desliga" — as outras variáveis tratam vazio como default |
| 1 | SQLite com uma conexão só (`SetMaxOpenConns(1)`) e `_txlock=immediate` | padrão do Smaug; um processo, WAL |
| 1 | Log `slog` em texto; `Shutdown` com prazo de 10 s | legível no tmux; não segura o restart do deploy |
| 1 | `webpush-go` sai do `go.mod` até a etapa 4 (o `go mod tidy` remove dependência sem uso) | volta quando `internal/push` existir |
| 2 | Geração só do dia/período corrente, **sem catch-up**: dia em que o serviço ficou fora não tem ocorrência (neutro — nem perdida, nem quebra sequência; mesma lógica da pausa) | literal de §3.3 regra 1; o banco não guarda histórico de pausa |
| 2 | `vezes_periodo` criado no meio do período: 1º período vai da criação ao fim do período, meta = min(vezes, dias restantes); teto de `vezes` mensal = 28 | não cobrar meta impossível; 28 cabe em qualquer mês |
| 2 | `intervalo`: 1ª ocorrência vence no dia da criação; registro (mesmo antecipado) fecha a aberta com `fim` = dia do registro e abre a próxima em dia + N; registro com dia ≤ `fim` da última cumprida é extra | A2; mantém `inicio` único |
| 2 | Compromisso nunca vira `perdida`: fica aberto e aparece como atrasado até ser concluído | §3.3 regra 2 fala de hábitos; §6 mostra compromissos atrasados |
| 2 | Registro em ocorrência `pulada` que bate a meta a torna `cumprida`; despular recalcula | regra 4 não cobre; o registro é o fato mais forte |
| 2 | Horários: hábito só `HH:MM`; compromisso com hora só antecedências (sempre há o lembrete na hora); compromisso sem hora e com data só `HH:MM`; sem data, nenhum lembrete | §3.5 |
| 2 | Taxa 30 d sem ocorrências fechadas = "sem taxa" (não 0 %) | evita mostrar 0 % em hábito novo |
| 3 | `em` do registro aceita RFC3339 ou `YYYY-MM-DD` (= 12:00 local, marcação retroativa); no futuro → 400 | calendário marca por dia |
| 3 | Pausar/arquivar hábito: ocorrências abertas com `fim` viram `pulada` (a de intervalo fica); retomar gera a do dia/período | pausa não pode virar perdida nem quebrar sequência (§3.1) |
| 3 | Compromisso: concluir via `/estado` = registrar agora; reabrir só desfazendo o registro (`concluido→ativo` = 409); `hora` exige `data` | um caminho só para "cumprido" |
| 3 | Transições de estado inválidas ou para o mesmo estado = 409 | explícito para o front |
| 3 | Mudança de regra/data apaga as abertas não vencidas e regenera para hoje; intervalo sem aberta renasce em max(última feita + N, maior `inicio` + 1) | §5 PATCH; nunca colide no UNIQUE |
| 3 | Desfazer em intervalo só reabre se o registro fechou a **última** cumprida e não sobrou outro registro no dia | histórico antigo fica |
| 3 | Formatos de request/response documentados em [API.md](API.md) (base da etapa 6) | o contrato não fixava o JSON |
| 4 | `Observador.ItemMudou`/`OcorrenciaMudou`/`OcorrenciaApagando` não devolvem erro (interface fixada assim na etapa 3) — uma falha ao gerar/cancelar notificação é só logada (`slog`), não derruba o caso de uso principal (criar item, registrar, etc.) | mudar a assinatura quebraria o contrato do gancho já commitado; a confiabilidade do módulo de notificação não deveria travar a API síncrona do usuário |
| 4 | `App.SetPush(pusher, vapidPublic)` em vez de acrescentar parâmetros a `New(...)` | `New(st, clock, loc, obs)` já é chamado assim em todos os testes das etapas 1–3; mudar a assinatura quebraria todos eles por um recurso que a maioria não usa |
| 4 | Notificações são geradas só para "hoje" (mesmo espírito da decisão 2/etapa 2: sem catch-up) — o job diário roda `gerarNotificacoesHoje` para todo item ativo, e `ItemMudou`/`OcorrenciaMudou` fazem o mesmo na hora, para não esperar o job do dia seguinte | literal de §3.5 ("job diário + ao criar/editar item + ao abrir ocorrência nova"); como o job diário roda todo dia (idempotente), os dias futuros de um compromisso com antecedência são cobertos quando "hoje" chega neles — não precisa pré-calcular o futuro |
| 4 | Um job só (`gerar_ocorrencias`) cobre ocorrências e notificações do dia | mesma cadência (uma vez por dia); dois jobs separados só duplicariam o mecanismo (nome, dia) sem ganho |
| 4 | Scheduler reivindica a notificação (`UPDATE ... WHERE status='pendente'`) ANTES de chamar o Pusher, não depois | é o que garante que um tick concorrente (ou dois ticks muito próximos) nunca manda o mesmo push duas vezes — só quem reivindica a linha continua |
| 4 | Push manda pra **todas** as inscrições de uma notificação (não só a primeira) | `push_sub` já é uma tabela de N dispositivos (schema da fase 1); mandar pra todas é o comportamento do spike e cobre trocar de aparelho sem perder o lembrete no antigo até expirar |
| 4 | `POST /api/notificacoes/{id}/feito` e `.../adiar` escrevem em duas transações (a de `Registrar`/inserir a notificação nova, depois a de marcar a ação), não uma só | `Registrar` já abre sua própria transação (é um caso de uso completo, com Observador); `database/sql` não suporta transação aninhada — o risco (marcar a ação falhar depois do efeito principal já ter acontecido) é aceito |
| 4 | `secrets/vapid.json` é lido no formato literal do spike (`{"Public","Private"}`) e só é gerado se ausente — nunca reescrito quando já existe | preserva a inscrição já feita no Poco através do deploy da fase 1 (CONTRATO §7); testado (`push/vapid_test.go`) checando que o arquivo não muda de mtime/conteúdo |
| 5 | `ops.Jobs.Tick` roda `RodarJobsDiarios` numa goroutine própria (com `sync.Mutex.TryLock` para nunca ter duas em voo) em vez de rodar dentro de `scheduler.Tick` como o `gerar_ocorrencias` da etapa 3 | `renovar_cert` pode levar até 10 min (timeout); rodar síncrono travaria o envio de notificações pendentes pelo mesmo tempo, já que o scheduler só tem uma goroutine de tick. `TryLock` faz os ticks de 30 s que caem durante uma execução em voo só a ignorarem (idempotente por (nome, dia): quando ela termina, o próximo tick reencontra o job já marcado) |
| 5 | `renovar_cert`, `alerta_cert` e `backup` são três jobs (nome, dia) independentes, não um só | são falhas independentes (acme.sh fora do ar não deve impedir o backup do dia) e a etapa 5 chama isso de "jobs" no plural — diferente da etapa 4, que uniu ocorrência+notificação por serem a mesma cadência e dependentes uma da outra |
| 5 | `renovar_cert` só marca o job como feito quando o Executor não devolve erro; falha tenta de novo, mas com **backoff em memória**: no máximo 1 tentativa/hora e 4/dia (`Jobs.renovarUltimaTentativa`/`renovarTentativas`, zerados a cada dia novo; restart zera também — aceitável, pior caso é "mais uma tentativa hoje"); na 4ª falha do dia, loga 1x e só volta a tentar amanhã | revisão do coordenador: sem backoff, o tick de 30s tentaria de novo a cada falha (~120×/h) — cada tentativa cria um TXT na Hostinger e um pedido de validação na CA, e a Let's Encrypt limita 5 validações falhas/hora por conta+hostname; o Pocket se auto-bloquearia bem antes do problema real (Hostinger/CA fora do ar) se resolver |
| 5 | Depois de `renovar_cert` rodar com sucesso, o job chama `certLoader.Get(nil)` explicitamente antes de calcular `alerta_cert` | sem isso, `alerta_cert` (no mesmo `RodarJobsDiarios`) podia ler a validade antiga: `CertLoader` só relê o arquivo dentro de `Get` (chamado normalmente por uma requisição TLS), não em `Validade()` |
| 5 | Alerta de certificado: `title` carrega o texto literal do contrato ("Pocket: certificado vence em N dias"), `body` carrega a dica ("confira o log do renovar_cert") | mesmo formato de payload `{notificacao_id, item_id, title, body}` do scheduler (etapa 4) e de `TestarPush`, para o `sw.js` não precisar de um caso especial; `notificacao_id`/`item_id` ficam 0 (não existe linha em `notificacao`, CONTRATO §7) |
| 5 | Backup: checa `os.Stat` do arquivo do dia *antes* de rodar `VACUUM INTO` (em vez de deixar o SQLite falhar por arquivo já existente) | literal do contrato ("se o arquivo do dia já existir, não refaz"); falha de `VACUUM INTO` sobre arquivo existente seria tratada como erro do job, o que reagendaria uma tentativa desnecessária a cada tick |
| 5 | Falha ao rotacionar backups antigos (apagar os além dos 7) não impede marcar o job como feito | o backup do dia já foi gravado com sucesso; não marcar faria o job tentar `VACUUM INTO` nos ticks seguintes sobre um arquivo que já existe — pior que só deixar um backup extra sobrando até o próximo dia |
| 5 | `GET /health` ganha `SetSaude(cert, tick)` (setter) em vez de mais parâmetros em `httpapi.New` | mesmo padrão de `App.SetPush`/`Scheduler.SetOpsJobs`: `New(st, ap, versao)` já é chamado assim em todos os testes das etapas 1–4; sem `SetSaude`, `tick_em`/`cert_expira_em` saem `null` e não entram no cálculo de `ok` (comportamento antigo preservado) |
| 5 | `ok=false` por tick ou cert só é avaliado quando `SetSaude` foi chamado (senão os dois campos saem `null` e não pesam no cálculo) | permite os testes/ambientes que não injetam `CertInfo`/`TickInfo` continuarem validando só `ok = banco responde`, como nas etapas 1–4 |
| 6 | Sem React Router nem biblioteca de estado: navegação é um `useState<Tela>` discriminado em `App.tsx` (5 telas + a folha modal do editor por cima) e cada tela busca seus próprios dados com `fetch` + hooks simples | o contrato só pede React Router "se precisar" — 5 telas fixas com uma pilha rasa (lista → detalhe) não justificam a dependência; abas como estado já era a sugestão do próprio contrato |
| 6 | `//go:embed all:dist` fica em `main.go`, não em `internal/httpapi` | `dist/` é vizinho de `main.go` (`servicos/pocket/dist`), não de `internal/httpapi/`, e `go:embed` não aceita `..`; `httpapi.SetEstatico(fs.FS)` segue o mesmo padrão de setter de `SetPush`/`SetSaude`/`SetOpsJobs` (main.go faz `fs.Sub(distFS, "dist")` e injeta) |
| 6 | `GET /` cai no próprio mux de `httpapi.Server` (rota de menor prioridade do `ServeMux` de Go 1.22+, nunca sombreia `/api/*` nem `/health`) em vez de um mux separado em `main.go` | mantém um único `http.Handler` (como já era); qualquer `/api/*` sem rota registrada devolve `404` de verdade (checado explicitamente em `handleEstatico`), nunca o `index.html` |
| 6 | Editor de criar/editar (`EditorSheet`) sempre mostra os campos do tipo já decidido (`habito`/`compromisso`) — não existe uma tela "escolher tipo" separada; ao criar pelo botão **+**, o tipo inicial é `compromisso` na aba Compromissos e `habito` nas demais | o contrato pede "tipo → título → ..." como ordem de campos dentro da folha, não uma tela própria; o botão **+** já sabe o tipo mais provável pela aba ativa, e trocar de tipo no meio da criação normalmente significa recomeçar de qualquer forma (o corpo do formulário muda inteiro) |
| 6 | Antecedências de compromisso: 3 chips fixos (`-15m`, `-1h`, `-1d`) + campo "personalizada" (número + unidade m/h/d) que adiciona um chip removível — sem limite de quantas antecedências | API.md não limita a lista de `horarios` de antecedência; deixar quantas o usuário quiser é mais simples que impor um teto arbitrário no front |
| 6 | Desmarcar retroativamente no calendário do hábito (mapa de calor) só funciona para dias com registro dentro dos **últimos 30** (`GET /api/itens/{id}/registros` devolve só isso, via `GET /api/itens/{id}`); dias marcados mais antigos ficam com o toque desabilitado (tooltip indica o motivo) | a API não expõe id de registro por dia fora dessa janela — inventar uma rota nova estava fora do escopo da etapa 6 (só consome `API.md` como está); 30 dias cobre a janela realista de "toquei errado, quero desfazer" |
| 6 | `sw.js`: cache do shell é `network-first` para navegação (documento) com fallback para o `index.html` cacheado, e `cache-first` só para `/assets/*` (nomes com hash do build, nunca mudam de conteúdo sob o mesmo nome); `/api/*` e `/health` nunca passam pelo cache | literal de CONTRATO §6 ("dados sempre da rede"); hash no nome do arquivo torna `cache-first` seguro para `/assets/*` sem precisar de lista gerada no build |
| 6 | Ícones do manifest (192/512, maskable) gerados por um script Node de uma linha (PNG RGBA manual com zlib, sem `node-canvas`) desenhando um círculo ouro sobre o fundo `#141a2e` — reaproveita a ideia do spike, sem nova dependência de build | instalar `sharp`/`canvas` só para dois ícones estáticos era desproporcional; o resultado (círculo sólido) já cobre o "maskable" (fica dentro do círculo seguro) |
| 6 | `POST /api/itens/{id}/estado {"estado":"concluido"}` é o único caminho para concluir um compromisso em Hoje/Compromissos (a UI não chama `POST /registros` diretamente para compromisso) | API.md/CONTRATO §5 dizem que `/estado → concluido` "equivale a `POST /registros` agora" — usar o endpoint de estado deixa a intenção explícita na UI e evita duplicar a lógica de "qual rota conclui o quê" |
| 6 | Revisão (Opus): botão "+" flutua acima da barra inferior (antes ficava dentro dela, apagado pelo seletor `.bottom-nav button` e cobrindo a aba Compromissos) | bug visto no teste local |
| 6 | Revisão (Opus): hábito `intervalo` mostra só "em dia / vence hoje / atrasado", sem sequência nem taxa, na lista e no detalhe | CONTRATO §3.6 |
| 6 | Revisão (Opus): `ErroGlobal` (error boundary) envolve o app — erro de renderização mostra mensagem + recarregar, nunca tela branca | CONTRATO §6 |
| 6 | Revisão (Opus): `aria-label` nos dias do calendário e nos chips de dia da semana (S/Q se repetem), `role=dialog` na folha do editor | acessibilidade |
| F3-1 | Migração 0002: sem `PRAGMA foreign_keys=OFF` na reconstrução de `notificacao` | nada no schema referencia `notificacao` por FK (só ela referencia item/lista/ocorrência para fora), e as linhas copiadas da tabela antiga ficam com `lista_id` NULL (FK nunca reprova NULL) — não havia risco real a evitar |
| F3-1 | `notificacao.item_id`/`lista_id` — upsert de `NotificacaoInserir` usa **dois `ON CONFLICT` encadeados** (`WHERE item_id IS NOT NULL` / `WHERE lista_id IS NOT NULL`, SQLite 3.35+) num INSERT só, em vez de decidir em Go qual índice checar | o CHECK do schema já garante que só um dos dois nunca é nulo ao mesmo tempo; encadear os dois conflitos cobre ambos os casos sem duplicar a função `notificacaoInserir` |
| F3-1 | `domain.Notificacao.ItemID` virou `*int64` (era `int64`) | a partir da migração 0002 uma notificação pode ser de lista (`ItemID` nil) — o tipo já teria que mudar na fase 3 de qualquer forma; ações "feito"/"adiar" (`app/push.go`) e o envio do scheduler tratam `ItemID == nil` como 409/falha defensiva (lembrete de lista não tem essas ações, §2.6, e o envio em si é F3-2) |
| F3-1 | `NomeChave` roda a remoção de plural **antes** de remover acento, com exceção explícita para as terminações `-ães`/`-ões`/`-ãos` | bate o exemplo literal do contrato ("pães" → "paes"): tirar o "s" veria "e" como vogal e daria "pae", errado; plural nasal de "-ão" não é "s após vogal simples" mesmo, então vira exceção em vez de generalizar a regra |
| F3-1 | Seção automática (§2.3 regra 2) e histórico (§2.4) ficam só em `app`, sem tabela/índice dedicado além de `compra_historico` já definida no contrato | o próprio schema de §4 já cobre a consulta (`PRIMARY KEY (lista_id, nome_chave)`); nenhuma decisão extra necessária |
| F3-1 | `ListaPatch.Lembrete` usa `json.RawMessage` **não-ponteiro** no DTO do httpapi (não `*json.RawMessage`, ao contrário do `Regra` de item na fase 1) | aqui "ausente" e "`lembrete: null`" precisam de sentidos diferentes (não mexe vs. remove, §5) — só o `json.RawMessage` sem ponteiro preserva os 4 bytes literais `"null"` como não-nil; com ponteiro os dois casos colapsariam no mesmo nil, como a fase 1 fez de propósito para `Regra` (lá os dois sentidos são iguais) |
| F3-1 | Lembrete de lista exige **pelo menos um horário** (`400` se `horarios` vier vazio) | um lembrete sem horário nunca dispararia — mais simples recusar na validação do que gravar um lembrete morto |
| F3-1 | `DELETE /api/compras/{id}` e `DELETE /api/secoes/{id}` devolvem `204` sem corpo (a fase 1 não tinha rota DELETE "de verdade" para comparar) | convenção HTTP padrão para remoção sem conteúdo a devolver |
| F3-1 | `Riscar`/`Desriscar` fazem upsert/desfeita de `compra_historico` numa Tx só com a atualização do item (mesmo padrão da fase 1: um caso de uso, uma transação) — `Desriscar` **apaga** a linha do histórico quando `vezes` chega a 0 (em vez de manter em 0) | "riscar e desriscar no mesmo dia" não deveria deixar rastro de "já foi comprado uma vez" nos frequentes; contrato não especifica, mas isso é o que faz `Frequentes` (que já filtra por existência da linha) continuar correto sem um caso especial de "vezes=0" |
| F3-1 | **Bug da fase 1 corrigido**: compromisso "C10 teste" concluído às 11:31:47 (item `concluido`, ocorrência `cumprida`); às 11:31:49 "Adiar 30 min" na notificação (já `enviada` antes da conclusão — a conclusão só cancela `pendente`) criava uma notificação nova pendente para um item que não está mais aberto. Correção em duas camadas: (1) `app.acaoFeito`/`acaoAdiar` checam `notificacaoAindaAcionavel` (item `ativo` e, se houver, ocorrência ligada `aberta`) antes de agir — se não, gravam a ação mesmo assim e devolvem `409 "item já concluído"`, sem registrar/reagendar; (2) `scheduler.enviar`, depois de reivindicar a notificação como "enviada" mas antes de chamar o Pusher, roda a mesma checagem (`itemAindaAtivo`) e, se falhar, sobrescreve para `cancelada` (`Store.NotificacaoMarcarCanceladaClaimed`, incondicional — a linha já está reivindicada) em vez de mandar o push | defesa em duas camadas: a do `app` cobre o caminho normal (usuário toca a ação pela API), a do `scheduler` cobre qualquer corrida em que uma notificação pendente sobreviva à mudança de estado do item por fora do Observador (ex.: o próprio intervalo entre o job gerar o lembrete e o tick que ia enviá-lo) |
| F3-2 | Listas não têm um gancho de `Observador` próprio — `app.sincronizarNotificacoesListaHoje` é chamada direto por `CriarLista`/`EditarLista`/`MudarEstadoLista` (dentro da mesma Tx), em vez de introduzir uma segunda interface tipo `ObservadorLista` | uma lista só tem 3 casos de uso que mudam algo relevante para notificação (criar não seta lembrete — só `POST /api/listas {nome}` — então nem precisa sincronizar); duplicar a interface do Observador de item pra cobrir só isso era complexidade sem ganho |
| F3-2 | Lembrete de lista gera a notificação de hoje mesmo com o horário já passado (mesmo espírito de `gerarNotificacoesHoje` de item, fase 1) — não filtra "já passou" na geração | o scheduler já expira (sem enviar) quem venceu há mais de 30 min no momento do envio (tolerância de sempre, decisão A3); reaproveitar esse mecanismo evita duplicar "não gera se já passou" |
| F3-2 | `MontarResumoCompras` (domain, pura) usa **até 3 seções nomeadas** antes de agregar o resto em `"+N"` (não 2) | o exemplo do próprio CONTRATO-FASE3 §2.6 ("hortifrúti 3, laticínios 2, +2") tem só 2 nomes antes do "+2", mas o pedido da etapa F3-2 pede testes explícitos para "3 seções" (sem agregado) e "mais de 3 seções" (com agregado) — só faz sentido com o limite em 3; o exemplo do contrato foi tratado como ilustrativo, não literal |
| F3-2 | `scheduler.enviar` foi refeito em torno de `montarPayloadOuCancelar` (item **ou** lista, despachado por `n.ItemID`/`n.ListaID`) — a checagem "cancelar em vez de enviar" (item não mais ativo/ocorrência fechada, **ou** lista arquivada/vazia) e a montagem do payload viraram uma função por tipo, compartilhando o helper `cancelar` (que chama `NotificacaoMarcarCanceladaClaimed`, já existente do bug da F3-1) | mesma decisão de F3-1: contagem de pendentes "no momento do envio" (§2.6) exige os mesmos dados (`ItensPorLista`) que já se buscava para checar "vazia"; separar por tipo em vez de um `if` gigante deixa cada caminho (item vs. lista) legível sozinho |
| F3-2 | Payload de item ganhou o campo `tipo: "lembrete"` (era só `notificacao_id`/`item_id`/`title`/`body`) — aditivo, não quebra o `sw.js` da etapa 6 (que ignora campos desconhecidos) | o contrato pede `tipo` nos dois payloads para o `sw.js` decidir mostrar ou não as ações — mais simples ter o campo nos dois desde já do que o `sw.js` ter que tratar "ausência de tipo = lembrete de item" como um caso especial |
| F3-2 | Mensagem de erro de `feito`/`adiar` em notificação de lista unificada para `"lembrete de lista não tem essa ação"` (F3-1 tinha duas mensagens, uma por ação, citando o nome da ação) | pedido explícito da revisão; uma mensagem só é mais simples e o `acao` em si já aparece no path da rota (`/api/notificacoes/{id}/{acao}`), então repetir o nome da ação na mensagem era redundante |
| F3-3 | `src/parseItem.ts`: número isolado no início **ou** no fim vira quantidade mesmo quando não é o único token restante (ex.: `"1 cx bombom"` → nome `"cx bombom"`, quantidade `"1"`) | leitura literal do contrato ("número isolado no início ou no fim") — não exigia que o resto virasse um nome de uma palavra só; a alternativa (só separar quando sobra 1 token) não estava escrita em nenhum lugar |
| F3-3 | Checagem de `parseItem` em `frontend/scripts/testParseItem.ts`, rodado com `node --experimental-strip-types` (Node 22.17 lê `.ts` direto, sem compilar) — sem framework de teste, fora do `npm run build` (script `test:parse` à parte) | o pedido pedia "o mais simples"; instalar Vitest só para uma função pura era desproporcional, e `--experimental-strip-types` evita duplicar a lógica num `.mjs` que pudesse divergir do `.ts` real |
| F3-3 | `nome_chave` do front (`chaveAproximada` em `Compras.tsx`) é uma normalização simples (minúsculas, sem acento, sem espaço duplicado) — **não** replica a regra de plural do servidor (§2.5) | serve só para o dedupe/otimismo visual antes da resposta do servidor (que é a fonte da verdade); replicar a regra exata do backend (exceções de `-ães`/`-ões`/`-ãos`) no front arriscava divergir silenciosamente — o pior caso de errar é o mesmo já aceito pelo contrato: "um item duplicado ou um frequente a mais" até o servidor responder |
| F3-3 | Fila offline (`src/offline.ts`) trata riscar/desriscar/adicionar como **sempre** otimistas e enfileirados — mesmo online — em vez de só quando `navigator.onLine` é falso | o próprio §3 do contrato descreve o fluxo como "otimista" sem condicionar a estar offline; unificar os dois caminhos (online e offline) evita duplicar a lógica de "chamar a API direto" vs. "enfileirar", e drena a fila imediatamente quando há rede — sem diferença perceptível de latência |
| F3-3 | Erro 5xx (ou qualquer erro que não seja `ApiError` 4xx) na drenagem da fila é tratado como falha de rede (para e tenta depois), não como "descarta a operação" | o contrato só define o caso 4xx explicitamente ("erro 4xx... descarta"); um 5xx é falha do servidor, não da operação em si — descartar arriscaria perder uma mudança válida por uma instabilidade passageira do serviço |
| F3-3 | Drenagem da fila roda por instância da tela Compras (ao montar, no evento `online`, e a cada 20s enquanto há fila) — não num worker/serviço global do `App.tsx` | mais simples: a fila só é populada por ações dentro da própria tela Compras (riscar/desriscar/adicionar), então não há necessidade de drenar fora dela; ao reabrir a aba (ou o app) a drenagem roda de novo e pega o que sobrou |
| F3-3 | `POST /api/listas/{id}/itens` offline usa payload completo (`nome`, `quantidade`, `origem`, `secao_id?`) já resolvido no momento do enfileiramento, em vez de reconstruir a partir do id provisório na hora de drenar | o parser e a seção sugerida (`Outros`/primeira seção) já rodaram no clique — reenfileirar só o texto bruto exigiria repetir essa lógica na drenagem; guardar o payload pronto mantém `offline.ts` sem depender de `parseItem`/regras de seção |
| F3-3 | `AjustesListaSheet`/ações de item (editar, mover, apagar, seções) ficam com `disabled`/`fieldset disabled` quando offline, mas continuam **visíveis** (não somem da tela) | F2 diz que essas ações "exigem rede" — desabilitar deixa claro o motivo sem esconder a interface; a mensagem "essas ações exigem rede"/o banner cobre o "por quê" |
| F3-3 | `App.tsx` lê `?lista=` só uma vez (estado inicial via `useState(() => ...)`) e limpa o parâmetro da URL com `history.replaceState` antes do primeiro render de `Compras` | evita reabrir a mesma lista num F5 manual (a URL já está limpa) e evita depender de um `useEffect` que rodaria depois do primeiro paint |
| F3-3 | Barra inferior: fonte do rótulo das 4 abas reduzida de 11px para 10px (nenhuma abreviada) | pedido explícito: "se Compromissos não couber, reduza a fonte para todas, não abrevie" — testado em 393px de largura no Browser pane, as 4 abas cabem com folga e o botão "+" flutuante não cobre nenhuma |
| F3-3 | Revisão (Opus): contador do chip da lista aberta calculado pela tela (antes ficava no valor do carregamento e mostrava 0 com itens pendentes) | bug visto no teste local |
| F3-3 | Revisão (Opus): botão "add" → "adicionar"; faixa offline sem redundância ("sem conexão — 2 mudanças pendentes") | pt-BR / clareza |
| pós-F3 (28/09) | Ícones SVG com `width/height` 20 padrão (sem isso a seta da lista de hábitos e o "voltar" ocupavam o container inteiro e espremiam o título) | bug relatado pelo Sergio |
| pós-F3 (28/09) | Anotar o que foi feito: `PATCH /api/registros/{id}` + "Anotar" na barra de desfazer de Hoje (8 s, pausa com a folha aberta) + registros tocáveis no detalhe do hábito | pedido do Sergio: acompanhar evolução além da constância; o campo `nota` já existia desde a fase 1 |
| pós-F3 (28/09) | Barra de desfazer sobe para cima do "+" flutuante (estavam sobrepostos) | visto no teste local |
| F1.5 (28/09) | 429/502 depois de transcrever devolvem também `texto`; o front o põe no campo com "salvar como tarefa" | V3: o recado falado não se perde quando só o LLM falha |
| F1.5 (28/09) | "fiz X" de item que não existe vira `criar_item` de **hábito** (não compromisso) | "fiz X" quase sempre é hábito ainda não cadastrado |
| F1.5 (28/09) | Prompt: hora sem data = hoje (amanhã se já passou); recado sem data nem hora = compromisso sem data | "me lembra às 15h de ligar pro João" é o recado mais comum |
| F1.5 (28/09) | "Tem mais" mora no App, não em Hoje | salvar no EditorSheet recria Hoje (`key`) e o resto sumia — visto no teste local |
| F1.5 (28/09) | `POCKET_GROQ_URL` (só dev) | testar a tela contra um Groq falso local sem gastar cota |
| F1.5 (28/09) | 429 = "limite do Groq atingido — espere um minuto"; bateria com 20 s de pausa | gpt-oss-120b gratuito: **8 mil tokens/min** (medido) — ~2–3 capturas/min; o limite diário (1.000 req) não é o que aperta |
| F1.5 (28/09) | **Bateria de 30 frases contra o Groq real: 29/30 (96,7%)**, ~1.860 tokens de prompt + ~300 de resposta por chamada. Única falha: "tomar remédio a cada 8 horas" virou `criar_item` (esperado `desconhecido`) — aceitável, o formulário abre para ajustar | régua para mexer no prompt depois (`go test -tags groq -run Bateria ./internal/captura/` no redmilab) |
| F1.5 (28/09) | Conexão própria (`conexao.ts`): falha de rede/prazo marca offline, sondagem de `/health` a cada 10 s marca a volta; prazo de 12 s em toda chamada (75 s na captura) e "cancelar" no "Entendendo…"; áudio guardado para "tentar de novo" sem regravar | aceite V10: com Tailscale ligado o Android segue `navigator.onLine = true` em modo avião e o fetch fica pendurado — app preso em "Entendendo…" |
| F1.5 (28/09) | Service worker: navegação network-first com prazo de 4 s, depois o shell do cache | aceite V10: "em modo avião o Pocket não abre" (mesma causa) |
| F1.5 (28/09) | Atalho "Falar com o Pocket" exige o Chrome regerar o WebAPK instalado (ele relê o manifest periodicamente) ou reinstalar o app | aceite V11: só aparecia "configurações do site" |
| F1.5 (28/09) | `GROQ_API_KEY` do Pocket copiada do `.env` do Smaug **sem o comentário da linha** | o valor bruto com "# comentário" quebrava o header Authorization fora do godotenv |

## Front (dev e build)

- **Dev no PC:** duas janelas de terminal. Uma roda o backend (`go run .` em
  `servicos/pocket/`, com `POCKET_LISTEN=127.0.0.1:9104` e um cert
  autoassinado em `secrets/tls/` — §8). A outra roda `cd frontend && npm
  install && npm run dev` — abre em `http://127.0.0.1:5173` com proxy de
  `/api` e `/health` para `https://127.0.0.1:9104` (`vite.config.ts`,
  `secure: false` porque o cert de dev não é assinado por uma CA confiável).
- **Build:** `cd servicos/pocket/frontend && npm install && npm run build`
  roda `tsc -b && vite build` e escreve em `servicos/pocket/dist/` — pasta
  **commitada** (não está no `.gitignore` da raiz; `node_modules/` sim está,
  globalmente). O binário Go embute `dist/` via `//go:embed all:dist` em
  `main.go` — depois de trocar o front, é preciso rodar `npm run build` de
  novo e re-commitar `dist/` antes do próximo `go build`/deploy (o embed lê
  o conteúdo do disco no momento da compilação do binário, não em runtime).
- **Ícones do manifest:** `frontend/public/icon-192.png` e `icon-512.png`
  já estão commitados (gerados por um script de uma linha, decisão da
  tabela acima); só regenerar se a identidade visual mudar.
- **Verificação:** `npm run build` sem erro de TypeScript (`tsc -b`
  roda antes do `vite build` e falha o comando se houver erro de tipo);
  `go vet ./... && go test ./...` na pasta `servicos/pocket` (inclui
  `internal/httpapi/estatico_test.go`: índice em `/`, fallback de SPA em
  `/habitos`, `sw.js`/manifest com `Cache-Control: no-cache` e o
  `Content-Type` do manifest); `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go
  build -o /dev/null .` para confirmar que o binário ainda cross-compila
  para o Poco com o `dist/` embutido.
- **Checagem do parser de "adicionar item" (fase 3):** `cd
  servicos/pocket/frontend && npm run test:parse` roda
  `scripts/testParseItem.ts` com `node --experimental-strip-types` (Node
  22.6+; sem framework de teste, decisão da tabela acima) — não faz parte
  de `npm run build`, roda à parte.
- **Cert autoassinado de dev (§8):** `secrets/tls/` não é commitado.
  Gerar localmente antes de rodar `go run .`: `openssl req -x509
  -newkey rsa:2048 -nodes -keyout secrets/tls/key.pem -out
  secrets/tls/fullchain.pem -days 30 -subj "/CN=127.0.0.1"` (no Git Bash
  do Windows, prefixar com `MSYS_NO_PATHCONV=1` para o `/CN=...` não virar
  caminho de arquivo).
