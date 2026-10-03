package store

import (
	"testing"
	"time"

	"pocket/internal/domain"
)

func TestCapturaInserirBuscar(t *testing.T) {
	s := abrirTeste(t)
	ctx := t.Context()

	em := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	c := domain.Captura{
		Em: em, Origem: domain.OrigemCapturaTexto, Texto: "fiz o devocional",
		Resposta: `{"acao":"registrar"}`, Modelo: "openai/gpt-oss-120b", LatenciaMs: 1200,
	}
	id, err := capturaInserir(ctx, s.db, c)
	if err != nil {
		t.Fatalf("capturaInserir: %v", err)
	}

	lida, err := capturaBuscar(ctx, s.db, id)
	if err != nil {
		t.Fatalf("capturaBuscar: %v", err)
	}
	if lida.Texto != "fiz o devocional" || lida.Resposta != `{"acao":"registrar"}` {
		t.Fatalf("captura lida = %+v", lida)
	}
	if lida.Erro != "" || lida.Desfecho != "" {
		t.Fatalf("erro/desfecho deveriam ser vazios: %+v", lida)
	}
	if !lida.Em.Equal(em) {
		t.Errorf("em = %v, want %v", lida.Em, em)
	}

	if _, err := capturaBuscar(ctx, s.db, 999); !isErrNaoEncontrado(err) {
		t.Fatalf("capturaBuscar inexistente: err = %v, want ErrNaoEncontrado", err)
	}
}

func TestCapturaInserirComErro(t *testing.T) {
	s := abrirTeste(t)
	ctx := t.Context()

	em := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	c := domain.Captura{
		Em: em, Origem: domain.OrigemCapturaVoz, Texto: "dentista amanhã",
		Erro: "groq indisponível", Modelo: "openai/gpt-oss-120b", LatenciaMs: 20000,
	}
	id, err := capturaInserir(ctx, s.db, c)
	if err != nil {
		t.Fatalf("capturaInserir: %v", err)
	}
	lida, err := capturaBuscar(ctx, s.db, id)
	if err != nil {
		t.Fatalf("capturaBuscar: %v", err)
	}
	if lida.Resposta != "" || lida.Erro != "groq indisponível" {
		t.Fatalf("captura lida = %+v", lida)
	}
}

func TestCapturaDefinirDesfecho(t *testing.T) {
	s := abrirTeste(t)
	ctx := t.Context()

	em := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	id, err := capturaInserir(ctx, s.db, domain.Captura{
		Em: em, Origem: domain.OrigemCapturaTexto, Texto: "x", Resposta: "{}", Modelo: "m",
	})
	if err != nil {
		t.Fatalf("capturaInserir: %v", err)
	}

	if err := capturaDefinirDesfecho(ctx, s.db, id, domain.DesfechoConfirmada); err != nil {
		t.Fatalf("capturaDefinirDesfecho: %v", err)
	}
	lida, err := capturaBuscar(ctx, s.db, id)
	if err != nil {
		t.Fatalf("capturaBuscar: %v", err)
	}
	if lida.Desfecho != domain.DesfechoConfirmada {
		t.Errorf("desfecho = %q, want %q", lida.Desfecho, domain.DesfechoConfirmada)
	}

	if err := capturaDefinirDesfecho(ctx, s.db, 999, domain.DesfechoConfirmada); !isErrNaoEncontrado(err) {
		t.Fatalf("capturaDefinirDesfecho inexistente: err = %v, want ErrNaoEncontrado", err)
	}
}

func TestCapturaPodarMantemMaisRecentes(t *testing.T) {
	s := abrirTeste(t)
	ctx := t.Context()

	em := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	var ultimoID int64
	for i := 0; i < 10; i++ {
		id, err := capturaInserir(ctx, s.db, domain.Captura{
			Em: em.Add(time.Duration(i) * time.Minute), Origem: domain.OrigemCapturaTexto,
			Texto: "x", Resposta: "{}", Modelo: "m",
		})
		if err != nil {
			t.Fatalf("capturaInserir #%d: %v", i, err)
		}
		ultimoID = id
	}

	removidos, err := capturaPodar(ctx, s.db, 3)
	if err != nil {
		t.Fatalf("capturaPodar: %v", err)
	}
	if removidos != 7 {
		t.Errorf("removidos = %d, want 7", removidos)
	}

	if _, err := capturaBuscar(ctx, s.db, ultimoID); err != nil {
		t.Fatalf("a captura mais recente deveria ter sobrado: %v", err)
	}
	if _, err := capturaBuscar(ctx, s.db, ultimoID-9); !isErrNaoEncontrado(err) {
		t.Fatalf("a captura mais antiga deveria ter sido podada: err = %v", err)
	}

	// podar de novo com menos linhas que o limite não remove nada.
	removidos2, err := capturaPodar(ctx, s.db, 500)
	if err != nil {
		t.Fatalf("capturaPodar (2): %v", err)
	}
	if removidos2 != 0 {
		t.Errorf("removidos2 = %d, want 0", removidos2)
	}
}
