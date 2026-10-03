package app

import (
	"context"
	"testing"
	"time"
)

func TestCriarListaNasceComSecaoGeral(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()

	l, err := a.CriarLista(ctx, ListaEntrada{Nome: "Farmácia"})
	if err != nil {
		t.Fatalf("CriarLista: %v", err)
	}

	det, err := a.DetalheLista(ctx, l.ID)
	if err != nil {
		t.Fatalf("DetalheLista: %v", err)
	}
	if len(det.Secoes) != 1 || det.Secoes[0].Nome != "Geral" {
		t.Fatalf("secoes = %+v, want só 'Geral'", det.Secoes)
	}
}

func TestAdicionarItemDedupePorNomeChave(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()

	l, err := a.CriarLista(ctx, ListaEntrada{Nome: "Mercado teste"})
	if err != nil {
		t.Fatalf("CriarLista: %v", err)
	}

	leite, criado, err := a.AdicionarItem(ctx, l.ID, ItemCompraEntrada{Nome: "leite", Quantidade: "2"})
	if err != nil || !criado {
		t.Fatalf("AdicionarItem(leite): item=%+v criado=%v err=%v", leite, criado, err)
	}

	tomates, criado, err := a.AdicionarItem(ctx, l.ID, ItemCompraEntrada{Nome: "Tomates"})
	if err != nil || !criado {
		t.Fatalf("AdicionarItem(Tomates): err=%v criado=%v", err, criado)
	}
	tomate, criado, err := a.AdicionarItem(ctx, l.ID, ItemCompraEntrada{Nome: "tomate"})
	if err != nil {
		t.Fatalf("AdicionarItem(tomate): %v", err)
	}
	if criado {
		t.Fatalf("AdicionarItem(tomate) devia devolver o existente (nome_chave igual a Tomates), não criar outro")
	}
	if tomate.ID != tomates.ID {
		t.Fatalf("tomate.ID=%d, tomates.ID=%d — deviam ser o mesmo item", tomate.ID, tomates.ID)
	}

	det, err := a.DetalheLista(ctx, l.ID)
	if err != nil {
		t.Fatalf("DetalheLista: %v", err)
	}
	if len(det.Itens) != 2 {
		t.Fatalf("itens = %+v, want 2 (leite + tomate, sem duplicar)", det.Itens)
	}

	// leite readicionado com quantidade substitui.
	leite2, criado, err := a.AdicionarItem(ctx, l.ID, ItemCompraEntrada{Nome: "leite", Quantidade: "3"})
	if err != nil {
		t.Fatalf("AdicionarItem(leite de novo): %v", err)
	}
	if criado {
		t.Fatalf("leite de novo devia devolver o existente")
	}
	if leite2.ID != leite.ID || leite2.Quantidade != "3" {
		t.Fatalf("leite2 = %+v, want id=%d quantidade=3", leite2, leite.ID)
	}
}

func TestAdicionarItemSecaoAutomaticaPorHistorico(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()

	l, err := a.CriarLista(ctx, ListaEntrada{Nome: "Mercado teste"})
	if err != nil {
		t.Fatalf("CriarLista: %v", err)
	}
	frios, err := a.CriarSecao(ctx, l.ID, "Frios e laticínios")
	if err != nil {
		t.Fatalf("CriarSecao: %v", err)
	}

	// primeira vez: sem histórico, cai na seção "Geral" (única já
	// existente antes da nova, já que "Outros" não existe nesta lista).
	leite, _, err := a.AdicionarItem(ctx, l.ID, ItemCompraEntrada{Nome: "leite"})
	if err != nil {
		t.Fatalf("AdicionarItem(leite): %v", err)
	}

	// move manualmente para "Frios e laticínios" e risca — grava histórico
	// com essa seção.
	if _, err := a.EditarItemCompra(ctx, leite.ID, ItemCompraPatch{SecaoID: &frios.ID}); err != nil {
		t.Fatalf("EditarItemCompra: %v", err)
	}
	if _, err := a.Riscar(ctx, leite.ID); err != nil {
		t.Fatalf("Riscar: %v", err)
	}

	// readiciona sem secao_id: regra 2 usa o histórico (Frios e laticínios).
	leite2, criado, err := a.AdicionarItem(ctx, l.ID, ItemCompraEntrada{Nome: "leite"})
	if err != nil || !criado {
		t.Fatalf("AdicionarItem(leite, 2a vez): item=%+v criado=%v err=%v", leite2, criado, err)
	}
	if leite2.SecaoID != frios.ID {
		t.Fatalf("leite2.SecaoID = %d, want %d (Frios e laticínios, do histórico)", leite2.SecaoID, frios.ID)
	}
}

func TestAdicionarItemSecaoAutomaticaCaiEmOutros(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()

	l, err := a.CriarLista(ctx, ListaEntrada{Nome: "Mercado teste"})
	if err != nil {
		t.Fatalf("CriarLista: %v", err)
	}
	outros, err := a.CriarSecao(ctx, l.ID, "Outros")
	if err != nil {
		t.Fatalf("CriarSecao: %v", err)
	}

	item, _, err := a.AdicionarItem(ctx, l.ID, ItemCompraEntrada{Nome: "coisa qualquer"})
	if err != nil {
		t.Fatalf("AdicionarItem: %v", err)
	}
	if item.SecaoID != outros.ID {
		t.Fatalf("item.SecaoID = %d, want %d (Outros)", item.SecaoID, outros.ID)
	}
}

func TestRiscarDesriscarIdempotenteNoHistorico(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()

	l, err := a.CriarLista(ctx, ListaEntrada{Nome: "Mercado teste"})
	if err != nil {
		t.Fatalf("CriarLista: %v", err)
	}
	leite, _, err := a.AdicionarItem(ctx, l.ID, ItemCompraEntrada{Nome: "leite"})
	if err != nil {
		t.Fatalf("AdicionarItem: %v", err)
	}

	if _, err := a.Riscar(ctx, leite.ID); err != nil {
		t.Fatalf("Riscar (1a): %v", err)
	}
	if _, err := a.Riscar(ctx, leite.ID); err != nil {
		t.Fatalf("Riscar (repetido): %v", err)
	}

	if _, err := a.Desriscar(ctx, leite.ID); err != nil {
		t.Fatalf("Desriscar (1a): %v", err)
	}
	if _, err := a.Desriscar(ctx, leite.ID); err != nil {
		t.Fatalf("Desriscar (repetido): %v", err)
	}

	// riscar de novo, no mesmo dia — o histórico devia continuar em 1 (não
	// 2), já que o desriscar desfez o +1 do primeiro riscar.
	if _, err := a.Riscar(ctx, leite.ID); err != nil {
		t.Fatalf("Riscar (2a): %v", err)
	}
	if _, err := a.LimparComprados(ctx, l.ID); err != nil {
		t.Fatalf("LimparComprados: %v", err)
	}

	freq, err := a.Frequentes(ctx, l.ID, "")
	if err != nil {
		t.Fatalf("Frequentes: %v", err)
	}
	if len(freq) != 1 || freq[0].NomeChave != "leite" || freq[0].Vezes != 1 {
		t.Fatalf("Frequentes = %+v, want [{leite vezes=1}]", freq)
	}
}

func TestFrequentesExcluiPendentesEOrdenaPorScore(t *testing.T) {
	a, fake := novaAppTeste(t, segunda28)
	ctx := context.Background()

	l, err := a.CriarLista(ctx, ListaEntrada{Nome: "Mercado teste"})
	if err != nil {
		t.Fatalf("CriarLista: %v", err)
	}

	// "leite": comprado 3 vezes, a mais recente há 1 dia -> score alto.
	comprarERemover(t, a, ctx, l.ID, "leite")
	comprarERemover(t, a, ctx, l.ID, "leite")
	comprarERemover(t, a, ctx, l.ID, "leite")
	fake.Advance(24 * time.Hour)

	// "sabao": comprado uma vez, e o tempo segue avançando bastante depois
	// -> score baixo (pouca frequência, pouca recência).
	comprarERemover(t, a, ctx, l.ID, "sabao")
	fake.Advance(60 * 24 * time.Hour)

	// "acucar": tem histórico de uma compra anterior, mas está pendente
	// agora — não deve aparecer em Frequentes.
	comprarERemover(t, a, ctx, l.ID, "acucar")
	if _, _, err := a.AdicionarItem(ctx, l.ID, ItemCompraEntrada{Nome: "acucar"}); err != nil {
		t.Fatalf("AdicionarItem(acucar pendente): %v", err)
	}

	freq, err := a.Frequentes(ctx, l.ID, "")
	if err != nil {
		t.Fatalf("Frequentes: %v", err)
	}
	var nomes []string
	for _, h := range freq {
		nomes = append(nomes, h.NomeChave)
	}
	if len(nomes) != 2 {
		t.Fatalf("Frequentes = %v, want 2 (leite, sabao — acucar está pendente)", nomes)
	}
	if nomes[0] != "leite" {
		t.Fatalf("Frequentes[0] = %q, want leite (score mais alto: mais vezes e mais recente)", nomes[0])
	}
}

func comprarERemover(t *testing.T, a *App, ctx context.Context, listaID int64, nome string) {
	t.Helper()
	item, _, err := a.AdicionarItem(ctx, listaID, ItemCompraEntrada{Nome: nome})
	if err != nil {
		t.Fatalf("AdicionarItem(%s): %v", nome, err)
	}
	if _, err := a.Riscar(ctx, item.ID); err != nil {
		t.Fatalf("Riscar(%s): %v", nome, err)
	}
	if _, err := a.LimparComprados(ctx, listaID); err != nil {
		t.Fatalf("LimparComprados: %v", err)
	}
}

func TestFrequentesFiltraPorPrefixo(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()

	l, err := a.CriarLista(ctx, ListaEntrada{Nome: "Mercado teste"})
	if err != nil {
		t.Fatalf("CriarLista: %v", err)
	}
	comprarERemover(t, a, ctx, l.ID, "tomate")
	comprarERemover(t, a, ctx, l.ID, "toranja")
	comprarERemover(t, a, ctx, l.ID, "batata")

	freq, err := a.Frequentes(ctx, l.ID, "tom")
	if err != nil {
		t.Fatalf("Frequentes(tom): %v", err)
	}
	if len(freq) != 1 || freq[0].NomeChave != "tomate" {
		t.Fatalf("Frequentes(tom) = %+v, want só tomate", freq)
	}
}

func TestApagarSecaoMoveItensEUltimaDa409(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()

	l, err := a.CriarLista(ctx, ListaEntrada{Nome: "Mercado teste"})
	if err != nil {
		t.Fatalf("CriarLista: %v", err)
	}
	det, err := a.DetalheLista(ctx, l.ID)
	if err != nil {
		t.Fatalf("DetalheLista: %v", err)
	}
	geral := det.Secoes[0]

	extra, err := a.CriarSecao(ctx, l.ID, "Extra")
	if err != nil {
		t.Fatalf("CriarSecao: %v", err)
	}
	item, _, err := a.AdicionarItem(ctx, l.ID, ItemCompraEntrada{Nome: "coisa", SecaoID: &extra.ID})
	if err != nil {
		t.Fatalf("AdicionarItem: %v", err)
	}

	if err := a.ApagarSecao(ctx, extra.ID); err != nil {
		t.Fatalf("ApagarSecao(extra): %v", err)
	}
	relido, err := a.EditarItemCompra(ctx, item.ID, ItemCompraPatch{})
	if err != nil {
		t.Fatalf("reler item: %v", err)
	}
	if relido.SecaoID != geral.ID {
		t.Fatalf("item.SecaoID após apagar seção = %d, want %d (Geral, primeira restante)", relido.SecaoID, geral.ID)
	}

	// apagar a última seção restante ("Geral") é 409.
	err = a.ApagarSecao(ctx, geral.ID)
	if _, ok := err.(ErrConflito); !ok {
		t.Fatalf("ApagarSecao(última): err = %v, want ErrConflito", err)
	}
}

func TestEditarListaLembreteNullRemove(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()

	l, err := a.CriarLista(ctx, ListaEntrada{Nome: "Mercado teste"})
	if err != nil {
		t.Fatalf("CriarLista: %v", err)
	}

	lem := &ListaLembreteEntrada{Regra: []byte(`{"tipo":"diario"}`), Horarios: []string{"09:00"}}
	l2, err := a.EditarLista(ctx, l.ID, ListaPatch{LembretePresente: true, Lembrete: lem})
	if err != nil {
		t.Fatalf("EditarLista (setar lembrete): %v", err)
	}
	if l2.Lembrete == nil {
		t.Fatalf("lembrete não gravado")
	}

	l3, err := a.EditarLista(ctx, l.ID, ListaPatch{LembretePresente: true, LembreteRemover: true})
	if err != nil {
		t.Fatalf("EditarLista (remover lembrete): %v", err)
	}
	if l3.Lembrete != nil {
		t.Fatalf("lembrete = %+v, want nil após remover", l3.Lembrete)
	}
}

func TestEditarListaLembreteRejeitaVezesPeriodo(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()

	l, err := a.CriarLista(ctx, ListaEntrada{Nome: "Mercado teste"})
	if err != nil {
		t.Fatalf("CriarLista: %v", err)
	}

	lem := &ListaLembreteEntrada{Regra: []byte(`{"tipo":"vezes_periodo","vezes":3,"periodo":"semana"}`), Horarios: []string{"09:00"}}
	_, err = a.EditarLista(ctx, l.ID, ListaPatch{LembretePresente: true, Lembrete: lem})
	if _, ok := err.(ErrValidacao); !ok {
		t.Fatalf("EditarLista (vezes_periodo): err = %v, want ErrValidacao", err)
	}
}

func TestHojeComprasListaComPendentes(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()

	l, err := a.CriarLista(ctx, ListaEntrada{Nome: "Mercado teste"})
	if err != nil {
		t.Fatalf("CriarLista: %v", err)
	}
	if _, _, err := a.AdicionarItem(ctx, l.ID, ItemCompraEntrada{Nome: "leite"}); err != nil {
		t.Fatalf("AdicionarItem: %v", err)
	}

	hoje, err := a.Hoje(ctx)
	if err != nil {
		t.Fatalf("Hoje: %v", err)
	}
	achou := false
	for _, c := range hoje.Compras {
		if c.ListaID == l.ID {
			achou = true
			if c.Pendentes != 1 || c.Nome != "Mercado teste" {
				t.Fatalf("HojeLista = %+v, want pendentes=1 nome=Mercado teste", c)
			}
		}
	}
	if !achou {
		t.Fatalf("Hoje.Compras = %+v, want a lista %d nela", hoje.Compras, l.ID)
	}
}
