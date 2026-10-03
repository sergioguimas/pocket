# Aceite da fase 1.5 — captura por voz e texto (marcar no Poco)

Critérios de [CONTRATO-FASE15.md §8](CONTRATO-FASE15.md). Antes de começar: feche e abra o Pocket
**duas vezes** (o service worker novo assume) e confira a barra "Fale ou escreva…" no topo de Hoje.
Na primeira vez que tocar no microfone, o Chrome pede permissão — libere.

Limite do Groq gratuito: ~2–3 capturas por minuto (8 mil tokens/min). Se aparecer "limite do Groq
atingido", espere um minuto — não é defeito.

| # | Como testar | Esperado | ✓ |
|---|---|---|---|
| V1 | Microfone: "dentista dia 14 às 15h, me lembra um dia antes" | formulário de compromisso: 14/10, 15:00, "1 dia antes" marcado. Cancelar (não precisa salvar) | ✓ 28/09 |
| V2 | Microfone: "ler 20 minutos todo dia às 21h" | formulário de hábito "Todo dia", lembrete 21:00 | ✓ 28/09 |
| V3 | Microfone: "academia 3 vezes por semana" | hábito "X vezes por semana" com 3 | ✓ 28/09 |
| V4 | Microfone: "fiz o devocional, Romanos 8" | cartão "Registrar **Devocional** hoje?" com a nota; registrar → aparece em Últimos registros do Devocional | ✓ 28/09 |
| V5 | Microfone: "ontem fui na academia" (ou outro hábito seu) | cartão com "ontem" | ✓ 28/09 |
| V6 | Microfone: "comprar leite, pão e 2 kg de arroz" | 3 itens, arroz com "2 kg", em Mercado; confirmar → itens na aba Compras | ✓ 28/09 |
| V7 | Microfone: "dentista amanhã e comprar leite" | formulário do dentista + "Tem mais: comprar leite" na barra; tocar → cartão de compras | ✓ 28/09 |
| V8 | Escrever "asdf qwer" e enviar | cartão "Não entendi bem" com **salvar como tarefa** → vira compromisso sem data | ✓ 28/09 |
| V9 | Nas capturas acima, contar do fim da fala até o cartão | ≤ 5 s na maioria | ✓ 28/09 |
| V10 | Modo avião (com o Tailscale ligado) | abrir o app: abre em ≤ 4 s com o aviso "sem conexão com o redmilab". Com o app aberto: uma captura para em ≤ 75 s (ou no "cancelar") com "sem conexão" e "tentar de novo"; quando a rede volta, o aviso some sozinho em ~10 s | |
| V11 | Segurar o ícone do Pocket na tela inicial → "Falar com o Pocket" | abre gravando, ou com o microfone piscando em destaque (um toque grava) | |
| V12 | Abrir um cartão qualquer e fechar no X | nada foi criado/registrado | ✓ 28/09 |
| V13 | (testes Go — já passam no build) | — | ✓ |

Anote a frase exata de qualquer erro de interpretação: cada captura fica guardada (tabela `captura`,
com o desfecho), e é com esses casos reais que o prompt melhora.
