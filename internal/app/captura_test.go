package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"pocket/internal/captura"
	"pocket/internal/domain"
)

func TestCapturaAtivaSemSetCapturaEhFalse(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	if a.CapturaAtiva() {
		t.Fatal("CapturaAtiva() deveria ser false sem SetCaptura")
	}
}

func TestCapturarSemCapturaAtivaDevolveErrCapturaDesligada(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	_, err := a.Capturar(context.Background(), CapturaEntrada{Texto: "fiz o devocional"})
	var desligada ErrCapturaDesligada
	if !errors.As(err, &desligada) {
		t.Fatalf("err = %v, want ErrCapturaDesligada", err)
	}
}

func modelJSONDesconhecido() []byte {
	return []byte(`{"acao":"desconhecido","item":{"tipo":null,"titulo":null,"notas":null,"regra_tipo":null,"regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":[]},"registro":{"item_id":null,"titulo":null,"dia":null,"nota":null},"compras":{"lista_nome":null,"itens":[]},"resto":null}`)
}

func TestCapturarTextoAtivaAoRegistrarHabitoDoContexto(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()

	item, err := a.CriarItem(ctx, ItemEntrada{Tipo: domain.TipoHabito, Titulo: "Devocional", Regra: []byte(`{"tipo":"diario"}`)})
	if err != nil {
		t.Fatalf("CriarItem: %v", err)
	}

	modelJSON := []byte(`{"acao":"registrar","item":{"tipo":null,"titulo":null,"notas":null,"regra_tipo":null,"regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":[]},"registro":{"item_id":` + itoaTeste(item.ID) + `,"titulo":"Devocional","dia":null,"nota":"Romanos 8"},"compras":{"lista_nome":null,"itens":[]},"resto":null}`)
	interp := &captura.FakeInterpretador{ModelJSON: modelJSON, Uso: captura.Uso{PromptTokens: 10, CompletionTokens: 5}}
	a.SetCaptura(nil, interp, "modelo-teste")

	if !a.CapturaAtiva() {
		t.Fatal("CapturaAtiva() deveria ser true depois de SetCaptura")
	}

	res, err := a.Capturar(ctx, CapturaEntrada{Texto: "fiz o devocional, Romanos 8"})
	if err != nil {
		t.Fatalf("Capturar: %v", err)
	}
	if res.ID == 0 {
		t.Fatal("esperava um captura_id != 0")
	}
	if res.Resposta.Acao != captura.AcaoRegistrar || res.Resposta.Registro == nil {
		t.Fatalf("resposta = %+v", res.Resposta)
	}
	if res.Resposta.Registro.ItemID != item.ID {
		t.Fatalf("ItemID = %d, want %d", res.Resposta.Registro.ItemID, item.ID)
	}

	// o contexto que chegou no interpretador continha o hábito recém-criado.
	if len(interp.Chamadas) != 1 {
		t.Fatalf("chamadas ao interpretador = %d, want 1", len(interp.Chamadas))
	}
	ctxRecebido := interp.Chamadas[0].Contexto
	if len(ctxRecebido.Habitos) != 1 || ctxRecebido.Habitos[0].ID != item.ID {
		t.Fatalf("contexto.Habitos = %+v", ctxRecebido.Habitos)
	}

	// persistiu a linha na tabela captura.
	linha, err := a.st.CapturaBuscar(ctx, res.ID)
	if err != nil {
		t.Fatalf("CapturaBuscar: %v", err)
	}
	if linha.Texto != "fiz o devocional, Romanos 8" || linha.Erro != "" || linha.Resposta == "" {
		t.Fatalf("linha = %+v", linha)
	}
	if linha.Origem != domain.OrigemCapturaTexto {
		t.Fatalf("Origem = %q, want texto", linha.Origem)
	}
}

func TestCapturarAudioTranscritoVazioNaoChamaInterpretador(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()

	transc := &captura.FakeTranscritor{Texto: "   "}
	interp := &captura.FakeInterpretador{ModelJSON: modelJSONDesconhecido()}
	a.SetCaptura(transc, interp, "modelo-teste")

	res, err := a.Capturar(ctx, CapturaEntrada{Audio: []byte("audio-fake"), Mime: "audio/webm"})
	if err != nil {
		t.Fatalf("Capturar: %v", err)
	}
	if res.Resposta.Acao != captura.AcaoDesconhecido {
		t.Fatalf("Acao = %q, want desconhecido", res.Resposta.Acao)
	}
	if len(res.Resposta.Avisos) == 0 {
		t.Fatal("esperava aviso 'não ouvi nada'")
	}
	if len(interp.Chamadas) != 0 {
		t.Fatalf("interpretador não deveria ter sido chamado, chamadas = %d", len(interp.Chamadas))
	}
	if len(transc.Chamadas) != 1 || string(transc.Chamadas[0].Audio) != "audio-fake" || transc.Chamadas[0].Mime != "audio/webm" {
		t.Fatalf("transc.Chamadas = %+v", transc.Chamadas)
	}

	linha, err := a.st.CapturaBuscar(ctx, res.ID)
	if err != nil {
		t.Fatalf("CapturaBuscar: %v", err)
	}
	if linha.Origem != domain.OrigemCapturaVoz {
		t.Fatalf("Origem = %q, want voz", linha.Origem)
	}
}

func TestCapturarErroDeLimiteVira429EPersisteErro(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()

	interp := &captura.FakeInterpretador{Err: captura.ErrLimite{Msg: "limite do Groq atingido — espere um minuto ou use o formulário"}}
	a.SetCaptura(nil, interp, "modelo-teste")

	_, err := a.Capturar(ctx, CapturaEntrada{Texto: "dentista amanhã"})
	var limite ErrCapturaLimite
	if !errors.As(err, &limite) {
		t.Fatalf("err = %v, want ErrCapturaLimite", err)
	}

	// persistiu com erro mesmo sem sucesso.
	linhas := contarCapturas(t, ctx, a)
	if linhas != 1 {
		t.Fatalf("linhas em captura = %d, want 1", linhas)
	}
}

func TestCapturarErroDeProvedorVira502(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()

	interp := &captura.FakeInterpretador{Err: captura.ErrProvedor{Msg: "groq indisponível"}}
	a.SetCaptura(nil, interp, "modelo-teste")

	_, err := a.Capturar(ctx, CapturaEntrada{Texto: "dentista amanhã"})
	var provedor ErrCapturaProvedor
	if !errors.As(err, &provedor) {
		t.Fatalf("err = %v, want ErrCapturaProvedor", err)
	}
}

func TestCapturarContextoIncluiListasNaoArquivadas(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()

	lista, err := a.CriarLista(ctx, ListaEntrada{Nome: "Farmácia"})
	if err != nil {
		t.Fatalf("CriarLista: %v", err)
	}

	interp := &captura.FakeInterpretador{ModelJSON: modelJSONDesconhecido()}
	a.SetCaptura(nil, interp, "modelo-teste")

	if _, err := a.Capturar(ctx, CapturaEntrada{Texto: "hmm"}); err != nil {
		t.Fatalf("Capturar: %v", err)
	}
	if len(interp.Chamadas) != 1 {
		t.Fatalf("chamadas = %d, want 1", len(interp.Chamadas))
	}
	// migrations/0002_compras.sql já semeia a lista "Mercado" — a nova
	// "Farmácia" precisa aparecer junto, não sozinha.
	listas := interp.Chamadas[0].Contexto.Listas
	achou := false
	for _, l := range listas {
		if l.ID == lista.ID && l.Nome == "Farmácia" {
			achou = true
		}
	}
	if !achou {
		t.Fatalf("Listas = %+v, esperava conter Farmácia (id %d)", listas, lista.ID)
	}
}

func TestRegistrarDesfecho(t *testing.T) {
	a, _ := novaAppTeste(t, segunda28)
	ctx := context.Background()

	interp := &captura.FakeInterpretador{ModelJSON: modelJSONDesconhecido()}
	a.SetCaptura(nil, interp, "modelo-teste")

	res, err := a.Capturar(ctx, CapturaEntrada{Texto: "hmm"})
	if err != nil {
		t.Fatalf("Capturar: %v", err)
	}

	if err := a.RegistrarDesfecho(ctx, res.ID, domain.DesfechoConfirmada); err != nil {
		t.Fatalf("RegistrarDesfecho: %v", err)
	}
	linha, err := a.st.CapturaBuscar(ctx, res.ID)
	if err != nil {
		t.Fatalf("CapturaBuscar: %v", err)
	}
	if linha.Desfecho != domain.DesfechoConfirmada {
		t.Fatalf("Desfecho = %q", linha.Desfecho)
	}

	if err := a.RegistrarDesfecho(ctx, res.ID, "inválido"); err == nil {
		t.Fatal("esperava erro de validação para desfecho inválido")
	}
	if err := a.RegistrarDesfecho(ctx, 999999, domain.DesfechoConfirmada); err == nil {
		t.Fatal("esperava ErrNaoEncontrado para id inexistente")
	}
}

func itoaTeste(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func contarCapturas(t *testing.T, ctx context.Context, a *App) int {
	t.Helper()
	n := 0
	for id := int64(1); id < 1000; id++ {
		if _, err := a.st.CapturaBuscar(ctx, id); err == nil {
			n++
		}
	}
	return n
}
