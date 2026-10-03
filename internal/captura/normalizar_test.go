package captura

import (
	"testing"
	"time"

	"pocket/internal/domain"
)

func locTeste(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	return loc
}

// segunda28 (2026-09-28) é a mesma data fixa usada no resto do repo.
func hojeAgora(t *testing.T) (domain.Dia, time.Time, *time.Location) {
	loc := locTeste(t)
	agora := time.Date(2026, 9, 28, 10, 0, 0, 0, loc)
	return domain.Hoje(fakeClockAt(agora), loc), agora, loc
}

// fakeClockAt satisfaz clock.Clock com um instante fixo, só para computar
// domain.Hoje sem importar internal/clock (evitar dependência circular de
// pacote de teste — mais simples fazer aqui).
type fakeClockAt time.Time

func (f fakeClockAt) Now() time.Time { return time.Time(f) }

func TestNormalizarJSONMalformado(t *testing.T) {
	hoje, agora, loc := hojeAgora(t)
	r := Normalizar(Contexto{}, []byte(`{isso não é json`), hoje, agora, loc)
	if r.Acao != AcaoDesconhecido {
		t.Fatalf("Acao = %q, want desconhecido", r.Acao)
	}
	if len(r.Avisos) == 0 {
		t.Fatal("esperava algum aviso")
	}
}

func TestNormalizarAcaoDesconhecidaExplicita(t *testing.T) {
	hoje, agora, loc := hojeAgora(t)
	r := Normalizar(Contexto{}, []byte(`{"acao":"desconhecido","item":{"tipo":null,"titulo":null,"notas":null,"regra_tipo":null,"regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":[]},"registro":{"item_id":null,"titulo":null,"dia":null,"nota":null},"compras":{"lista_nome":null,"itens":[]},"resto":null}`), hoje, agora, loc)
	if r.Acao != AcaoDesconhecido {
		t.Fatalf("Acao = %q, want desconhecido", r.Acao)
	}
}

// --- criar_item ---

func TestNormalizarCriarItemHabitoDiario(t *testing.T) {
	hoje, agora, loc := hojeAgora(t)
	proposta := `{"acao":"criar_item","item":{"tipo":"habito","titulo":"Ler","notas":null,"regra_tipo":"diario","regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":["21:00"]},"registro":{"item_id":null,"titulo":null,"dia":null,"nota":null},"compras":{"lista_nome":null,"itens":[]},"resto":null}`
	r := Normalizar(Contexto{}, []byte(proposta), hoje, agora, loc)
	if r.Acao != AcaoCriarItem {
		t.Fatalf("Acao = %q, want criar_item", r.Acao)
	}
	if r.Item == nil || r.Item.Tipo != domain.TipoHabito || r.Item.Titulo != "Ler" {
		t.Fatalf("Item = %+v", r.Item)
	}
	if len(r.Item.Regra) == 0 {
		t.Fatal("esperava regra preenchida")
	}
	if _, err := domain.ParseRegra(r.Item.Regra); err != nil {
		t.Fatalf("regra inválida: %v", err)
	}
	if len(r.Item.Horarios) != 1 || r.Item.Horarios[0] != "21:00" {
		t.Fatalf("Horarios = %v", r.Item.Horarios)
	}
	if len(r.Avisos) != 0 {
		t.Fatalf("avisos inesperados: %v", r.Avisos)
	}
}

func TestNormalizarCriarItemHabitoVezesPeriodo(t *testing.T) {
	hoje, agora, loc := hojeAgora(t)
	proposta := `{"acao":"criar_item","item":{"tipo":"habito","titulo":"Academia","notas":null,"regra_tipo":"vezes_periodo","regra_dias":[],"regra_vezes":3,"regra_periodo":"semana","regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":[]},"registro":{"item_id":null,"titulo":null,"dia":null,"nota":null},"compras":{"lista_nome":null,"itens":[]},"resto":null}`
	r := Normalizar(Contexto{}, []byte(proposta), hoje, agora, loc)
	if r.Acao != AcaoCriarItem || r.Item == nil {
		t.Fatalf("resposta = %+v", r)
	}
	regra, err := domain.ParseRegra(r.Item.Regra)
	if err != nil {
		t.Fatalf("ParseRegra: %v", err)
	}
	if regra.Tipo != domain.RegraVezesPeriodo || regra.Vezes != 3 || regra.Periodo != "semana" {
		t.Fatalf("regra = %+v", regra)
	}
}

func TestNormalizarCriarItemCompromissoComData(t *testing.T) {
	hoje, agora, loc := hojeAgora(t)
	proposta := `{"acao":"criar_item","item":{"tipo":"compromisso","titulo":"Dentista","notas":null,"regra_tipo":null,"regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":"2026-10-14","hora":"15:00","horarios":["-1d"]},"registro":{"item_id":null,"titulo":null,"dia":null,"nota":null},"compras":{"lista_nome":null,"itens":[]},"resto":null}`
	r := Normalizar(Contexto{}, []byte(proposta), hoje, agora, loc)
	if r.Acao != AcaoCriarItem || r.Item == nil {
		t.Fatalf("resposta = %+v", r)
	}
	if r.Item.Data != "2026-10-14" || r.Item.Hora != "15:00" {
		t.Fatalf("Item = %+v", r.Item)
	}
	if len(r.Item.Horarios) != 1 || r.Item.Horarios[0] != "-1d" {
		t.Fatalf("Horarios = %v", r.Item.Horarios)
	}
	if len(r.Avisos) != 0 {
		t.Fatalf("avisos inesperados: %v", r.Avisos)
	}
}

func TestNormalizarCriarItemRegraInvalidaViraAviso(t *testing.T) {
	hoje, agora, loc := hojeAgora(t)
	proposta := `{"acao":"criar_item","item":{"tipo":"habito","titulo":"Ler","notas":null,"regra_tipo":"dias_semana","regra_dias":[9],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":[]},"registro":{"item_id":null,"titulo":null,"dia":null,"nota":null},"compras":{"lista_nome":null,"itens":[]},"resto":null}`
	r := Normalizar(Contexto{}, []byte(proposta), hoje, agora, loc)
	if r.Acao != AcaoCriarItem || r.Item == nil {
		t.Fatalf("resposta = %+v", r)
	}
	if len(r.Item.Regra) != 0 {
		t.Fatalf("regra deveria ter sido descartada: %s", r.Item.Regra)
	}
	if len(r.Avisos) == 0 {
		t.Fatal("esperava aviso sobre a frequência")
	}
}

func TestNormalizarCriarItemSemTituloViraDesconhecido(t *testing.T) {
	hoje, agora, loc := hojeAgora(t)
	proposta := `{"acao":"criar_item","item":{"tipo":"habito","titulo":null,"notas":null,"regra_tipo":"diario","regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":[]},"registro":{"item_id":null,"titulo":null,"dia":null,"nota":null},"compras":{"lista_nome":null,"itens":[]},"resto":null}`
	r := Normalizar(Contexto{}, []byte(proposta), hoje, agora, loc)
	if r.Acao != AcaoDesconhecido {
		t.Fatalf("Acao = %q, want desconhecido", r.Acao)
	}
}

func TestNormalizarCompromissoHoraSemDataIgnorada(t *testing.T) {
	hoje, agora, loc := hojeAgora(t)
	proposta := `{"acao":"criar_item","item":{"tipo":"compromisso","titulo":"Ligar","notas":null,"regra_tipo":null,"regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":"15:00","horarios":[]},"registro":{"item_id":null,"titulo":null,"dia":null,"nota":null},"compras":{"lista_nome":null,"itens":[]},"resto":null}`
	r := Normalizar(Contexto{}, []byte(proposta), hoje, agora, loc)
	if r.Acao != AcaoCriarItem || r.Item == nil {
		t.Fatalf("resposta = %+v", r)
	}
	if r.Item.Hora != "" {
		t.Fatalf("Hora = %q, want vazio (hora sem data é descartada)", r.Item.Hora)
	}
	if len(r.Avisos) == 0 {
		t.Fatal("esperava aviso")
	}
}

// --- registrar ---

func TestNormalizarRegistrarItemExistente(t *testing.T) {
	hoje, agora, loc := hojeAgora(t)
	ctxInfo := Contexto{Habitos: []ContextoHabito{{ID: 3, Titulo: "Devocional"}}}
	proposta := `{"acao":"registrar","item":{"tipo":null,"titulo":null,"notas":null,"regra_tipo":null,"regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":[]},"registro":{"item_id":3,"titulo":"Devocional","dia":null,"nota":"Romanos 8"},"compras":{"lista_nome":null,"itens":[]},"resto":null}`
	r := Normalizar(ctxInfo, []byte(proposta), hoje, agora, loc)
	if r.Acao != AcaoRegistrar || r.Registro == nil {
		t.Fatalf("resposta = %+v", r)
	}
	if r.Registro.ItemID != 3 || r.Registro.Titulo != "Devocional" || r.Registro.Tipo != domain.TipoHabito {
		t.Fatalf("Registro = %+v", r.Registro)
	}
	if r.Registro.Em != nil {
		t.Fatalf("Em = %v, want nil (hoje/agora)", *r.Registro.Em)
	}
	if r.Registro.Nota != "Romanos 8" {
		t.Fatalf("Nota = %q", r.Registro.Nota)
	}
}

func TestNormalizarRegistrarItemIDInexistenteViraCriarItem(t *testing.T) {
	hoje, agora, loc := hojeAgora(t)
	ctxInfo := Contexto{Habitos: []ContextoHabito{{ID: 3, Titulo: "Devocional"}}}
	proposta := `{"acao":"registrar","item":{"tipo":null,"titulo":null,"notas":null,"regra_tipo":null,"regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":[]},"registro":{"item_id":999,"titulo":"Ioga","dia":null,"nota":null},"compras":{"lista_nome":null,"itens":[]},"resto":null}`
	r := Normalizar(ctxInfo, []byte(proposta), hoje, agora, loc)
	if r.Acao != AcaoCriarItem || r.Item == nil {
		t.Fatalf("resposta = %+v", r)
	}
	if r.Item.Titulo != "Ioga" || r.Item.Tipo != domain.TipoHabito {
		t.Fatalf("Item = %+v, want hábito Ioga", r.Item)
	}
	if len(r.Avisos) == 0 {
		t.Fatal("esperava aviso")
	}
}

func TestNormalizarRegistrarDiaPassado(t *testing.T) {
	hoje, agora, loc := hojeAgora(t)
	ctxInfo := Contexto{Habitos: []ContextoHabito{{ID: 1, Titulo: "Academia"}}}
	proposta := `{"acao":"registrar","item":{"tipo":null,"titulo":null,"notas":null,"regra_tipo":null,"regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":[]},"registro":{"item_id":1,"titulo":"Academia","dia":"2026-09-27","nota":null},"compras":{"lista_nome":null,"itens":[]},"resto":null}`
	r := Normalizar(ctxInfo, []byte(proposta), hoje, agora, loc)
	if r.Acao != AcaoRegistrar || r.Registro == nil {
		t.Fatalf("resposta = %+v", r)
	}
	if r.Registro.Em == nil {
		t.Fatal("esperava Em preenchido (dia passado)")
	}
	if !strings_HasPrefix(*r.Registro.Em, "2026-09-27T12:00:00") {
		t.Fatalf("Em = %q, want prefixo 2026-09-27T12:00:00", *r.Registro.Em)
	}
}

func TestNormalizarRegistrarDiaFuturoViraAviso(t *testing.T) {
	hoje, agora, loc := hojeAgora(t)
	ctxInfo := Contexto{Habitos: []ContextoHabito{{ID: 1, Titulo: "Academia"}}}
	proposta := `{"acao":"registrar","item":{"tipo":null,"titulo":null,"notas":null,"regra_tipo":null,"regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":[]},"registro":{"item_id":1,"titulo":"Academia","dia":"2026-09-29","nota":null},"compras":{"lista_nome":null,"itens":[]},"resto":null}`
	r := Normalizar(ctxInfo, []byte(proposta), hoje, agora, loc)
	if r.Acao != AcaoRegistrar || r.Registro == nil {
		t.Fatalf("resposta = %+v", r)
	}
	if r.Registro.Em != nil {
		t.Fatalf("Em = %v, want nil (futuro recusado)", *r.Registro.Em)
	}
	if len(r.Avisos) == 0 {
		t.Fatal("esperava aviso de data futura")
	}
}

func TestNormalizarRegistrarNotaLonga(t *testing.T) {
	hoje, agora, loc := hojeAgora(t)
	ctxInfo := Contexto{Habitos: []ContextoHabito{{ID: 1, Titulo: "Ler"}}}
	notaLonga := ""
	for i := 0; i < 600; i++ {
		notaLonga += "a"
	}
	proposta := `{"acao":"registrar","item":{"tipo":null,"titulo":null,"notas":null,"regra_tipo":null,"regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":[]},"registro":{"item_id":1,"titulo":"Ler","dia":null,"nota":"` + notaLonga + `"},"compras":{"lista_nome":null,"itens":[]},"resto":null}`
	r := Normalizar(ctxInfo, []byte(proposta), hoje, agora, loc)
	if r.Registro == nil {
		t.Fatalf("resposta = %+v", r)
	}
	if len(r.Registro.Nota) != 500 {
		t.Fatalf("len(Nota) = %d, want 500", len(r.Registro.Nota))
	}
	if len(r.Avisos) == 0 {
		t.Fatal("esperava aviso de nota cortada")
	}
}

// --- compras ---

func TestNormalizarComprasListaCitada(t *testing.T) {
	hoje, agora, loc := hojeAgora(t)
	ctxInfo := Contexto{Listas: []ContextoLista{{ID: 1, Nome: "Mercado"}, {ID: 2, Nome: "Farmácia"}}}
	proposta := `{"acao":"compras","item":{"tipo":null,"titulo":null,"notas":null,"regra_tipo":null,"regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":[]},"registro":{"item_id":null,"titulo":null,"dia":null,"nota":null},"compras":{"lista_nome":"farmácia","itens":[{"nome":"dipirona","quantidade":null}]},"resto":null}`
	r := Normalizar(ctxInfo, []byte(proposta), hoje, agora, loc)
	if r.Acao != AcaoCompras || r.Compras == nil {
		t.Fatalf("resposta = %+v", r)
	}
	if r.Compras.ListaID != 2 || r.Compras.ListaNome != "Farmácia" {
		t.Fatalf("Compras = %+v", r.Compras)
	}
	if len(r.Compras.Itens) != 1 || r.Compras.Itens[0].Nome != "dipirona" {
		t.Fatalf("Itens = %+v", r.Compras.Itens)
	}
}

func TestNormalizarComprasSemListaCitadaUsaPrimeira(t *testing.T) {
	hoje, agora, loc := hojeAgora(t)
	ctxInfo := Contexto{Listas: []ContextoLista{{ID: 1, Nome: "Mercado"}}}
	proposta := `{"acao":"compras","item":{"tipo":null,"titulo":null,"notas":null,"regra_tipo":null,"regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":[]},"registro":{"item_id":null,"titulo":null,"dia":null,"nota":null},"compras":{"lista_nome":null,"itens":[{"nome":"leite","quantidade":null},{"nome":"pão","quantidade":null},{"nome":"arroz","quantidade":"2 kg"}]},"resto":null}`
	r := Normalizar(ctxInfo, []byte(proposta), hoje, agora, loc)
	if r.Acao != AcaoCompras || r.Compras == nil {
		t.Fatalf("resposta = %+v", r)
	}
	if r.Compras.ListaID != 1 {
		t.Fatalf("ListaID = %d, want 1", r.Compras.ListaID)
	}
	if len(r.Compras.Itens) != 3 {
		t.Fatalf("Itens = %+v", r.Compras.Itens)
	}
	if r.Compras.Itens[2].Quantidade != "2 kg" {
		t.Fatalf("Quantidade = %q", r.Compras.Itens[2].Quantidade)
	}
}

func TestNormalizarComprasSemListaNoContextoViraDesconhecido(t *testing.T) {
	hoje, agora, loc := hojeAgora(t)
	proposta := `{"acao":"compras","item":{"tipo":null,"titulo":null,"notas":null,"regra_tipo":null,"regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":[]},"registro":{"item_id":null,"titulo":null,"dia":null,"nota":null},"compras":{"lista_nome":null,"itens":[{"nome":"leite","quantidade":null}]},"resto":null}`
	r := Normalizar(Contexto{}, []byte(proposta), hoje, agora, loc)
	if r.Acao != AcaoDesconhecido {
		t.Fatalf("Acao = %q, want desconhecido", r.Acao)
	}
}

func TestNormalizarComprasMaisDe20ItensTrunca(t *testing.T) {
	hoje, agora, loc := hojeAgora(t)
	ctxInfo := Contexto{Listas: []ContextoLista{{ID: 1, Nome: "Mercado"}}}
	itensJSON := "["
	for i := 0; i < 25; i++ {
		if i > 0 {
			itensJSON += ","
		}
		itensJSON += `{"nome":"item` + itoa(i) + `","quantidade":null}`
	}
	itensJSON += "]"
	proposta := `{"acao":"compras","item":{"tipo":null,"titulo":null,"notas":null,"regra_tipo":null,"regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":null,"hora":null,"horarios":[]},"registro":{"item_id":null,"titulo":null,"dia":null,"nota":null},"compras":{"lista_nome":null,"itens":` + itensJSON + `},"resto":null}`
	r := Normalizar(ctxInfo, []byte(proposta), hoje, agora, loc)
	if r.Acao != AcaoCompras || r.Compras == nil {
		t.Fatalf("resposta = %+v", r)
	}
	if len(r.Compras.Itens) != 20 {
		t.Fatalf("len(Itens) = %d, want 20", len(r.Compras.Itens))
	}
	if len(r.Avisos) == 0 {
		t.Fatal("esperava aviso de truncamento")
	}
}

func TestNormalizarResto(t *testing.T) {
	hoje, agora, loc := hojeAgora(t)
	proposta := `{"acao":"criar_item","item":{"tipo":"compromisso","titulo":"Dentista","notas":null,"regra_tipo":null,"regra_dias":[],"regra_vezes":null,"regra_periodo":null,"regra_intervalo_dias":null,"dia":"2026-09-29","hora":null,"horarios":[]},"registro":{"item_id":null,"titulo":null,"dia":null,"nota":null},"compras":{"lista_nome":null,"itens":[]},"resto":"comprar leite"}`
	r := Normalizar(Contexto{}, []byte(proposta), hoje, agora, loc)
	if r.Resto == nil || *r.Resto != "comprar leite" {
		t.Fatalf("Resto = %v", r.Resto)
	}
}

// helpers pequenos para não puxar strconv/strings inteiros só para isso nos testes.

func strings_HasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
