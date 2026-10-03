# Pocket — checklist de aceite da fase 3 (compras)

Marcar **no Poco** depois do deploy (CONTRATO-FASE3 §8). Feche e abra o app duas vezes para o
service worker novo assumir (a 4ª aba "Compras" tem que aparecer).

Validado no PC antes do deploy (27/09/2026, backend da fase 3 sobre uma cópia do banco real + front no
navegador em 393 px): K2, K3, K4, K5 (servidor derrubado no meio: riscar e "pão 6" ficaram na tela e
foram entregues ao religar), K8, K9, K10 e o lembrete gravado/agendado pela folha de ajustes.
No Poco o que importa é o que o PC não prova: modo avião de verdade e o push da lista.

- [ ] **K1** A aba Compras abre com "Mercado" e as 10 seções; criar a lista "Farmácia" nasce com "Geral"
- [ ] **K2** "leite 2", "Tomates", "tomate" → leite com quantidade 2; tomate não duplica
- [ ] **K3** Tocar risca (vai para o fim da seção, esmaecido); tocar de novo desrisca; "Limpar comprados" some com os riscados
- [ ] **K4** Depois de comprar leite (em Frios e laticínios) uma vez, ele aparece em "de novo?" e, readicionado, cai na mesma seção sozinho
- [ ] **K5** **Modo avião** dentro da lista: a lista continua, riscar/adicionar funcionam na hora, a faixa mostra as mudanças pendentes; ao voltar a rede a faixa some e o servidor fica igual à tela
- [ ] **K6** Lembrete da lista (Ajustes da lista → com lembrete → horário daqui a 3 min) chega com "N itens: seção X, …"; tocar abre a aba Compras nessa lista
- [ ] **K7** Com a lista sem pendentes, o lembrete **não** chega
- [ ] **K8** Hoje mostra "Mercado · N itens" e leva à lista
- [x] **K9** Migração 0002 sobre o banco real sem perda — ensaiada 2× em cópias do banco do aparelho (26 e 27/09) e verificada de novo no deploy
- [x] **K10** Testes Go (nome_chave, dedupe, seção automática, frequentes, idempotência, migração, lembrete com lista vazia) + parser "leite 2" (13 casos) — verdes
