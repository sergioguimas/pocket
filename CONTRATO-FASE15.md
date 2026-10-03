# Pocket — contrato da fase 1.5 (captura rápida por voz e texto)

**Status:** TO-BE, **fechado em 28/09/2026** — V1–V6 (§9) confirmadas pelo Sergio. Build por §10.
**Base:** fases 1 e 3 no ar ([CONTRATO.md](CONTRATO.md), [CONTRATO-FASE3.md](CONTRATO-FASE3.md), [API.md](API.md)).
Decisão de origem: A4 do CONTRATO.md (o LLM só preenche o formulário, nada é gravado sem o toque do usuário).
**Papel:** falar ou escrever uma frase solta — "dentista dia 14 às 15h", "fiz o devocional, Romanos 8",
"comprar leite, pão e 2 kg de arroz" — e o Pocket devolve a ação pronta para um toque de confirmação.

---

## 1. Fronteiras

- **O LLM só propõe.** A captura não tem rota de escrita própria: a confirmação usa as rotas que já
  existem (`POST /api/itens`, `POST /api/itens/{id}/registros`, `POST /api/listas/{id}/itens`).
  Garantido por construção, não por disciplina.
- **A interpretação mora no backend**, não no front: o front manda áudio ou texto e recebe uma proposta
  validada. Qualquer outra porta de entrada futura reaproveita a mesma rota.
- **Sem rede, sem captura.** O botão fica desabilitado offline (a fila offline das Compras não se aplica).
- **Fora:** perguntas ("o que tenho hoje?"), conversa de vários turnos, resposta falada, valor numérico
  (fase 2 — "li até a página 142" vira **nota** agora), receitas (fase 4).

## 2. Ações que a captura entende

| `acao` | Exemplo | O que a tela mostra | Confirmar chama |
|---|---|---|---|
| `criar_item` | "dentista dia 14 às 15h, me lembra um dia antes" · "ler 20 min todo dia às 21h" · "academia 3 vezes por semana" | o **EditorSheet já preenchido** (tipo, título, regra, data, hora, horários, antecedências) | `POST /api/itens` (o Salvar de sempre) |
| `registrar` | "fiz o devocional, Romanos 8" · "ontem fui na academia" · "já fui no dentista" | cartão "Registrar **Devocional** hoje?" com a nota editável | `POST /api/itens/{id}/registros` `{em?, nota?}` — em compromisso, isso é concluir (API.md) |
| `compras` | "comprar leite, pão e 2 kg de arroz" · "na farmácia: dipirona" | cartão com os itens (desmarcáveis, nome/quantidade editáveis) e a lista de destino | `POST /api/listas/{id}/itens` por item (a regra de duplicata da fase 3 vale) |
| `desconhecido` | "hmm deixa eu ver" · transcrição vazia | o texto + **"Salvar como tarefa"** (V3) + "Abrir formulário" | `POST /api/itens` compromisso sem data, título = texto |

Regras de interpretação (vão no prompt e são **revalidadas em Go**):
1. **Datas relativas** resolvidas pelo LLM com o contexto do §4 ("sexta", "dia 14", "amanhã", "semana que
   vem"). "Dia 14" já passado neste mês = mês que vem.
2. **Hábito × compromisso:** frequência ("todo dia", "3× por semana", "a cada 3 dias", "seg e qua") = hábito;
   data única ou nenhuma = compromisso.
3. **`registrar` só aponta item que existe:** o LLM recebe a lista de hábitos ativos e compromissos abertos
   (id + título) e escolhe um `item_id`. Id fora da lista → o Go descarta e a ação vira `criar_item` com o
   título falado.
4. **Dia do registro:** "ontem", "sábado" → `em` naquele dia às 12:00 local; omitido = agora. Futuro → recusado
   (vira aviso).
5. **Nota:** o que vier depois do "fiz X" vira `nota` (≤ 500, mesma regra do PATCH de 28/09).
6. **Compras:** lista citada pelo nome ("na farmácia") ou a primeira não arquivada; quantidade no mesmo
   formato livre da fase 3 ("2 kg", "3"). Máximo 20 itens.
7. **Uma ação por captura** (V1). Se a frase tiver mais de uma coisa, o LLM devolve a primeira e o resto
   do texto em `resto`; a tela mostra "Tem mais: *comprar leite* → interpretar".

## 3. API

### `GET /api/captura`
`{"ativa": true}` — `false` quando `GROQ_API_KEY` está vazia. O front esconde a barra de captura.

### `POST /api/captura`
Corpo, um de dois:
- `multipart/form-data` com `audio` (≤ 2 MB, ≤ 60 s; `audio/webm`, `audio/ogg`, `audio/mp4`) — o Go transcreve
  e interpreta.
- JSON `{"texto": "..."}` (1–500 caracteres) — só interpreta. É o que o "reinterpretar"/"Tem mais" usa.

Resposta `200`:
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
- `item` (em `criar_item`) tem o formato exato de `NovoItemPayload` — já passou por `domain.Regra`, horários
  e datas. Campo inválido é **removido** e vira aviso ("não entendi a frequência"); o formulário deixa corrigir.
- `compras`: `{"lista_id", "lista_nome", "itens": [{"nome", "quantidade"}]}`.
- `avisos`: frases curtas em português para mostrar no cartão.

Erros: `400` corpo inválido/áudio grande/texto vazio · `503` captura desligada · `429` limite do Groq
("limite do Groq atingido — espere um minuto ou use o formulário") · `502` Groq falhou depois das tentativas
("não consegui entender agora — tente de novo ou use o formulário").

### `POST /api/captura/{id}/desfecho`
`{"desfecho": "confirmada" | "ajustada" | "descartada"}` — o front avisa o que o usuário fez com a proposta
(V5). Sem efeito colateral além da linha em `captura`. Falha aqui é ignorada pelo front.

## 4. Backend

Pacote novo `internal/captura`:

| Peça | Faz |
|---|---|
| `Transcritor` (interface) + `groqTranscritor` | `whisper-large-v3-turbo`, `language=pt`, multipart cru — **cópia adaptada** de `smaug/internal/transcricao/groq.go` (módulos Go separados, não dá para importar) |
| `Interpretador` (interface) + `groqInterpretador` | chat completions OpenAI-compatível do Groq, `response_format` `json_schema` com `strict: true`, `reasoning_effort: "low"`, `temperature` 0 |
| `Fake` de ambos | para os testes de `app`/`httpapi` sem rede |
| `Normalizar` | JSON do modelo → resposta do §3: valida com `domain`, confere ids contra o contexto, remove o inválido, gera avisos. **Todo o "não confiar no LLM" fica aqui**, testado com JSON malformado/ids falsos/datas absurdas |

**Contexto enviado ao LLM** a cada chamada (montado por `app.Capturar` a partir do banco):
data e hora locais com dia da semana, fuso, hábitos ativos `[{id, titulo, regra}]`, compromissos abertos
`[{id, titulo, data}]` (até 50, os mais próximos), listas de compras não arquivadas `[{id, nome}]`.
Nada de registros, notas nem histórico.

**Tentativas:** o usuário está esperando com o celular na mão — 1 nova tentativa em 429/5xx/rede,
depois erro. Prazos: transcrição 20 s, interpretação 15 s. Meta: **≤ 5 s** do fim da gravação até o cartão.

**Log:** latência de cada etapa, modelo, tokens, `acao`. **Nunca** o texto nem o áudio no log.

## 5. Banco — `0003_captura.sql` (V5)

```sql
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
```
O áudio **nunca** é guardado. Poda: o job diário mantém as 500 mais recentes.

## 6. Front

1. **Barra de captura no topo de Hoje:** campo "Fale ou escreva…" + botão de microfone. Escondida se
   `GET /api/captura` → `ativa: false`; desabilitada offline.
2. **Gravação:** toque para começar, toque para parar (não segurar), para sozinha em 60 s. Mostra tempo e
   nível. Permissão negada → mensagem com o caminho para liberar.
3. **"Entendendo…"** enquanto o `POST` roda. Depois, conforme §2: EditorSheet preenchido, cartão de
   registrar, cartão de compras ou cartão de desconhecido. Todos mostram "Você disse: *…*" com **editar
   texto → reinterpretar**.
4. `EditorSheet` ganha a prop `inicial?: NovoItemPayload` (cria, mas já preenchido). Nada mais muda nele.
5. **Atalho no ícone** (V6): `shortcuts` no manifest — segurar o ícone do Pocket → "Falar com o Pocket"
   abre `/?captura=voz`, que já começa a gravar (o Chrome pode exigir um toque antes de liberar o microfone;
   se exigir, abre com o botão em destaque).
6. Depois de confirmar: toast "Criado: *Dentista*" / "Registrado em *Devocional*" / "3 itens em *Mercado*",
   e Hoje/Compras recarregam.

## 7. Configuração (`.env`)

| Variável | Default | |
|---|---|---|
| `GROQ_API_KEY` | vazia | vazia = captura desligada (V4: a mesma do Smaug) |
| `POCKET_CAPTURA_MODELO` | `openai/gpt-oss-120b` | `openai/gpt-oss-20b` é o plano B se a latência pesar |
| `POCKET_TRANSCRICAO_MODELO` | `whisper-large-v3-turbo` | |
| `POCKET_GROQ_URL` | `https://api.groq.com/openai/v1` | só no dev: aponta para um Groq falso local (adicionada na revisão) |

Tier gratuito do Groq (26/09): gpt-oss-120b 1.000 req/dia; whisper 2.000 req/dia. O limite é por
**organização**, não por chave — Smaug e Pocket dividem a cota de qualquer jeito.

## 8. Aceite (marcar no Poco)

| # | Critério |
|---|---|
| V1 | Voz: "dentista dia 14 às 15h, me lembra um dia antes" → formulário com compromisso, data certa, 15:00 e antecedência `-1d` |
| V2 | Voz: "ler 20 minutos todo dia às 21h" → hábito diário com lembrete 21:00 |
| V3 | Voz: "academia 3 vezes por semana" → hábito `vezes_periodo` 3/semana |
| V4 | Voz: "fiz o devocional, Romanos 8" → cartão; confirmar cria registro com a nota, visível no detalhe |
| V5 | Voz: "ontem fui na academia" → registro com dia de ontem |
| V6 | Voz: "comprar leite, pão e 2 kg de arroz" → 3 itens, arroz com "2 kg", na lista Mercado |
| V7 | Voz: "dentista amanhã e comprar leite" → cria o dentista e mostra "Tem mais"; tocar interpreta o leite |
| V8 | Frase sem sentido → "Salvar como tarefa" cria compromisso sem data com o texto |
| V9 | Do fim da fala ao cartão ≤ 5 s na maioria das vezes (log de latência) |
| V10 | Sem rede: microfone desabilitado com aviso; com `GROQ_API_KEY` vazia a barra some |
| V11 | Segurar o ícone → "Falar com o Pocket" abre gravando (ou com o botão em destaque) |
| V12 | Nada é gravado se o cartão for fechado sem confirmar (conferir no banco) |
| V13 | Testes Go de `Normalizar`: JSON malformado, `item_id` inexistente, regra inválida, data passada em registro futuro, >20 itens |

Além do aceite: **bateria de 30 frases** (§10, etapa 3) rodada contra o Groq real, com a taxa de acerto
registrada no README — a régua para mexer no prompt depois.

## 9. Decisões confirmadas (28/09/2026)

| # | Proposta | Por quê | Alternativa |
|---|---|---|---|
| V1 | **Uma ação por captura**, com "Tem mais" para o resto | cartão simples, um toque confirma; frase dupla é minoria | várias ações num cartão só: mais poder, UI e validação bem mais complexas |
| V2 | **Compras entra na captura** | a fase 3 já está no ar; "comprar X" é o recado mais comum | só criar/registrar, como no plano de 26/09 (antes da fase 3 existir) |
| V3 | **Não entendeu → "Salvar como tarefa"** (compromisso sem data com o texto) | recado nunca se perde, mesmo com o LLM confuso | só mostrar erro e mandar para o formulário |
| V4 | **Reusar a `GROQ_API_KEY` do Smaug** | a cota do Groq é por organização — chave nova não dá cota nova | chave própria: só ganha poder revogar uma sem mexer na outra |
| V5 | **Guardar o texto de cada captura** (sem áudio, últimas 500) + o desfecho | é a matéria-prima para corrigir o prompt com erros reais; dado seu, no seu homelab | não guardar nada: menos uma tabela, mas ajuste do prompt no escuro |
| V6 | **Atalho "Falar com o Pocket" no ícone** | custa um bloco no manifest e encurta o caminho até a fala | só a barra dentro de Hoje |

## 10. Build (mesma esteira: contrato Opus → etapa Sonnet → revisão Opus)

1. **Backend:** config, migração 0003, `internal/captura` (Groq + fakes + `Normalizar`), `app.Capturar`,
   rotas do §3, poda no job diário. Testes com fakes e com `httptest` imitando o Groq.
2. **Front:** barra, gravador, cartões, prefill do EditorSheet, atalho do manifest, `dist/` rebuildado.
   Teste no `vite dev` contra o backend local com `Interpretador` fake **e** com o Groq real.
3. **Bateria de frases:** teste Go com build tag `groq` (fora do `go test` normal), 30 frases pt-BR com a
   resposta esperada, rodado no redmilab (onde a chave mora). Ajusta o prompt até ≥ 90%.
4. **Deploy** (push para `celular master`), `GROQ_API_KEY` no `.env` do Pocket no aparelho, aceite V1–V13.
