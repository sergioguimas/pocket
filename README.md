# Pocket

PWA pessoal de **hábitos, tarefas, evolução e compras**, com captura por voz ou texto, notificações push e uso offline. Backend em Go, frontend em React, banco SQLite. Roda em um celular Android antigo (Redmi Note 8 Pro, via Termux) e é acessado pela rede privada do dono (Tailscale).

> **Cópia ilustrativa, para portfólio.** Este repositório é uma vitrine higienizada. A fonte da verdade é um repositório privado, onde o desenvolvimento acontece. Telefones, endereços, domínio e saldos que aparecem aqui são fictícios. Não há segredos nem dados reais, e o histórico de commits foi reiniciado.

## O que ele faz

| Módulo | Descrição |
|---|---|
| Hábitos, tarefas e lembretes | Rotinas fixas e compromissos pontuais, com notificação push e cobrança. |
| Evolução | Hábito com valor registrado (carga, página, episódio) e histórico. |
| Compras | Várias listas, marcação no mercado, fila offline em IndexedDB e lembrete de lista. |
| Captura por voz e texto | "Anota: supino 3x12 com 40kg". O áudio é transcrito (Whisper via Groq) e interpretado por um LLM, que devolve o item estruturado para confirmação. |

## Pontos técnicos

- **Go puro, sem cgo.** SQLite via `modernc.org/sqlite`, o que permite compilar direto no aparelho ARM.
- **Web Push com VAPID** e **HTTPS com certificado Let's Encrypt** renovado pelo próprio serviço (`acme.sh`). O serviço alerta por push quando faltam menos de 20 dias.
- **Migrations versionadas**, ensaiadas em cópia do banco antes de cada deploy, com backup diário (`VACUUM INTO`, retenção de 7 dias).
- **Frontend PWA** com service worker, fila offline e telas pensadas para uso com uma mão.
- **Cerca de 234 testes Go** e testes de parsing no frontend. O backend roda sobre SQLite real nos testes, sem mocks de banco.

## Como foi construído

O projeto seguiu um processo orientado a contrato, e os documentos estão neste repositório:

1. [`CONTRATO.md`](CONTRATO.md), [`CONTRATO-FASE3.md`](CONTRATO-FASE3.md) e [`CONTRATO-FASE15.md`](CONTRATO-FASE15.md) definem comportamento, API e critérios de aceite antes do código.
2. [`PLANO-DE-BUILD.md`](PLANO-DE-BUILD.md) quebra o contrato em etapas, cada uma com seu prompt e seu checklist de revisão.
3. [`CHECKLIST-ACEITE*.md`](CHECKLIST-ACEITE.md) é a conferência manual no aparelho antes de dar a fase como pronta.
4. [`FASE-0-SPIKE.md`](FASE-0-SPIKE.md) registra o spike que validou push, TLS e share target antes de se comprometer com a arquitetura.

A API está em [`API.md`](API.md). Detalhes de operação e decisões de design (D1 a D9) estão em [`OPERACAO.md`](OPERACAO.md).

## Rodando localmente

```bash
# backend
cp .env.example .env        # defina POCKET_LISTEN=127.0.0.1:9104 e deixe POCKET_ACME= vazio
cd frontend && npm ci && npm run build && cd ..
go test ./...
go run .
```

O diretório `dist/` vem vazio (só `.gitkeep`) porque é gerado por `npm run build` e embutido no binário com `go:embed`. Sem o build o servidor sobe, mas sem a interface.

A captura por voz precisa de uma `GROQ_API_KEY`. Sem ela, o recurso fica desligado e o resto funciona normalmente.

## Estrutura

```
main.go            entrada, embed do frontend
internal/          domínio, store (SQLite + migrations), API HTTP, scheduler, push, captura
frontend/          React + TypeScript + Vite (PWA)
*.md               contratos, plano de build, checklists de aceite, API
```

## Licença

Sem licença de uso: o código está público para leitura e avaliação. Todos os direitos reservados.
