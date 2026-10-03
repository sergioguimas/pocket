# Pocket — API HTTP (fase 1, etapas 1–5; fase 3, etapas F3-1/F3-2)

Referência para quem constrói o front (etapa 6 / F3-3) sem ler o código Go — só este arquivo e
[CONTRATO.md](CONTRATO.md)/[CONTRATO-FASE3.md](CONTRATO-FASE3.md) (especialmente §3/§2 para o
significado dos campos). Cobre tudo de CONTRATO §5, incluindo `/api/push/*` e
`/api/notificacoes/*` (etapa 4) e o `GET /health` completo (etapa 5), mais as rotas de compras de
CONTRATO-FASE3 §5 (etapa F3-1) e a geração/envio do lembrete de lista (§2.6, etapa F3-2).

Convenções:

- Toda resposta é JSON. Erro é sempre `{"erro": "mensagem em português"}` com status HTTP
  `400` (validação), `404` (não encontrado), `409` (conflito de estado) ou `500` (erro interno —
  mensagem genérica "erro interno", nunca detalhe do erro real).
- Datas: `"YYYY-MM-DD"` (string). Timestamps: RFC3339 com o offset local (`America/Sao_Paulo`,
  ex. `"2026-09-28T21:00:00-03:00"`).
- Campo ausente num objeto de resposta só acontece quando marcado "omitido se vazio" abaixo;
  os demais aparecem sempre, com `null` quando não há valor (ex. `taxa_30d`, `ocorrencia_aberta`).

## Objeto `item`

Aparece em toda resposta que envolve um item (dentro de `"item": {...}}` ou em listas).

```json
{
  "id": 1,
  "tipo": "habito",
  "titulo": "ler",
  "notas": "",
  "regra": { "tipo": "diario" },
  "data": null,
  "hora": "",
  "horarios": ["21:00"],
  "estado": "ativo",
  "cor": "",
  "ordem": 0,
  "criado_em": "2026-09-28T10:00:00-03:00",
  "atualizado_em": "2026-09-28T10:00:00-03:00",
  "metricas": {
    "sequencia_atual": 3,
    "melhor_sequencia": 5,
    "taxa_30d": 0.8,
    "intervalo": null
  }
}
```

- `regra` só existe para hábito (omitido para compromisso). Formatos (§3.2):
  `{"tipo":"diario"}` · `{"tipo":"dias_semana","dias":[1,3,5]}` (1=segunda..7=domingo) ·
  `{"tipo":"vezes_periodo","vezes":3,"periodo":"semana"}` (`periodo`: `"semana"` ou `"mes"`) ·
  `{"tipo":"intervalo","dias":3}`.
- `data`/`hora` só fazem sentido para compromisso (`null`/`""` quando ausentes). Hábito nunca tem.
- `horarios`: lista de `"HH:MM"` (hábito, ou compromisso com data e sem hora) ou de antecedências
  `"-15m"`/`"-2h"`/`"-1d"` (compromisso com hora — sempre soma um lembrete implícito na própria hora).
- `metricas` só existe para hábito (omitido para compromisso — não tem sequência/taxa).
  - `taxa_30d`: número 0–1, ou `null` quando não há ocorrência fechada ainda ("sem taxa", não 0%).
  - `intervalo`: `{"em_dia": true, "dias": 2}` só para regra `intervalo` (senão `null`);
    `em_dia=true` → faltam `dias` dias; `em_dia=false` → atrasado `dias` dias (`0` = vence hoje).

## Objeto `ocorrencia`

```json
{ "id": 10, "inicio": "2026-09-28", "fim": "2026-09-28", "meta": 1, "status": "aberta", "feitos": 0 }
```

- `status`: `aberta` · `cumprida` · `perdida` · `pulada`.
- `inicio`/`fim`: omitidos só para a ocorrência de compromisso sem data. `fim` também omitido na
  ocorrência aberta de um hábito `intervalo` (vence quando for feito, não tem data marcada).
- `feitos`: quantos registros já caíram no intervalo desta ocorrência — só aparece em
  `GET /api/hoje` (chip "2/3"); omitido nas outras rotas.

## Objeto `registro`

```json
{ "id": 5, "em": "2026-09-28T21:03:00-03:00", "nota": "", "origem": "app" }
```

`origem`: `"app"` (toque na tela) ou `"notificacao"` (ação "Feito" da notificação — etapa 4).

---

## `GET /api/hoje`

O que mostrar na tela Hoje (§6.1).

```json
{
  "data": "2026-09-28",
  "habitos": [
    {
      "item": { "...": "item sem metricas" },
      "ocorrencia": { "id": 10, "inicio": "2026-09-28", "fim": "2026-09-28", "meta": 1, "status": "aberta", "feitos": 0 },
      "intervalo": null
    }
  ],
  "compromissos": [
    {
      "item": { "...": "item sem metricas" },
      "ocorrencia": { "id": 11, "inicio": "2026-09-28", "fim": "2026-09-28", "meta": 1, "status": "aberta" },
      "atrasado": false
    }
  ],
  "sem_data": 2
}
```

- `habitos`: hábitos **ativos** com ocorrência cobrindo hoje, **qualquer status** (o front decide o
  visual — cumprida/perdida/pulada aparecem, não só aberta). `intervalo` só entra (não-null) para
  regra `intervalo`, e só quando a ocorrência aberta já venceu (`inicio <= hoje`) — "em dia" (ainda
  não venceu) não aparece em Hoje.
- `compromissos`: compromissos **ativos** com `data == hoje` ou `data < hoje` (`atrasado: true`),
  mais os **concluídos hoje** (registro criado hoje). Compromisso sem data nunca entra aqui.
- `sem_data`: contagem de compromissos ativos sem `data` (a aba Compromissos os lista).
- `item` dentro de `habitos`/`compromissos` **não** traz `metricas` (use `GET /api/itens/{id}` ou
  `GET /api/itens` para isso).

## `GET /api/itens?tipo=&estado=`

Lista itens (com métricas nos hábitos). `tipo`/`estado` opcionais (`habito`/`compromisso`,
`ativo`/`pausado`/`arquivado`/`concluido`); omitidos = todos.

Resposta: `[ {...item...}, ... ]` (array de `item`, ordenado por `ordem` e depois `id`).

## `POST /api/itens`

Cria um item; valida regra/horários; já gera as ocorrências que valem na hora (a única, se
compromisso; a de hoje/período, se hábito).

Corpo:

```json
{
  "tipo": "habito",
  "titulo": "ler",
  "notas": "",
  "regra": { "tipo": "diario" },
  "horarios": ["21:00"],
  "cor": "",
  "ordem": 0
}
```

Compromisso (sem `regra`; `data`/`hora` opcionais):

```json
{ "tipo": "compromisso", "titulo": "dentista", "data": "2026-10-14", "hora": "15:00", "horarios": ["-15m"] }
```

- `titulo`: obrigatório, 1–120 caracteres (espaços nas pontas são cortados).
- Hábito: `regra` obrigatória (§3.2); não aceita `data`/`hora`.
- Compromisso: não aceita `regra`; `hora` sem `data` é erro 400 ("hora requer data").
- `horarios`: ver formato em cima; validado contra o tipo do item.

Resposta: `201` com o `item` criado (sem `metricas` — hábito recém-criado nunca tem ocorrência
fechada ainda; use `GET /api/itens/{id}` se precisar do objeto zerado).

Erros: `400` (título vazio/longo, tipo desconhecido, regra/horários inválidos, combinação errada
de campos com o tipo).

## `GET /api/itens/{id}`

Detalhe de um item: item + métricas + últimos 30 registros (mais recentes primeiro) + a
ocorrência "aberta" corrente (a que decide o que mostrar agora — `null` se não houver nenhuma
ainda aberta: por exemplo, hábito diário já cumprido hoje).

```json
{
  "item": { "...": "item com metricas" },
  "registros": [ { "id": 5, "em": "2026-09-28T21:03:00-03:00", "nota": "", "origem": "app" } ],
  "ocorrencia_aberta": { "id": 10, "inicio": "2026-09-28", "fim": "2026-09-28", "meta": 1, "status": "aberta" }
}
```

Erros: `404` (id não existe).

## `PATCH /api/itens/{id}`

Edita campos (todos opcionais — só manda o que muda): `titulo`, `notas`, `regra`, `data`, `hora`,
`horarios`, `cor`, `ordem`. **`tipo` nunca muda** — mandar a chave `"tipo"` no corpo (com
qualquer valor) é sempre `400`.

```json
{ "regra": { "tipo": "dias_semana", "dias": [1, 3, 5] } }
```

- Mudar a `regra` (hábito) ou a `data` (compromisso) **não reescreve o passado**: ocorrências já
  fechadas (cumprida/perdida/pulada com `fim` antes de hoje) ficam intocadas; as ainda abertas
  (hoje em diante) são apagadas e refeitas com a regra nova, recalculando o status com os
  registros que já existirem no intervalo de cada uma.
- `data: ""` remove a data de um compromisso (vira tarefa sem data); `hora: ""` remove a hora.

Resposta: `200` com o `item` atualizado (sem `metricas`). Erros: `400` (validação, tipo no
corpo), `404` (id não existe).

## `POST /api/itens/{id}/estado`

Muda o estado do item. Corpo: `{"estado": "pausado"}` (valores: `ativo`, `pausado`, `arquivado`,
`concluido`).

Transições válidas:

| Tipo | De → Para |
|---|---|
| hábito | `ativo` ↔ `pausado`; `ativo`/`pausado` → `arquivado`; `arquivado` → `ativo` |
| compromisso | `ativo` → `concluido` (equivale a `POST /registros` agora); `ativo`/`concluido` → `arquivado`; `arquivado` → `ativo` |

Qualquer outra transição (inclusive `concluido` → `ativo` — "desfaça o registro para reabrir") é
`409`. Pausar/arquivar hábito: as ocorrências ainda abertas com `fim` viram `pulada` (não
`perdida`, para não quebrar a sequência durante a pausa); a aberta de um hábito `intervalo` não
é afetada. Retomar (→ `ativo`): gera a ocorrência de hoje/período se estiver faltando.

Resposta: `200` com o `item` atualizado. Erros: `400` (estado desconhecido), `404`, `409`.

## `GET /api/itens/{id}/calendario?de=&ate=`

Um registro por dia, para o mapa de calor do detalhe do hábito. `de`/`ate` são `"YYYY-MM-DD"`
opcionais — default: últimas 12 semanas até hoje. Intervalo maior que 400 dias é `400`.

```json
[
  { "dia": "2026-09-27", "status": "", "registros": 0 },
  { "dia": "2026-09-28", "status": "cumprida", "registros": 1 }
]
```

`status` é o da ocorrência que cobre aquele dia (`""` se nenhuma cobrir). `registros` é quantos
registros do item caíram naquele dia (pode ser > 1 mesmo com meta 1 — todos contam).

Erros: `404` (item), `400` (`de` depois de `ate`, ou intervalo > 400 dias).

## `POST /api/itens/{id}/registros`

Um toque em "feito". Corpo (tudo opcional):

```json
{ "em": "2026-09-28", "nota": "capítulo 3" }
```

- `em`: RFC3339 (`"2026-09-28T21:00:00-03:00"`), `"YYYY-MM-DD"` (marcação retroativa pelo
  calendário — vira meio-dia local daquele dia) ou omitido (agora). Nunca no futuro (`400`
  "não dá para registrar no futuro").
- A ocorrência coberta pelo dia local de `em` é achada e recalculada; dia sem ocorrência
  (ex.: `dias_semana` num dia não marcado) grava o registro como extra, sem tocar em ocorrência.
- Hábito `intervalo`: fecha a aberta (`fim` = dia do registro) e nasce a próxima em `dia + N`.
- Compromisso: a ocorrência vira `cumprida` e o item vira `concluido`.

Resposta: `201`:

```json
{
  "item": { "...": "item atualizado, com metricas" },
  "registro": { "id": 5, "em": "2026-09-28T21:00:00-03:00", "nota": "capítulo 3", "origem": "app" }
}
```

Erros: `400` (`em` inválido ou no futuro), `404` (item), `409` (item arquivado).

## `PATCH /api/registros/{id}`

Anota o que foi feito num registro (28/09/2026). Corpo `{"nota": "Romanos 8"}` — `nota` obrigatória
no corpo; vazia apaga; aparada; máximo 500 caracteres (400 acima). Não mexe em ocorrência nem em
métrica. Resposta 200: o objeto `registro` (`{id, em, nota?, origem}`). 404 se o registro não existe.

## `DELETE /api/registros/{id}`

Desfaz (hard delete — decisão A6, não é histórico). Recalcula a ocorrência que o registro
cobria; compromisso volta a `ativo`/aberta; hábito `intervalo` tem uma regra própria (§3.3 regra
5): se era o registro que fechou a **última** cumprida, ela reabre e a próxima (sem registros) é
apagada; se era de uma cumprida mais antiga, só o registro sai — o histórico fica.

Resposta: `200` com o `item` atualizado (com métricas). Erros: `404` (registro não existe).

## `POST /api/ocorrencias/{id}/pular` · `POST /api/ocorrencias/{id}/despular`

Pular marca a ocorrência (deve estar `aberta`) como `pulada` — não conta na taxa nem quebra a
sequência, e é desfazível por `despular`, que recalcula o status como se nunca tivesse sido
pulada (considerando os registros já existentes no intervalo). Ambas respondem `200` com o
`item` atualizado (com métricas).

Erros: `404` (ocorrência não existe), `409` (pular algo que não está aberta, ou despular algo que
não está pulada).

## `GET /health`

Sem `/api` — para a dashboard/monitoramento (etapa 5, completo).

```json
{
  "ok": true,
  "versao": "a1b2c3d",
  "tick_em": "2026-09-28T21:00:30-03:00",
  "cert_expira_em": "2026-11-20T10:00:00-03:00",
  "subs": 1
}
```

- `tick_em`: instante do último tick do scheduler (`null` se ele ainda não rodou).
- `cert_expira_em`: validade (`NotAfter`) do certificado TLS carregado.
- `subs`: quantas inscrições push existem (`push_sub`).
- `ok`: `false` se o banco não responde, se `cert_expira_em` é em menos de **7 dias**, ou se
  `tick_em` tem mais de **5 min** (scheduler travado ou processo preso).

---

## Push e notificações (etapa 4; `tipo`/lembrete de lista — fase 3, etapa F3-2)

`notificacao` não tem objeto próprio documentado aqui — o front nunca lista notificações, só
reage a elas (recebe o push, mostra, chama uma destas rotas). O payload do push em si (o que o
`sw.js` recebe em `event.data.json()`) tem dois formatos, discriminados por `tipo`:

**Lembrete de item** (hábito/compromisso, `tipo: "lembrete"`):

```json
{ "notificacao_id": 42, "item_id": 7, "tipo": "lembrete", "title": "Pocket — ler", "body": "toque em Feito, ou adie 30 min" }
```

**Lembrete de lista** (`tipo: "lista"`, CONTRATO-FASE3 §2.6/§5 — etapa F3-2):

```json
{ "notificacao_id": 55, "lista_id": 1, "tipo": "lista", "title": "Compras — Mercado", "body": "7 itens: hortifrúti 3, laticínios 2, +2" }
```

- `lista_id` no lugar de `item_id`; `tipo: "lista"` é o sinal para o `sw.js` **não mostrar as
  ações "Feito"/"Adiar"** (não fazem sentido para lista, §2.6) e abrir `/?lista=<id>` no toque
  (front, F3-3) em vez de abrir o item.
- `body`: contagem de pendentes por seção **no momento do envio** (não da geração) — até 3
  seções com mais pendentes, nome em minúsculas, o restante (se houver mais de 3 seções com
  pendentes) somado num `"+N"` final; `"1 item: ..."` no singular quando o total é 1.
- Lista arquivada ou com **zero pendentes no momento do envio** não gera push nenhum — a
  notificação vira `cancelada` sem tentar mandar (§2.6: "zero → cancelada, não incomoda com
  lista vazia").
- Regra do lembrete de lista: só `"diario"` ou `"dias_semana"` (mesmo formato de regra de hábito,
  §3.2 da fase 1) — `vezes_periodo`/`intervalo` não fazem sentido aqui e o `PATCH
  /api/listas/{id}` os recusa. `horarios`: lista de `"HH:MM"`, pelo menos um. Ver o objeto
  `lista` em "Compras" para o formato gravado (`lembrete.regra`/`lembrete.horarios`).
- `POST /api/notificacoes/{id}/feito` e `.../adiar` numa notificação de lista devolvem `409`
  (`"lembrete de lista não tem essa ação"`); `recebida`/`abrir` funcionam normalmente.

### `GET /api/push/vapid`

Chave pública VAPID, para `PushManager.subscribe({ applicationServerKey: ... })`.

```json
{ "chave_publica": "BN4..." }
```

### `POST /api/push/subscribe`

Upsert por `endpoint` (§5: "o app chama em toda abertura, se a permissão estiver concedida" —
cobre troca de endpoint e a migração do spike). Corpo = `PushSubscription.toJSON()` do browser:

```json
{ "endpoint": "https://fcm.googleapis.com/...", "keys": { "p256dh": "...", "auth": "..." } }
```

Resposta: `200 {"ok": true}`. Erros: `400` (endpoint/p256dh/auth ausente).

### `POST /api/push/teste`

Push imediato "teste" para todas as inscrições (tela de Ajustes). Sem corpo.

Resposta: `200 {"enviados": 2}` — quantas inscrições receberam com sucesso (uma inscrição morta,
404/410, é removida e não conta; não conta como erro da rota).

### `POST /api/notificacoes/{id}/{acao}`

Chamada pelo `sw.js`, nunca pela tela (§3.5). `acao` é um dos quatro valores abaixo — qualquer
outro é `400`.

| `acao` | Quando | Efeito |
|---|---|---|
| `recebida` | `push` chegou no service worker | grava `recebida_em` (tela de Ajustes mostra "último recebido") |
| `abrir` | usuário tocou no corpo da notificação | grava a ação; quem abre o app é o `sw.js` (`clients.openWindow`) |
| `feito` | usuário tocou "Feito" na notificação (decisão A5: sem abrir o app) | `POST /registros` equivalente, `origem: "notificacao"` |
| `adiar` | usuário tocou "Adiar 30 min" | cria uma nova notificação `agendada_para = agora + 30min` |

Sem corpo. Resposta: `200 {"ok": true}`. Erros: `400` (`acao` desconhecida), `404` (notificação
não existe) — os mesmos erros de `POST /registros` propagam se `acao=feito` falhar (ex.: item
arquivado → `409`).

---

## Compras (fase 3, CONTRATO-FASE3.md §2/§5 — etapas F3-1/F3-2)

Lembrete de lista (§2.6: geração no job diário, envio com resumo, cancelamento com lista vazia,
payload `tipo`/`lista_id`) é a etapa **F3-2** — documentado na seção "Push e notificações" acima
(o payload) e no campo `lembrete` do objeto `lista` abaixo (o que se grava).

### Objeto `lista`

```json
{
  "id": 1,
  "nome": "Mercado",
  "ordem": 0,
  "arquivada": false,
  "lembrete": { "regra": { "tipo": "diario" }, "horarios": ["09:00"] },
  "criado_em": "2026-09-26T00:00:00-03:00",
  "pendentes": 7
}
```

- `lembrete`: `null` quando a lista não tem lembrete. `regra` só aceita `{"tipo":"diario"}` ou
  `{"tipo":"dias_semana","dias":[...]}` (mesmo formato de regra de hábito, §3.2 da fase 1) —
  `vezes_periodo`/`intervalo` são recusados (`400`). `horarios`: lista de `"HH:MM"`, pelo menos um.
- `pendentes`: só aparece em `GET /api/listas` (quantos itens não riscados a lista tem); omitido
  nas demais respostas.

### Objeto `secao`

```json
{ "id": 3, "lista_id": 1, "nome": "Frios e laticínios", "ordem": 3 }
```

### Objeto `item` (de compra)

```json
{
  "id": 10, "lista_id": 1, "secao_id": 3,
  "nome": "Leite", "nome_chave": "leite",
  "quantidade": "2", "nota": "",
  "comprado": false, "comprado_em": "",
  "origem": "manual", "receita_id": null,
  "criado_em": "2026-09-28T10:00:00-03:00"
}
```

- `nome_chave`: normalizado pelo servidor (§2.5) — só leitura, nunca enviado pelo front.
- `origem`: `"manual"` \| `"frequente"` (veio de um chip "de novo?") \| `"receita"` (fase 4,
  `receita_id` sempre `null` até lá).
- `quantidade`/`nota`/`comprado_em` vêm como `""` (nunca omitidos na leitura direta do banco, mas
  o JSON os omite quando vazios — trate ausência como `""`).

### `GET /api/listas`

Listas não arquivadas, com `pendentes` e `lembrete`. `200` → `[lista, ...]`.

### `POST /api/listas`

`{"nome": "Farmácia"}` → cria com uma seção só, `"Geral"`. `201` → `lista`. Erros: `400` (nome
vazio ou > 40 caracteres).

### `PATCH /api/listas/{id}`

`{nome?, ordem?, lembrete?}` — todo campo opcional. `lembrete: null` remove o lembrete existente;
`lembrete` ausente não mexe; `lembrete: {...}` valida e substitui (ver Objeto `lista` acima).
`200` → `lista`. Erros: `400`, `404`.

### `POST /api/listas/{id}/estado`

`{"arquivada": true}` → arquiva/desarquiva (§2.1: arquivar esconde, histórico e itens ficam).
`200` → `lista`. Erros: `404`.

### `GET /api/listas/{id}`

`200` → `{"lista": {...}, "secoes": [...], "itens": [...]}`. `secoes` ordenadas por `ordem`;
`itens` na ordem de §2.3 regra 4 (seção → pendentes por criação → riscados por quando foram
riscados). Erros: `404`.

### `POST /api/listas/{id}/itens`

`{"nome": "leite", "quantidade": "2", "secao_id": null, "origem": "manual"}` — `quantidade`,
`secao_id` e `origem` são opcionais. Regra 1 de §2.3 (dedupe por `nome_chave` entre pendentes):
se já existe um pendente com o mesmo `nome_chave`, devolve **esse** item (`200`) — se veio
`quantidade`, ela substitui a do existente; senão cria um novo (`201`). Sem `secao_id`, a seção é
automática (§2.3 regra 2: última seção em que esse `nome_chave` foi comprado nesta lista; sem
histórico, `"Outros"` se existir, senão a primeira seção). Erros: `400` (nome vazio/> 60
caracteres, origem inválida, seção de outra lista), `404` (lista não existe).

### `PATCH /api/compras/{id}`

`{nome?, quantidade?, secao_id?, nota?}`. Editar o nome para um `nome_chave` que já é de outro
pendente na mesma lista é `409`. `200` → `item`. Erros: `400`, `404`, `409`.

### `POST /api/compras/{id}/comprado` · `POST /api/compras/{id}/desmarcar`

Riscar/desriscar — idempotentes (repetir não muda nada nem conta 2× no histórico, §2.4). Riscar
faz upsert em `compra_historico` (`vezes += 1`); desriscar **no mesmo dia** desfaz o `+1` (outro
dia: o histórico fica, foi uma compra de verdade). `200` → `item`. Erros: `404`.

### `DELETE /api/compras/{id}`

Remove o item (hard delete). `204` sem corpo. Erros: `404`.

### `POST /api/listas/{id}/limpar-comprados`

Apaga (hard delete — o histórico guarda, §2.3 regra 3) os itens riscados da lista. `200` →
`{"removidos": 3}`. Erros: `404`.

### `GET /api/listas/{id}/frequentes?q=`

Histórico da lista menos o que já está pendente, ordenado por
`vezes / (1 + dias desde a última compra / 30)` (§2.4), top 20. Com `q`, filtra por prefixo de
`nome_chave` (autocompletar). `200` → lista de:

```json
{ "nome": "Leite", "nome_chave": "leite", "secao_id": 3, "ultima_quantidade": "2", "vezes": 5, "ultima_em": "2026-09-20T11:00:00-03:00" }
```

### `POST /api/listas/{id}/secoes` · `PATCH /api/secoes/{id}` · `DELETE /api/secoes/{id}`

Criar (`{"nome": "..."}`, `201` → `secao`), renomear/reordenar (`{nome?, ordem?}`, `200` →
`secao`) e apagar seções (§2.2). Apagar move os itens da seção para a primeira seção restante da
lista; apagar a **última** seção de uma lista é `409`. `DELETE` bem-sucedido: `204` sem corpo.
Erros: `400`, `404`, `409`.

### `GET /api/hoje` (campo novo)

Ganha `"compras": [{"lista_id": 1, "nome": "Mercado", "pendentes": 7}, ...]` — só listas não
arquivadas com `pendentes > 0` (§2.7).

---

## Captura por voz e texto (fase 1.5, CONTRATO-FASE15.md §3)

Falar ou escrever uma frase solta e receber a ação pronta para um toque de confirmação. A captura
não tem rota de escrita própria — confirmar sempre chama `POST /api/itens`, `POST /api/itens/{id}/
registros` ou `POST /api/listas/{id}/itens`, como de costume.

### `GET /api/captura`

`200` → `{"ativa": true}` — `false` quando `GROQ_API_KEY` está vazia no servidor (o front esconde
a barra de captura).

### `POST /api/captura`

Corpo, um de dois:

- `multipart/form-data` com o campo `audio` (≤ 2 MB; `audio/webm`, `audio/ogg` ou `audio/mp4`,
  parâmetros como `;codecs=opus` são ignorados) — o servidor transcreve e interpreta.
- JSON `{"texto": "..."}` (1–500 caracteres) — só interpreta (é o que "reinterpretar"/"Tem mais"
  do front usa).

`200`:

```json
{
  "captura_id": 41,
  "texto": "fiz o devocional, Romanos 8",
  "acao": "registrar",
  "item": null,
  "registro": { "item_id": 3, "titulo": "Devocional", "tipo": "habito", "em": null, "nota": "Romanos 8" },
  "compras": null,
  "resto": null,
  "avisos": []
}
```

- `acao`: `"criar_item"` | `"registrar"` | `"compras"` | `"desconhecido"`. Só o campo
  correspondente (`item`/`registro`/`compras`) vem preenchido; os outros dois são `null`.
- `item` (em `criar_item`) tem o formato exato de `NovoItemPayload` (`POST /api/itens`) — já
  validado (regra, horários, data). Campo que o modelo mandou errado é removido e vira um texto em
  `avisos` (ex. `"não entendi a frequência"`); o formulário deixa corrigir o resto.
- `registro` (em `registrar`): `item_id`/`titulo`/`tipo` apontam para um hábito ou compromisso que
  já existe no contexto (nunca inventado); `em` é `null` (agora) ou `"YYYY-MM-DDT12:00:00-03:00"`
  quando a frase citou um dia passado; `nota` é o que veio depois de "fiz X".
- `compras` (em `compras`): `{"lista_id", "lista_nome", "itens": [{"nome", "quantidade"}]}` — até
  20 itens.
- `resto`: quando a frase tinha mais de uma coisa ("dentista amanhã **e comprar leite**"), o
  pedaço não resolvido — a tela mostra "Tem mais: *comprar leite* → interpretar" (reenviar como
  `POST /api/captura` `{"texto": resto}`).
- `avisos`: frases curtas em português para mostrar no cartão (não impedem a proposta).

Erros: `400` (corpo inválido, áudio > 2 MB ou tipo não suportado, texto vazio/> 500 caracteres) ·
`503` (`"captura desligada"`) · `429` (`"limite do Groq atingido — espere um minuto ou use o formulário"`) ·
`502` (`"não consegui entender agora — tente de novo ou use o formulário"`).
Em `429`/`502` depois de uma transcrição bem-sucedida, o corpo traz também `"texto"` (o que foi
entendido da fala) — o front o põe no campo e oferece "salvar como tarefa" (V3).

### `POST /api/captura/{id}/desfecho`

`{"desfecho": "confirmada" | "ajustada" | "descartada"}` — o front avisa o que o usuário fez com a
proposta; sem efeito colateral além da linha em `captura` (matéria-prima para ajustar o prompt).
`200` → `{"ok": true}`. Erros: `400` (desfecho inválido), `404` (captura não existe).
