# Pocket — checklist de aceite da fase 1

Marcar **no Poco**, com o app instalado a partir de https://pocket.example.com:9104 depois do deploy
(CONTRATO §9). Antes de começar: abrir o app uma vez com a notificação já permitida (ele se reinscreve
sozinho) e conferir em Ajustes → "inscrito neste aparelho: sim".

Dica para os testes de horário: crie os lembretes para 3–5 min à frente, feche o app e bloqueie a tela.

- [ ] **C1** Criar um hábito de cada formato (todo dia, dias da semana, X vezes por semana, a cada N dias) e um compromisso com e sem data; todos aparecem certo em Hoje/Compromissos
- [ ] **C2** Marcar e desfazer em Hoje atualiza sequência e taxa (aba Hábitos)
- [ ] **C3** "3× por semana": chip mostra 1/3, 2/3; ao chegar em 3 para de lembrar no resto da semana *(criado no meio da semana, a meta da 1ª semana é menor — decisão do build)*
- [ ] **C4** "A cada 3 dias": depois de marcar some de Hoje e volta no 3º dia; sem marcar, mostra "atrasado 1 dia"
- [ ] **C5** Dia de "todo dia" não marcado vira perdido depois de 00:00 e zera a sequência; "pular" não zera
- [ ] **C6** Marcar um dia passado no calendário do hábito recupera a sequência
- [x] **C7** Lembrete chega no horário (± 2 min) com o app fechado; **Feito** na notificação registra sem abrir o app — ✅ 26/09: "Teste" (-15 min de 11:33) agendado 11:18:00, enviado 11:18:23, chegou 11:18:24, Feito 11:18:35 → registro `origem=notificacao`, compromisso concluído e o lembrete das 11:33 **cancelado** (o lado compromisso do C9)
- [x] **C8** **Adiar 30 min** reenvia 30 min depois — ✅ 26/09: Adiar às 11:31:49 → reenviado 12:01:54, chegou 12:01:57 (revelou o bug de adiar item já concluído, corrigido na fase 3)
- [ ] **C9** Hábito já feito não recebe o lembrete do horário seguinte
- [x] **C10** Lembrete para daqui a 5 min + `svc_restart pocket` no redmilab: o lembrete chega mesmo assim — ✅ 26/09: `svc_restart pocket` às 11:25 com lembrete das 11:31 pendente → enviado 11:31:20, chegou 11:31:34
- [x] **C11** `svc_stop pocket` por 1 h com lembretes nesse intervalo: ao religar, eles **não** chegam em rajada (ficam expirados) — ✅ 27/09: serviço parado 01:42–02:44 com lembretes de 01:47, 01:57 e 02:07 → os três `expirada` ao religar, nenhum enviado
- [x] **C12** Ajustes mostra a validade do certificado; log do `svc-pocket` mostra o `renovar_cert` rodando sem erro — ✅ 26/09: `/health` com `cert_expira_em`; log `renovar_cert rodou` → acme.sh "Skipping. Next renewal time is: 2026-11-25"
- [x] **C13** Com o Wi-Fi e os dados desligados, o app abre com o aviso "sem conexão com o redmilab", não tela branca — ✅ 27/09: com o serviço parado, o app abriu com o aviso de sem conexão
- [x] **C14** Testes Go de domínio (4 formatos, virada de mês/semana, meses de 28–31 dias, fechamento, sequência, taxa, intervalo) — verdes em 26/09/2026 (`go test ./...`)

RAM do serviço logo após o deploy (26/09/2026): **16 MB** RSS (`ps -eo pid,rss,args | grep "[b]in/pocket"`) — alvo do contrato < 40 MB.


**Observação do C11 (27/09):** um `sleep` agendado no Termux atrasou ~3 min porque, com a tela do Redmi apagada e sem wake-lock, a CPU entra em sono profundo e o relógio monotônico para. O tick do scheduler sofre o mesmo efeito (a fase 0 já tinha medido 26 s). Decisão (27/09): o `svc-pocket` passou a rodar com `termux-wake-lock` (`scripts/services.sh`).
