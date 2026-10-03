// Casos de uso da fase 3 (CONTRATO-FASE3.md §2, §4, §5) — listas de
// compras. Mesmo padrão do resto do pacote: nenhum SQL aqui, validação
// primeiro, depois uma Tx só por caso de uso que escreve.
//
// Geração/envio do lembrete de lista (§2.6) ficam para F3-2 — este arquivo
// só grava/valida o campo bruto (ver ListaLembreteEntrada/validarLembrete).
package app

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"pocket/internal/domain"
	"pocket/internal/store"
)

// --- Lista ---

// ListaEntrada é o corpo de POST /api/listas (§5).
type ListaEntrada struct {
	Nome string
}

// ListaPatch é o corpo de PATCH /api/listas/{id} (§5): todo campo é
// opcional (ponteiro nil = não veio).
//
// DECISÃO: LembretePresente/LembreteRemover distinguem os três estados de
// "lembrete" no JSON (chave ausente / "lembrete": null / objeto) porque,
// ao contrário do Regra de item na fase 1 (onde ausente e null significam
// a mesma coisa), aqui o contrato dá sentidos diferentes: ausente = não
// mexe, null = remove (§5). O httpapi monta isto a partir de um
// json.RawMessage não-ponteiro (chave ausente deixa o campo no zero-value
// nil; "null" explícito grava os 4 bytes literais "null", distinguível de
// nil) — o mesmo truque, só que aplicado ao objeto inteiro em vez de só à
// Regra.
type ListaPatch struct {
	Nome             *string
	Ordem            *int
	LembretePresente bool
	LembreteRemover  bool
	Lembrete         *ListaLembreteEntrada
}

// ListaLembreteEntrada é o lembrete cru de §2.6: regra (só diario/
// dias_semana; vezes_periodo/intervalo são recusados aqui) + horarios
// "HH:MM".
type ListaLembreteEntrada struct {
	Regra    []byte
	Horarios []string
}

// ListaComPendentes é a linha de GET /api/listas (§5): lista + quantos
// itens pendentes ela tem.
type ListaComPendentes struct {
	Lista     domain.Lista
	Pendentes int
}

// ListaDetalhe é o corpo de GET /api/listas/{id} (§5): lista + seções
// (ordenadas por ordem) + itens (ordem de §2.3 regra 4 — já vem assim de
// ItensPorLista).
type ListaDetalhe struct {
	Lista  domain.Lista
	Secoes []domain.Secao
	Itens  []domain.ItemCompra
}

// nomeSecaoGeral/nomeSecaoOutros são os nomes usados literalmente por §2.1
// (lista nova nasce com "Geral") e §2.3 regra 2 (seção automática cai em
// "Outros" quando não há histórico).
const (
	nomeSecaoGeral  = "Geral"
	nomeSecaoOutros = "Outros"
)

func validarNomeCurto(campo, nome string, max int) (string, error) {
	n := strings.TrimSpace(nome)
	if utf8.RuneCountInString(n) < 1 || utf8.RuneCountInString(n) > max {
		return "", erroValidacao("%s deve ter entre 1 e %d caracteres", campo, max)
	}
	return n, nil
}

// CriarLista grava uma lista nova com uma seção só, "Geral" (§2.1).
func (a *App) CriarLista(ctx context.Context, in ListaEntrada) (domain.Lista, error) {
	nome, err := validarNomeCurto("nome", in.Nome, 40)
	if err != nil {
		return domain.Lista{}, err
	}

	var criada domain.Lista
	err = a.st.WithTx(ctx, func(tx *store.Tx) error {
		l := domain.Lista{Nome: nome, CriadoEm: domain.FormatInstante(a.agora())}
		id, err := tx.ListaInserir(ctx, l)
		if err != nil {
			return err
		}
		l.ID = id
		if _, err := tx.SecaoInserir(ctx, domain.Secao{ListaID: id, Nome: nomeSecaoGeral, Ordem: 0}); err != nil {
			return err
		}
		criada = l
		return nil
	})
	return criada, err
}

// ListarListas devolve as listas não arquivadas com sua contagem de
// pendentes (§5 GET /api/listas).
func (a *App) ListarListas(ctx context.Context) ([]ListaComPendentes, error) {
	listas, err := a.st.ListaListar(ctx, true)
	if err != nil {
		return nil, err
	}
	out := make([]ListaComPendentes, 0, len(listas))
	for _, l := range listas {
		n, err := a.st.PendentesPorLista(ctx, l.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, ListaComPendentes{Lista: l, Pendentes: n})
	}
	return out, nil
}

// DetalheLista devolve a lista, suas seções (ordenadas) e seus itens (§5
// GET /api/listas/{id}).
func (a *App) DetalheLista(ctx context.Context, listaID int64) (ListaDetalhe, error) {
	l, err := a.st.ListaBuscar(ctx, listaID)
	if err != nil {
		return ListaDetalhe{}, mapearErroStore(err, "lista não encontrada")
	}
	secoes, err := a.st.SecoesPorLista(ctx, listaID)
	if err != nil {
		return ListaDetalhe{}, err
	}
	itens, err := a.st.ItensPorLista(ctx, listaID)
	if err != nil {
		return ListaDetalhe{}, err
	}
	return ListaDetalhe{Lista: l, Secoes: secoes, Itens: itens}, nil
}

// EditarLista aplica patch (§5 PATCH /api/listas/{id}).
func (a *App) EditarLista(ctx context.Context, id int64, patch ListaPatch) (domain.Lista, error) {
	var atualizada domain.Lista
	err := a.st.WithTx(ctx, func(tx *store.Tx) error {
		l, err := tx.ListaBuscar(ctx, id)
		if err != nil {
			return mapearErroStore(err, "lista não encontrada")
		}

		if patch.Nome != nil {
			nome, err := validarNomeCurto("nome", *patch.Nome, 40)
			if err != nil {
				return err
			}
			l.Nome = nome
		}
		if patch.Ordem != nil {
			l.Ordem = *patch.Ordem
		}
		if patch.LembretePresente {
			if patch.LembreteRemover {
				l.Lembrete = nil
			} else {
				lem, err := validarLembrete(patch.Lembrete)
				if err != nil {
					return err
				}
				l.Lembrete = lem
			}
		}

		if err := tx.ListaAtualizar(ctx, l); err != nil {
			return err
		}
		// F3-2: nome/ordem não afetam notificação nenhuma, mas sincronizar
		// sempre (em vez de só quando LembretePresente) é mais simples e
		// correto do mesmo jeito — sem lembrete, desejadas fica nil e o
		// laço de baixo só cancela pendentes de hoje que sobrarem de um
		// lembrete removido.
		if err := a.sincronizarNotificacoesListaHoje(ctx, tx, l, a.hoje()); err != nil {
			return err
		}
		atualizada = l
		return nil
	})
	return atualizada, err
}

// validarLembrete valida o lembrete de lista (§2.6): regra só diario/
// dias_semana (vezes_periodo/intervalo recusados — "não fazem sentido para
// lista"), horários só "HH:MM", pelo menos um.
func validarLembrete(in *ListaLembreteEntrada) (*domain.ListaLembrete, error) {
	if in == nil {
		return nil, erroValidacao("lembrete inválido")
	}
	if len(in.Regra) == 0 {
		return nil, erroValidacao("lembrete precisa de uma regra de frequência")
	}
	regra, err := domain.ParseRegra(in.Regra)
	if err != nil {
		return nil, ErrValidacao{Msg: err.Error()}
	}
	if regra.Tipo != domain.RegraDiario && regra.Tipo != domain.RegraDiasSemana {
		return nil, erroValidacao(`lembrete de lista só aceita regra "diario" ou "dias_semana" (veio %q)`, regra.Tipo)
	}
	if len(in.Horarios) == 0 {
		return nil, erroValidacao("lembrete precisa de ao menos um horário")
	}
	for _, h := range in.Horarios {
		if _, _, err := domain.ParseHoraMinuto(h); err != nil {
			return nil, ErrValidacao{Msg: err.Error()}
		}
	}
	return &domain.ListaLembrete{Regra: regra, Horarios: in.Horarios}, nil
}

// MudarEstadoLista arquiva/desarquiva (§5 POST /api/listas/{id}/estado).
func (a *App) MudarEstadoLista(ctx context.Context, id int64, arquivada bool) (domain.Lista, error) {
	var atualizada domain.Lista
	err := a.st.WithTx(ctx, func(tx *store.Tx) error {
		l, err := tx.ListaBuscar(ctx, id)
		if err != nil {
			return mapearErroStore(err, "lista não encontrada")
		}
		l.Arquivada = arquivada
		if err := tx.ListaAtualizar(ctx, l); err != nil {
			return err
		}
		// F3-2: arquivar cancela as pendentes de hoje (sincronizarNotifica...
		// vê Arquivada=true e trata como "sem lembrete"); desarquivar
		// regenera se a lista ainda tem lembrete (mesmo espírito de "ao
		// retomar" hábito, mudarEstadoHabito acima).
		if err := a.sincronizarNotificacoesListaHoje(ctx, tx, l, a.hoje()); err != nil {
			return err
		}
		atualizada = l
		return nil
	})
	return atualizada, err
}

// --- Seção ---

// CriarSecao grava uma seção nova no fim da lista (§2.2).
func (a *App) CriarSecao(ctx context.Context, listaID int64, nomeIn string) (domain.Secao, error) {
	nome, err := validarNomeCurto("nome", nomeIn, 40)
	if err != nil {
		return domain.Secao{}, err
	}

	var criada domain.Secao
	err = a.st.WithTx(ctx, func(tx *store.Tx) error {
		if _, err := tx.ListaBuscar(ctx, listaID); err != nil {
			return mapearErroStore(err, "lista não encontrada")
		}
		atuais, err := tx.SecoesPorLista(ctx, listaID)
		if err != nil {
			return err
		}
		sc := domain.Secao{ListaID: listaID, Nome: nome, Ordem: len(atuais)}
		id, err := tx.SecaoInserir(ctx, sc)
		if err != nil {
			return err
		}
		sc.ID = id
		criada = sc
		return nil
	})
	return criada, err
}

// SecaoPatch é o corpo de PATCH /api/secoes/{id} (§2.2: renomear/reordenar).
type SecaoPatch struct {
	Nome  *string
	Ordem *int
}

func (a *App) EditarSecao(ctx context.Context, id int64, patch SecaoPatch) (domain.Secao, error) {
	var atualizada domain.Secao
	err := a.st.WithTx(ctx, func(tx *store.Tx) error {
		sc, err := tx.SecaoBuscar(ctx, id)
		if err != nil {
			return mapearErroStore(err, "seção não encontrada")
		}
		if patch.Nome != nil {
			nome, err := validarNomeCurto("nome", *patch.Nome, 40)
			if err != nil {
				return err
			}
			sc.Nome = nome
		}
		if patch.Ordem != nil {
			sc.Ordem = *patch.Ordem
		}
		if err := tx.SecaoAtualizar(ctx, sc); err != nil {
			return err
		}
		atualizada = sc
		return nil
	})
	return atualizada, err
}

// ApagarSecao apaga a seção id, movendo os itens dela para a primeira
// seção restante da lista (§2.2). Apagar a última seção de uma lista é 409
// (toda lista tem ≥ 1 seção).
func (a *App) ApagarSecao(ctx context.Context, id int64) error {
	return a.st.WithTx(ctx, func(tx *store.Tx) error {
		sc, err := tx.SecaoBuscar(ctx, id)
		if err != nil {
			return mapearErroStore(err, "seção não encontrada")
		}
		todas, err := tx.SecoesPorLista(ctx, sc.ListaID)
		if err != nil {
			return err
		}
		if len(todas) <= 1 {
			return erroConflito("não é possível apagar a última seção da lista")
		}
		var destino domain.Secao
		achou := false
		for _, outra := range todas {
			if outra.ID != id {
				destino = outra
				achou = true
				break
			}
		}
		if !achou { // defensivo — não deveria acontecer (len(todas) > 1 garante alguma diferente de id)
			return erroConflito("não é possível apagar a última seção da lista")
		}
		if err := tx.ItemCompraMoverSecao(ctx, id, destino.ID); err != nil {
			return err
		}
		return tx.SecaoApagar(ctx, id)
	})
}

// --- Item de compra ---

// ItemCompraEntrada é o corpo de POST /api/listas/{id}/itens (§5).
type ItemCompraEntrada struct {
	Nome       string
	Quantidade string
	SecaoID    *int64 // nil = seção automática (regra 2 de §2.3)
	Origem     string // "" = manual (default)
}

// AdicionarItem grava um item novo, ou devolve o pendente existente
// (dedupe por nome_chave, regra 1 de §2.3 — quantidade nova, se vier,
// substitui a existente). criado diz se uma linha nova foi gravada (o
// httpapi usa isso para decidir 201 vs 200).
func (a *App) AdicionarItem(ctx context.Context, listaID int64, in ItemCompraEntrada) (item domain.ItemCompra, criado bool, err error) {
	nome, err := validarNomeCurto("nome", in.Nome, 60)
	if err != nil {
		return domain.ItemCompra{}, false, err
	}
	origem := in.Origem
	if origem == "" {
		origem = domain.OrigemCompraManual
	}
	switch origem {
	case domain.OrigemCompraManual, domain.OrigemCompraFrequente, domain.OrigemCompraReceita:
	default:
		return domain.ItemCompra{}, false, erroValidacao("origem inválida (%q)", origem)
	}
	nomeChave := domain.NomeChave(nome)

	err = a.st.WithTx(ctx, func(tx *store.Tx) error {
		if _, err := tx.ListaBuscar(ctx, listaID); err != nil {
			return mapearErroStore(err, "lista não encontrada")
		}

		existente, errBusca := tx.ItemCompraPendentePorNomeChave(ctx, listaID, nomeChave)
		if errBusca == nil {
			// regra 1: já pendente — quantidade nova (se veio) substitui.
			if in.Quantidade != "" {
				existente.Quantidade = in.Quantidade
				if err := tx.ItemCompraAtualizar(ctx, existente); err != nil {
					return err
				}
			}
			item = existente
			criado = false
			return nil
		}
		if !isErrNaoEncontradoStore(errBusca) {
			return errBusca
		}

		secaoID, err := a.resolverSecaoDoItem(ctx, tx, listaID, nomeChave, in.SecaoID)
		if err != nil {
			return err
		}

		novo := domain.ItemCompra{
			ListaID: listaID, SecaoID: secaoID, Nome: nome, NomeChave: nomeChave,
			Quantidade: in.Quantidade, Origem: origem, CriadoEm: domain.FormatInstante(a.agora()),
		}
		id, err := tx.ItemCompraInserir(ctx, novo)
		if err != nil {
			return err
		}
		novo.ID = id
		item = novo
		criado = true
		return nil
	})
	return item, criado, err
}

// resolverSecaoDoItem aplica a regra 2 de §2.3: secaoID explícito (validado
// contra a lista) tem prioridade; senão, a última seção em que esse
// nome_chave foi comprado (histórico); sem histórico, "Outros" se existir,
// senão a primeira seção da lista.
func (a *App) resolverSecaoDoItem(ctx context.Context, tx *store.Tx, listaID int64, nomeChave string, secaoID *int64) (int64, error) {
	if secaoID != nil {
		sc, err := tx.SecaoBuscar(ctx, *secaoID)
		if err != nil || sc.ListaID != listaID {
			return 0, erroValidacao("seção não pertence a esta lista")
		}
		return sc.ID, nil
	}

	hist, err := tx.CompraHistoricoBuscar(ctx, listaID, nomeChave)
	if err == nil && hist.SecaoID != nil {
		if _, err := tx.SecaoBuscar(ctx, *hist.SecaoID); err == nil {
			return *hist.SecaoID, nil
		}
	} else if err != nil && !isErrNaoEncontradoStore(err) {
		return 0, err
	}

	secoes, err := tx.SecoesPorLista(ctx, listaID)
	if err != nil {
		return 0, err
	}
	if len(secoes) == 0 {
		return 0, erroConflito("lista sem seções")
	}
	for _, sc := range secoes {
		if sc.Nome == nomeSecaoOutros {
			return sc.ID, nil
		}
	}
	return secoes[0].ID, nil
}

// ItemCompraPatch é o corpo de PATCH /api/compras/{id} (§5).
type ItemCompraPatch struct {
	Nome       *string
	Quantidade *string
	SecaoID    *int64
	Nota       *string
}

func (a *App) EditarItemCompra(ctx context.Context, id int64, patch ItemCompraPatch) (domain.ItemCompra, error) {
	var atualizado domain.ItemCompra
	err := a.st.WithTx(ctx, func(tx *store.Tx) error {
		ic, err := tx.ItemCompraBuscar(ctx, id)
		if err != nil {
			return mapearErroStore(err, "item não encontrado")
		}

		if patch.Nome != nil {
			nome, err := validarNomeCurto("nome", *patch.Nome, 60)
			if err != nil {
				return err
			}
			novaChave := domain.NomeChave(nome)
			// DECISÃO: editar o nome de um pendente para bater com o
			// nome_chave de outro pendente já existente na lista seria a
			// mesma colisão que a regra 1 de §2.3 evita na criação — 409 em
			// vez de deixar o índice único devolver um erro de SQL cru.
			if !ic.Comprado && novaChave != ic.NomeChave {
				if outro, errBusca := tx.ItemCompraPendentePorNomeChave(ctx, ic.ListaID, novaChave); errBusca == nil && outro.ID != ic.ID {
					return erroConflito("já existe um item pendente com esse nome nesta lista")
				}
			}
			ic.Nome = nome
			ic.NomeChave = novaChave
		}
		if patch.Quantidade != nil {
			ic.Quantidade = *patch.Quantidade
		}
		if patch.Nota != nil {
			ic.Nota = *patch.Nota
		}
		if patch.SecaoID != nil {
			sc, err := tx.SecaoBuscar(ctx, *patch.SecaoID)
			if err != nil || sc.ListaID != ic.ListaID {
				return erroValidacao("seção não pertence a esta lista")
			}
			ic.SecaoID = sc.ID
		}

		if err := tx.ItemCompraAtualizar(ctx, ic); err != nil {
			return err
		}
		atualizado = ic
		return nil
	})
	return atualizado, err
}

func (a *App) ApagarItemCompra(ctx context.Context, id int64) error {
	return a.st.WithTx(ctx, func(tx *store.Tx) error {
		if _, err := tx.ItemCompraBuscar(ctx, id); err != nil {
			return mapearErroStore(err, "item não encontrado")
		}
		return tx.ItemCompraApagar(ctx, id)
	})
}

// Riscar marca o item como comprado (idempotente: repetir não conta 2× no
// histórico) e faz upsert em compra_historico (§2.3 regra 3, §2.4).
func (a *App) Riscar(ctx context.Context, id int64) (domain.ItemCompra, error) {
	var atualizado domain.ItemCompra
	err := a.st.WithTx(ctx, func(tx *store.Tx) error {
		ic, err := tx.ItemCompraBuscar(ctx, id)
		if err != nil {
			return mapearErroStore(err, "item não encontrado")
		}
		if ic.Comprado {
			atualizado = ic
			return nil // idempotente: já riscado, não conta de novo.
		}

		agora := a.agora()
		ic.Comprado = true
		ic.CompradoEm = domain.FormatInstante(agora)
		if err := tx.ItemCompraAtualizar(ctx, ic); err != nil {
			return err
		}

		if err := a.registrarHistoricoCompra(ctx, tx, ic, agora); err != nil {
			return err
		}
		atualizado = ic
		return nil
	})
	return atualizado, err
}

// registrarHistoricoCompra faz o upsert de compra_historico (§2.4) ao
// riscar: vezes += 1, nome/seção/quantidade atualizados para a última
// grafia usada.
func (a *App) registrarHistoricoCompra(ctx context.Context, tx *store.Tx, ic domain.ItemCompra, agora time.Time) error {
	secaoID := ic.SecaoID
	h, err := tx.CompraHistoricoBuscar(ctx, ic.ListaID, ic.NomeChave)
	vezes := 1
	if err == nil {
		vezes = h.Vezes + 1
	} else if !isErrNaoEncontradoStore(err) {
		return err
	}
	return tx.CompraHistoricoUpsert(ctx, domain.CompraHistorico{
		ListaID: ic.ListaID, NomeChave: ic.NomeChave, Nome: ic.Nome, SecaoID: &secaoID,
		UltimaQuantidade: ic.Quantidade, Vezes: vezes, UltimaEm: domain.FormatInstante(agora),
	})
}

// Desriscar desmarca o item (idempotente). Se o item tinha sido riscado no
// mesmo dia local (comparado a agora), desfaz o +1 do histórico (§2.4);
// riscado em outro dia, o histórico fica como está (foi uma compra de
// verdade, em outro dia).
func (a *App) Desriscar(ctx context.Context, id int64) (domain.ItemCompra, error) {
	var atualizado domain.ItemCompra
	err := a.st.WithTx(ctx, func(tx *store.Tx) error {
		ic, err := tx.ItemCompraBuscar(ctx, id)
		if err != nil {
			return mapearErroStore(err, "item não encontrado")
		}
		if !ic.Comprado {
			atualizado = ic
			return nil // idempotente: já não estava riscado.
		}

		mesmoDia := false
		if compradoEm, err := domain.ParseInstante(ic.CompradoEm); err == nil {
			mesmoDia = domain.DiaDeTime(compradoEm.In(a.loc)) == a.hoje()
		}

		ic.Comprado = false
		ic.CompradoEm = ""
		if err := tx.ItemCompraAtualizar(ctx, ic); err != nil {
			return err
		}

		if mesmoDia {
			if err := a.desfazerHistoricoCompra(ctx, tx, ic); err != nil {
				return err
			}
		}
		atualizado = ic
		return nil
	})
	return atualizado, err
}

// desfazerHistoricoCompra desfaz o +1 de registrarHistoricoCompra: vezes -=
// 1; chega a 0 apaga a linha (era a primeira vez, riscar e desriscar no
// mesmo dia não deveria deixar rastro de "já comprou uma vez").
func (a *App) desfazerHistoricoCompra(ctx context.Context, tx *store.Tx, ic domain.ItemCompra) error {
	h, err := tx.CompraHistoricoBuscar(ctx, ic.ListaID, ic.NomeChave)
	if err != nil {
		if isErrNaoEncontradoStore(err) {
			return nil // nada para desfazer (corrida rara / histórico já removido).
		}
		return err
	}
	if h.Vezes <= 1 {
		return tx.CompraHistoricoApagar(ctx, ic.ListaID, ic.NomeChave)
	}
	h.Vezes--
	return tx.CompraHistoricoUpsert(ctx, h)
}

// LimparComprados apaga (hard delete) os itens riscados da lista (§2.3
// regra 3 — o histórico guarda) e devolve quantos foram removidos.
func (a *App) LimparComprados(ctx context.Context, listaID int64) (int, error) {
	var n int
	err := a.st.WithTx(ctx, func(tx *store.Tx) error {
		if _, err := tx.ListaBuscar(ctx, listaID); err != nil {
			return mapearErroStore(err, "lista não encontrada")
		}
		var err error
		n, err = tx.LimparComprados(ctx, listaID)
		return err
	})
	return n, err
}

// Frequentes devolve o histórico da lista menos o que já está pendente,
// ordenado por ScoreFrequente (§2.4), top 20. Com prefixo != "", filtra por
// prefixo de nome_chave (autocompletar).
func (a *App) Frequentes(ctx context.Context, listaID int64, prefixo string) ([]domain.CompraHistorico, error) {
	hist, err := a.st.CompraHistoricoPorLista(ctx, listaID)
	if err != nil {
		return nil, err
	}
	itens, err := a.st.ItensPorLista(ctx, listaID)
	if err != nil {
		return nil, err
	}
	pendentes := make(map[string]bool, len(itens))
	for _, it := range itens {
		if !it.Comprado {
			pendentes[it.NomeChave] = true
		}
	}

	prefixoChave := ""
	if prefixo != "" {
		prefixoChave = domain.NomeChave(prefixo)
	}

	hoje := a.hoje()
	type comScore struct {
		h     domain.CompraHistorico
		score float64
	}
	var candidatos []comScore
	for _, h := range hist {
		if pendentes[h.NomeChave] {
			continue
		}
		if prefixoChave != "" && !strings.HasPrefix(h.NomeChave, prefixoChave) {
			continue
		}
		candidatos = append(candidatos, comScore{h: h, score: scoreDoHistorico(h, hoje)})
	}
	sort.SliceStable(candidatos, func(i, j int) bool {
		if candidatos[i].score != candidatos[j].score {
			return candidatos[i].score > candidatos[j].score
		}
		return candidatos[i].h.Nome < candidatos[j].h.Nome
	})
	if len(candidatos) > 20 {
		candidatos = candidatos[:20]
	}

	out := make([]domain.CompraHistorico, 0, len(candidatos))
	for _, c := range candidatos {
		out = append(out, c.h)
	}
	return out, nil
}

// scoreDoHistorico calcula os dias desde h.UltimaEm até hoje (local) e
// devolve domain.ScoreFrequente — a diferença de dias vem daqui (app,
// dono do fuso/clock) para domain continuar sem I/O.
func scoreDoHistorico(h domain.CompraHistorico, hoje domain.Dia) float64 {
	diasDesde := 0
	if t, err := domain.ParseInstante(h.UltimaEm); err == nil {
		ultimoDia := domain.DiaDeTime(t)
		if hojeT, err1 := hoje.Time(); err1 == nil {
			if ultimoT, err2 := ultimoDia.Time(); err2 == nil {
				diasDesde = int(hojeT.Sub(ultimoT).Hours() / 24)
			}
		}
	}
	return domain.ScoreFrequente(h.Vezes, diasDesde)
}

// isErrNaoEncontradoStore checa store.ErrNaoEncontrado sem importar o tipo
// concreto em toda função deste arquivo (mesmo espírito de
// mapearErroStore, mas devolvendo bool em vez de traduzir para
// app.ErrNaoEncontrado — usado nos pontos em que "não achei" é um caminho
// normal, não um erro a devolver ao cliente).
func isErrNaoEncontradoStore(err error) bool {
	if err == nil {
		return false
	}
	var naoEncontrado store.ErrNaoEncontrado
	return errors.As(err, &naoEncontrado)
}
