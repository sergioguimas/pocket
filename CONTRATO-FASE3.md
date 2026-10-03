# Pocket — contrato da fase 3 (listas de compras)

**Status:** TO-BE, **fechado em 26/09/2026** — decisões de produto do Sergio (várias listas · seções
fixas editáveis · lembrete em dia/hora fixos) e F1–F5 de §10 confirmadas. Build por §11, no mesmo esquema da fase 1 (Opus revisa, Sonnet escreve, branch `bot/pocket-f3`).
**Base:** fase 1 no ar ([CONTRATO.md](CONTRATO.md), [API.md](API.md), decisões em [README.md](OPERACAO.md)).
A fase 1.5 (captura por voz) e a 2 (evolução + cobrança) seguem planejadas e **não dependem** desta.
**Papel:** aba **Compras** com uma ou mais listas (Mercado, Farmácia…), itens agrupados por seção,
riscar com um toque dentro do mercado — inclusive com sinal ruim — e um lembrete com o resumo do que
falta comprar. Prepara o terreno para as receitas da fase 4 mandarem ingredientes para a lista.

---

## 1. Por que aba própria (decisão de 26/09)

Item de compra **volta** (leite toda semana), tem quantidade e seção, e é usado em pé no mercado;
compromisso concluído some para sempre e é consultado por data. Misturar poluiria o "sem data" de Hoje
e forçaria gambiarra no domínio. Compromisso sem data continua servindo para tarefa pontual
("comprar presente"). Receitas (fase 4) entram **dentro** desta aba, num seletor `Lista | Receitas`.

## 2. Domínio

### 2.1 Lista
`nome` (1–40), `ordem`, `arquivada`. A migração cria **"Mercado"** com as seções-padrão (§2.2).
Lista nova nasce com uma seção só, "Geral". Arquivar esconde (histórico e itens ficam).

### 2.2 Seção
Pertence a uma lista, tem `nome` e `ordem`; o usuário renomeia, reordena, cria e apaga.
Seções-padrão do Mercado, nesta ordem (a de um corredor típico):
Hortifrúti · Padaria · Açougue e peixaria · Frios e laticínios · Mercearia · Bebidas · Congelados ·
Limpeza · Higiene · Outros.
Toda lista tem ≥ 1 seção. Apagar uma seção move os itens dela para a primeira seção restante.

### 2.3 Item de compra
`nome` (1–60), `quantidade` (texto livre opcional: "2 kg", "3", "1 cx"), `secao_id`, `nota`,
`comprado` + `comprado_em`, `origem` (`manual` | `frequente` | `receita`) e `receita_id` (NULL até a fase 4).

Regras:
1. **Sem duplicata pendente:** adicionar um nome que já está pendente na lista (comparação por
   `nome_chave`, §2.5) não cria outro item — devolve o existente; se veio `quantidade`, ela substitui.
2. **Seção automática:** sem `secao_id`, usa a última seção em que esse `nome_chave` foi comprado nesta
   lista (histórico, §2.4); sem histórico, a seção "Outros" se existir, senão a primeira.
3. **Riscar/desriscar** alterna `comprado`. Riscado continua visível (fim da seção, esmaecido) até
   **"Limpar comprados"**, que apaga de vez os riscados daquela lista (hard delete — o histórico guarda).
4. Ordem na tela: seção (`ordem`) → pendentes por `criado_em` → riscados por `comprado_em`.

### 2.4 Histórico e frequentes
Ao riscar, faz upsert em `compra_historico` (`lista_id`, `nome_chave`): `nome` (última grafia),
`secao_id`, `ultima_quantidade`, `vezes += 1`, `ultima_em`. Desriscar no mesmo dia desfaz o `+1`.
**Frequentes** = histórico da lista **menos** o que já está pendente, ordenado por
`vezes` com peso de recência (score = `vezes / (1 + dias desde ultima_em / 30)`), top 20. Servem para
os chips "adicionar de novo" e para o autocompletar ao digitar (prefixo em `nome_chave`).

### 2.5 `nome_chave`
Minúsculas, sem acento, espaços colapsados, sem plural simples no fim (`s` após vogal: "tomates" →
"tomate"; não mexer em "arroz", "pães" fica "paes"). Função única em `domain`, com testes. Não precisa
ser perfeita — erro vira no máximo um item duplicado ou um frequente a mais.

### 2.6 Lembrete da lista
Cada lista pode ter **um** lembrete: regra `diario` ou `dias_semana` (as mesmas da fase 1 — `vezes_periodo`
e `intervalo` não fazem sentido aqui e são recusadas) + `horarios` `HH:MM`.
- O job diário gera as notificações do dia (tabela `notificacao`, com `lista_id`, §4).
- **No envio** (não na geração), o scheduler conta os pendentes: **zero → `cancelada`**, não incomoda com
  lista vazia. Senão, título `"Compras — <lista>"`, corpo `"7 itens: hortifrúti 3, laticínios 2, +2"`
  (até 3 seções com mais itens, o resto somado).
- Mesmas regras de agenda da fase 1: persistida, tick de 30 s, expira > 30 min.
- Sem ações de Feito/Adiar: tocar abre o app **na lista**.

### 2.7 Hoje
`GET /api/hoje` ganha `compras: [{lista_id, nome, pendentes}]` (só listas não arquivadas com pendentes > 0).
A tela Hoje mostra uma linha por lista ("Mercado · 7 itens") que abre a lista.

## 3. Uso no mercado (sinal ruim)

O mercado é o lugar onde o 4G mais falha, e aqui a fase 1 não basta ("fila offline é backlog"). Para
**esta aba**:
- A última versão de cada lista aberta fica guardada no aparelho (IndexedDB) e é mostrada se a rede cair,
  com a faixa "sem conexão — mudanças serão enviadas".
- **Riscar/desriscar é otimista:** a tela muda na hora; a operação entra numa fila local (IndexedDB) e é
  reenviada em ordem quando a rede volta (`online`, abertura do app, a cada 20 s enquanto houver fila).
  Operações idempotentes por natureza (`comprado` / `desmarcar` com o id), então reenviar não estraga.
- Adicionar item offline também entra na fila (id provisório negativo na tela até o servidor responder).
- Editar/apagar/limpar/seções **exigem rede** (botões desabilitados offline) — menos casos de conflito.
- Conflito: último a chegar vence. Um usuário, um aparelho: aceitável.

## 4. Schema — `store/migrations/0002_compras.sql`

```sql
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
```

Seed na mesma migração: lista "Mercado" (ordem 0) + as 10 seções de §2.2 (a migração roda uma vez só —
schema_version — então o seed não duplica). **A migração roda sobre o banco em produção**: testar
aplicando-a numa cópia de `data/pocket.db` do aparelho antes do deploy, e o backup diário garante volta.
Verificar que as notificações pendentes existentes (ex.: "Churrasco UMP") sobrevivem com o mesmo id.

## 5. API (acrescenta a [API.md](API.md); erro = `{"erro": ...}` como na fase 1)

| Método e rota | Faz |
|---|---|
| `GET /api/listas` | listas não arquivadas com `pendentes` e `lembrete` |
| `POST /api/listas` | `{nome}` → cria com seção "Geral" |
| `PATCH /api/listas/{id}` | `{nome?, ordem?, lembrete?}` (`lembrete: null` remove; valida §2.6) |
| `POST /api/listas/{id}/estado` | `{arquivada: bool}` |
| `GET /api/listas/{id}` | lista + seções (ordenadas) + itens (ordem de §2.3 regra 4) |
| `POST /api/listas/{id}/itens` | `{nome, quantidade?, secao_id?, origem?}` → 201 novo ou 200 existente (regra 1) |
| `PATCH /api/compras/{id}` | `{nome?, quantidade?, secao_id?, nota?}` |
| `POST /api/compras/{id}/comprado` · `/desmarcar` | riscar / desriscar (idempotentes: repetir não muda nada nem conta 2× no histórico) |
| `DELETE /api/compras/{id}` | remove item |
| `POST /api/listas/{id}/limpar-comprados` | apaga os riscados; devolve quantos |
| `GET /api/listas/{id}/frequentes?q=` | §2.4; com `q`, filtra por prefixo de `nome_chave` |
| `POST /api/listas/{id}/secoes` · `PATCH /api/secoes/{id}` · `DELETE /api/secoes/{id}` | §2.2 (apagar a última → 409) |
| `GET /api/hoje` | + `compras` (§2.7) |

Payload do push ganha `tipo` (`lembrete` \| `lista`) e `lista_id`; o `sw.js` não mostra ações quando
`tipo = lista` e abre `/?lista=<id>` no toque.

## 6. Telas

1. **Barra inferior com 4 abas:** Hoje · Hábitos · Compromissos · **Compras**. O "+" flutuante, na aba
   Compras, foca o campo de adicionar (não abre a folha modal).
2. **Compras:** seletor de lista no topo (chips com o nome e a contagem; "+ lista"). Campo fixo
   "adicionar item" com autocompletar dos frequentes e quantidade opcional ("leite 2" ou "2 leite" →
   nome "leite", quantidade "2" — parser simples no front, número isolado no início ou fim).
   Abaixo, chips "de novo?" com os frequentes que não estão na lista (toque = adiciona).
   Itens agrupados por seção, toque na linha inteira risca (alvo grande, uma mão, sem confirmar);
   pressionar e segurar (ou ícone ⋯) abre editar/mover de seção/apagar. Rodapé: "Limpar comprados (N)".
3. **Ajustes da lista** (ícone no topo da lista): renomear, lembrete (os mesmos seletores de frequência
   do editor de hábito, só `Todo dia`/`Em dias da semana`, + horários), seções (renomear, reordenar com
   setas ↑↓, criar, apagar), arquivar.
4. **Hoje:** linha "Mercado · 7 itens" por lista com pendentes.
5. Faixa offline e estados vazio/erro como no resto do app.

## 7. Operação

Nada novo: mesmo serviço, mesma porta, mesmo banco (migração 0002), mesmo backup. Deploy pelo hook.

## 8. Critérios de aceite (no Poco)

| # | Critério |
|---|---|
| K1 | A aba Compras abre com a lista "Mercado" e as 10 seções; criar a lista "Farmácia" nasce com "Geral" |
| K2 | Adicionar "leite 2", "Tomates", "tomate" → leite com quantidade 2; tomate não duplica |
| K3 | Riscar move o item para o fim da seção, esmaecido; desriscar volta; "Limpar comprados" some com os riscados |
| K4 | Depois de comprar leite uma vez, "leite" aparece em "de novo?" e, readicionado, cai em Frios e laticínios sozinho |
| K5 | Modo avião dentro da lista: a lista continua aparecendo, riscar funciona na hora, e ao voltar a rede o servidor fica igual à tela |
| K6 | Lembrete da lista (sábado 9h, ou daqui a 3 min para testar) chega com o resumo por seção; tocar abre a lista |
| K7 | Com a lista vazia, o lembrete **não** chega (fica `cancelada`) |
| K8 | Hoje mostra "Mercado · N itens" e leva à lista |
| K9 | A migração 0002 roda sobre o banco real sem perder itens, ocorrências, registros nem notificações pendentes da fase 1 |
| K10 | Testes Go: `nome_chave`, dedupe, seção automática, score de frequentes, idempotência de riscar, migração sobre banco da fase 1, lembrete com lista vazia |

## 9. Fora do escopo

Receitas (fase 4: `receita_id`, `origem = receita` e o seletor `Lista | Receitas` já têm lugar);
compartilhar lista com outra pessoa; preço e total gasto (isso é do Smaug); ordenar seções pelo
mapa de um mercado específico; foto do item; código de barras.

## 10. Decisões confirmadas (26/09/2026)

| # | Decisão | Por quê | Alternativa descartada |
|---|---|---|---|
| F1 | Riscados ficam visíveis até "Limpar comprados" (manual) | no mercado você quer ver o que já pegou | somem sozinhos após X horas |
| F2 | **Fila offline só para riscar/desriscar/adicionar** nesta aba | mercado é onde o sinal falha; editar/seções offline trariam conflito | nenhum offline (como a fase 1) ou tudo offline |
| F3 | Lembrete da lista só `diario`/`dias_semana` | "3× por semana" não faz sentido para lista | aceitar os 4 formatos |
| F4 | Quantidade é texto livre | "2 kg", "1 cx", "3" — não vale modelar unidade | número + unidade separados |
| F5 | Toque risca sem confirmação; editar por pressionar e segurar | velocidade com uma mão no mercado | toque abre o item, riscar pelo círculo |

## 11. Build

Branch `bot/pocket-f3`, uma etapa por vez, mesmo cabeçalho comum do [PLANO-DE-BUILD.md](PLANO-DE-BUILD.md) §4.

| # | Entrega | Prova |
|---|---|---|
| F3-1 | Migração 0002 (+ seed), `domain` (nome_chave, score de frequentes; o parser "leite 2" é do front), store, app, API de §5 exceto lembrete | testes de K10 (menos lembrete); migração aplicada sobre um banco da fase 1 com dados |
| F3-2 | Lembrete da lista: geração no job diário, envio com resumo e cancelamento com lista vazia, payload com `tipo`/`lista_id` | testes com relógio e pusher falsos (K6, K7) |
| F3-3 (PC) | Front: 4ª aba, lista, ajustes da lista, Hoje, fila offline em IndexedDB, `sw.js` para `tipo = lista` | `npm run build`; teste no navegador com o backend local, incluindo "offline" do DevTools |
| F3-4 | Ensaio da migração numa **cópia** do banco do aparelho, README/API.md, checklist K1–K10 | migração ok na cópia; depois merge + deploy |
