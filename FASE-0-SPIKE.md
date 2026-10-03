# Fase 0 — spike de infra do Pocket

Objetivo: provar, **no Poco M6 Pro real**, que um PWA servido pelo redmilab instala, abre fora de
casa pela tailnet e **recebe notificação com o app fechado**. Nada de domínio do Pocket aqui —
código descartável na raiz de `servicos/pocket/` (o hook só compila `go.mod` em `servicos/<nome>/`; a fase 1 reescreve). O resultado decide D4 (PWA ou Telegram).

## Pré-requisitos (Sergio)

- [x] Chave de API da Hostinger (hPanel → Perfil → API). Está em `~/.acme.sh/account.conf` no aparelho, **nunca no repo**. Passou pelo chat em 26/09 → rotacionar e atualizar o `account.conf`.
- [x] Registro DNS na Hostinger: `A pocket.example.com → 100.64.0.10`, TTL 14400 (resolvendo em 26/09/2026) (IP público→privado é válido; só resolve útil dentro da tailnet).
- [x] Poco na tailnet (confirmado 26/09/2026).

## Passos

1. **Certificado** (no redmilab, Termux) — ✅ **feito em 26/09/2026**, válido até 25/12/2026:
   ```bash
   pkg install openssl-tool                      # acme.sh precisa do binário openssl
   git clone --depth 1 https://github.com/acmesh-official/acme.sh.git && cd acme.sh
   ./acme.sh --install --nocron --home ~/.acme.sh   # Termux não tem crontab
   export HOSTINGER_Token=...                    # ATENÇÃO: é HOSTINGER_Token (a wiki diz HOSTINGER_API_KEY — errado na v3.1.6)
   ~/.acme.sh/acme.sh --issue --server letsencrypt --dns dns_hostinger --dnssleep 180 -d pocket.example.com
   ~/.acme.sh/acme.sh --install-cert -d pocket.example.com --ecc \
     --fullchain-file ~/homelab/servicos/pocket/secrets/tls/fullchain.pem \
     --key-file ~/homelab/servicos/pocket/secrets/tls/key.pem
   ```
   Aprendizados:
   - `--dnssleep 180` é necessário: com a checagem padrão, a Let's Encrypt consultou antes do TXT
     propagar nos nameservers da Hostinger (NXDOMAIN). O valor fica salvo no `.conf` do domínio e a renovação reaproveita.
   - Token salvo em `~/.acme.sh/account.conf` (`SAVED_HOSTINGER_Token`). Ao rotacionar o token no hPanel, trocar ali.
   - `secrets/` é ignorado pelo git e o hook faz `git archive | tar -x` (não apaga não-rastreados), então o deploy não toca nos certs.
   - **Renovação não está automatizada** (sem cron). Janela sugerida pela CA: 24–26/11/2026. Item obrigatório do
     contrato: o `svc-pocket` roda `acme.sh --cron` 1×/dia e o `--install-cert` já copia os arquivos novos; o
     servidor recarrega o cert (ex.: `GetCertificate` relendo o arquivo) sem reiniciar.
2. **Servidor mínimo em Go** (`main.go` + `web/`) — ✅ escrito 26/09: `ListenAndServeTLS` com os arquivos acima; serve
   `index.html`, `manifest.webmanifest`, `sw.js`, e três rotas: `GET /vapid` (chave pública),
   `POST /subscribe` (guarda a subscription em arquivo), `POST /push-test?delay=N` (dispara push após N s).
   Lib: `github.com/SherClockHolmes/webpush-go` (puro Go). Porta: tentar **443**; se o bind falhar,
   `9104` (anotar qual funcionou — o dashboard já escuta na 80, então provavelmente 443 funciona).
3. **Manifest com `share_target`** (GET, params `title`/`text`/`url`) apontando para `/share`, que só
   mostra o que recebeu — valida o fluxo Instagram/YouTube → Pocket sem extrair nada ainda.
4. **No Poco:** abrir `https://pocket.example.com[:porta]` no Chrome → Instalar app → permitir notificações.
   HyperOS: Configurações → Apps → Chrome → Economia de bateria **Sem restrições** + **Início automático** ligado.

## Critérios de aceite (marcar no aparelho)

| # | Teste | Passou? |
|---|---|---|
| A1 | Cert válido: cadeado sem aviso no Chrome do Poco | ✅ 26/09 |
| A2 | PWA instala e abre como app (sem barra do navegador) | ✅ 26/09 |
| A3 | Push com app **aberto** chega | ✅ chegou 0,05 s após o envio; "Feito" na notificação registrado |
| A4 | Push com app **fechado** (removido dos recentes) chega em < 1 min, delay de 5 min | ✅ chegou 1,6 s após o envio (tela ligada) |
| A5 | Push com **tela bloqueada há ≥ 30 min** chega (teste do HyperOS) | ✅ agendado 02:21:02, enviado 02:21:28, chegou 02:22:22 — 54 s no Poco em Doze, 80 s no total |
| A6 | Push fora de casa, no 4G, chega | ✅ Wi-Fi desligado: chegou 0,08 s após o envio; o "Feito" também voltou pelo 4G via tailnet |
| A7 | Instagram → Compartilhar → Pocket aparece na lista e `/share` recebe a URL do reel | ✅ URL chega em `text` (sem `title`) |
| A8 | YouTube → Compartilhar → Pocket idem | ✅ URL em `text`, título do vídeo em `title` (Shorts) |
| A9 | `yt-dlp` no Termux baixa **metadados** (legenda) de 1 reel e 1 vídeo do YouTube | ✅ os dois, **Instagram sem cookies** |

## Decisão

- **A1–A6 passam** → D4 confirmado (PWA puro). Escrever `CONTRATO.md`.
- **A4/A5 falham** mesmo com bateria sem restrição → notificações via **bot do Telegram** (plano B); tela continua PWA.
- **A7/A8 falham** → captura de receita por colar link no app (share target sai do escopo).
- **A9 falha no Instagram** → testar com cookies de conta secundária antes de concluir; se ainda falhar, receitas do Instagram só por upload do vídeo.

## Resultado

- **Porta: 9104.** A 443 dá `bind: permission denied` (Android só deixa root abaixo de 1024; o dashboard na 80 roda com sudo). URL: **https://pocket.example.com:9104**. Rodar o Pocket com sudo só pela 443 não compensa.
- Go no Android roda em UTC (sem `/etc/localtime`): o serviço embute `time/tzdata` e fixa `America/Sao_Paulo` (`POCKET_TZ` sobrescreve). Vale para o app da fase 1.
- Subscription via FCM (`fcm.googleapis.com`), `Urgency: high`, resposta `201 Created`.
- **A5: o atraso tem duas partes.** 54 s do FCM até o Poco em Doze (aceitável, o Android agrupa entregas com a tela bloqueada) e **26 s no próprio redmilab**: o timer do Go disparou atrasado, provável sono profundo da CPU do servidor. Para lembrete de hábito ±1 min é irrelevante; o contrato deve dizer "precisão de minuto" e não prometer segundo. Se incomodar, `termux-wake-lock` no svc.
- **A1–A6 aprovados → D4 confirmado: PWA puro, sem Telegram.**
- **A7–A9 (receitas):** Short do YouTube e reel do Instagram trouxeram a legenda completa via `yt-dlp --dump-json`, o Instagram **sem login**. Nos dois a legenda tinha **só ingredientes**, nenhum modo de preparo → o passo 2 da D8 (vídeo para o Gemini) é necessário na prática, não reserva. Sinal a vigiar: no Instagram `duration` veio 0 — pode indicar que o download do vídeo (não só metadados) vai exigir cookies (D9 segue como plano).
- URLs compartilhadas trazem parâmetros de rastreio (`?is=`, `?stkn=`): normalizar antes de guardar/deduplicar.
- **Fase 0 encerrada em 26/09/2026: A1–A9 aprovados.** Próximo: `CONTRATO.md` da fase 1.
