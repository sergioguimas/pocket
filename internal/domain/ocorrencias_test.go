package domain

import (
	"testing"
	"time"
)

// item60 monta um Item ativo, criado em criadoEm, com a regra dada — usado
// pelos testes de 60 dias abaixo.
func item60(regra Regra, criadoEm Dia) Item {
	t, _ := criadoEm.Time()
	return Item{ID: 1, Tipo: TipoHabito, Titulo: "x", Regra: &regra, Estado: EstadoAtivo, CriadoEm: t}
}

// rodar60Dias simula o job diário (gerar + fechar) chamado dia a dia, de
// criadoEm até criadoEm+59 (60 dias, cobrindo a virada de mês/semana pedida
// pelo C14), acumulando as ocorrências geradas. Devolve o estado final.
func rodar60Dias(t *testing.T, item Item, criadoEm Dia) []Ocorrencia {
	t.Helper()
	var todas []Ocorrencia
	dia := criadoEm
	for i := 0; i < 60; i++ {
		novas := GerarOcorrencias(item, todas, dia)
		todas = append(todas, novas...)
		vencidas := FecharVencidas(item, todas, dia)
		todas = aplicarStatus(todas, vencidas)
		dia = dia.AddDias(1)
	}
	return todas
}

// aplicarStatus substitui em todas as ocorrências que aparecem (por Inicio)
// em atualizadas — simula o "persistir as mudanças" que o store faria.
func aplicarStatus(todas, atualizadas []Ocorrencia) []Ocorrencia {
	for _, upd := range atualizadas {
		for i := range todas {
			if todas[i].Inicio != nil && upd.Inicio != nil && *todas[i].Inicio == *upd.Inicio {
				todas[i] = upd
			}
		}
	}
	return todas
}

func contarInicios(ocorrencias []Ocorrencia) map[Dia]int {
	m := map[Dia]int{}
	for _, oc := range ocorrencias {
		if oc.Inicio != nil {
			m[*oc.Inicio]++
		}
	}
	return m
}

// TestGerarOcorrencias60DiasDiario cobre o formato "diario" por 60 dias a
// partir de 15/01 (atravessa fevereiro não-bissexto, março de 31, abril de
// 30) — uma ocorrência por dia, nunca duplicada.
func TestGerarOcorrencias60DiasDiario(t *testing.T) {
	criadoEm := Dia("2026-01-15")
	item := item60(Regra{Tipo: RegraDiario}, criadoEm)
	todas := rodar60Dias(t, item, criadoEm)

	if len(todas) != 60 {
		t.Fatalf("len(todas) = %d, esperava 60 (uma por dia)", len(todas))
	}
	inicios := contarInicios(todas)
	for dia, n := range inicios {
		if n != 1 {
			t.Errorf("dia %q apareceu %d vezes, esperava 1 (idempotência)", dia, n)
		}
	}
	// último dia do intervalo simulado: 15/01 + 59 = 15/03.
	if _, ok := inicios["2026-03-15"]; !ok {
		t.Errorf("faltou ocorrência do último dia (2026-03-15); inicios=%v", inicios)
	}
	// atravessou fev (28 dias em 2026) e o fim de janeiro (31 dias).
	for _, d := range []Dia{"2026-01-31", "2026-02-01", "2026-02-28", "2026-03-01"} {
		if _, ok := inicios[d]; !ok {
			t.Errorf("faltou ocorrência do dia %q (virada de mês)", d)
		}
	}
}

// TestGerarOcorrencias60DiasFevereiroBissexto cobre a virada de fev
// bissexto (29 dias em 2024) explicitamente pedida pelo C14.
func TestGerarOcorrencias60DiasFevereiroBissexto(t *testing.T) {
	criadoEm := Dia("2024-01-15") // 2024 é bissexto
	item := item60(Regra{Tipo: RegraDiario}, criadoEm)
	todas := rodar60Dias(t, item, criadoEm)
	inicios := contarInicios(todas)
	if _, ok := inicios["2024-02-29"]; !ok {
		t.Errorf("faltou 29/02 (bissexto); inicios=%v", inicios)
	}
	if len(todas) != 60 {
		t.Fatalf("len(todas) = %d, esperava 60", len(todas))
	}
}

// TestGerarOcorrencias60DiasDiasSemana cobre "dias_semana" (seg/qua/sex) por
// 60 dias, incluindo a virada de semana (decisão A1: segunda a domingo).
func TestGerarOcorrencias60DiasDiasSemana(t *testing.T) {
	criadoEm := Dia("2026-01-15") // uma quinta-feira
	item := item60(Regra{Tipo: RegraDiasSemana, Dias: []int{1, 3, 5}}, criadoEm)
	todas := rodar60Dias(t, item, criadoEm)

	for _, oc := range todas {
		if oc.Inicio == nil {
			t.Fatalf("ocorrência sem Inicio: %+v", oc)
		}
		wd := DiaSemana(*oc.Inicio)
		if wd != 1 && wd != 3 && wd != 5 {
			t.Errorf("ocorrência em %q (dia da semana %d) não deveria existir", *oc.Inicio, wd)
		}
	}
	// idempotência: rodar de novo sobre o resultado não deve criar nada novo.
	novas := GerarOcorrencias(item, todas, criadoEm.AddDias(59))
	if len(novas) != 0 {
		t.Errorf("segunda chamada no mesmo dia gerou %d novas, esperava 0", len(novas))
	}
}

// TestGerarOcorrenciasVezesPeriodoSemana60Dias cobre "vezes_periodo" semanal
// por 60 dias: uma ocorrência por semana, meta truncada na primeira semana
// (decisão 3), cheia nas seguintes.
func TestGerarOcorrenciasVezesPeriodoSemana60Dias(t *testing.T) {
	criadoEm := Dia("2026-01-15") // quinta — semana trunca a partir daí
	item := item60(Regra{Tipo: RegraVezesPeriodo, Vezes: 3, Periodo: PeriodoSemana}, criadoEm)
	todas := rodar60Dias(t, item, criadoEm)

	// não pode haver duas ocorrências com o mesmo Inicio (idempotência).
	inicios := contarInicios(todas)
	for dia, n := range inicios {
		if n != 1 {
			t.Errorf("Inicio %q apareceu %d vezes", dia, n)
		}
	}

	primeira, ok := OcorrenciaDoDia(todas, criadoEm)
	if !ok {
		t.Fatalf("nenhuma ocorrência cobre o dia de criação")
	}
	if *primeira.Inicio != criadoEm {
		t.Errorf("primeira ocorrência: Inicio = %q, esperava %q (dia de criação, período truncado)", *primeira.Inicio, criadoEm)
	}
	if *primeira.Fim != FimSemana(criadoEm) {
		t.Errorf("primeira ocorrência: Fim = %q, esperava %q (fim da semana)", *primeira.Fim, FimSemana(criadoEm))
	}
	// semana de qui a dom = 4 dias restantes (qui,sex,sab,dom); meta = min(3,4) = 3
	if primeira.Meta != 3 {
		t.Errorf("primeira ocorrência: Meta = %d, esperava 3 (min(vezes, dias restantes))", primeira.Meta)
	}

	// segunda semana: período cheio, meta = vezes = 3.
	segundaSemanaInicio := InicioSemana(criadoEm.AddDias(7))
	segunda, ok := OcorrenciaDoDia(todas, segundaSemanaInicio)
	if !ok {
		t.Fatalf("nenhuma ocorrência cobre o início da segunda semana (%q)", segundaSemanaInicio)
	}
	if *segunda.Inicio != segundaSemanaInicio {
		t.Errorf("segunda ocorrência: Inicio = %q, esperava %q (período cheio)", *segunda.Inicio, segundaSemanaInicio)
	}
	if segunda.Meta != 3 {
		t.Errorf("segunda ocorrência: Meta = %d, esperava 3", segunda.Meta)
	}
}

// TestGerarOcorrenciasVezesPeriodoMes60Dias cobre "vezes_periodo" mensal
// atravessando a virada de fev (28 dias) para março (31 dias).
func TestGerarOcorrenciasVezesPeriodoMes60Dias(t *testing.T) {
	criadoEm := Dia("2026-01-15")
	item := item60(Regra{Tipo: RegraVezesPeriodo, Vezes: 10, Periodo: PeriodoMes}, criadoEm)
	todas := rodar60Dias(t, item, criadoEm)

	primeira, ok := OcorrenciaDoDia(todas, criadoEm)
	if !ok || *primeira.Inicio != criadoEm || *primeira.Fim != "2026-01-31" {
		t.Fatalf("primeira ocorrência mensal errada: %+v", primeira)
	}
	// dias restantes de 15/01 a 31/01 = 17; meta = min(10,17) = 10
	if primeira.Meta != 10 {
		t.Errorf("primeira ocorrência: Meta = %d, esperava 10", primeira.Meta)
	}

	fevereiro, ok := OcorrenciaDoDia(todas, Dia("2026-02-01"))
	if !ok || *fevereiro.Inicio != "2026-02-01" || *fevereiro.Fim != "2026-02-28" {
		t.Fatalf("ocorrência de fevereiro errada: %+v", fevereiro)
	}
	if fevereiro.Meta != 10 {
		t.Errorf("ocorrência de fevereiro: Meta = %d, esperava 10 (período cheio)", fevereiro.Meta)
	}

	// a simulação só chega até 15/03, mas o período de março (calendário
	// inteiro) já existe assim que o job roda em qualquer dia dele — a
	// simulação não limita o Fim, só até onde ela chamou GerarOcorrencias.
	marco, ok := OcorrenciaDoDia(todas, Dia("2026-03-01"))
	if !ok || *marco.Inicio != "2026-03-01" || *marco.Fim != "2026-03-31" {
		t.Fatalf("ocorrência de março errada: %+v", marco)
	}
	if marco.Meta != 10 {
		t.Errorf("ocorrência de março: Meta = %d, esperava 10 (período cheio)", marco.Meta)
	}
}

// TestGerarOcorrenciasIntervalo60Dias cobre "intervalo": só a primeira
// ocorrência nasce por GerarOcorrencias (decisão 4); nenhuma outra aparece
// mesmo rodando 60 dias, porque as seguintes só nascem via
// RegistrarIntervalo (etapa 3 aciona isso; aqui não há registro nenhum).
func TestGerarOcorrenciasIntervalo60Dias(t *testing.T) {
	criadoEm := Dia("2026-01-15")
	item := item60(Regra{Tipo: RegraIntervalo, Intervalo: 3}, criadoEm)
	todas := rodar60Dias(t, item, criadoEm)

	if len(todas) != 1 {
		t.Fatalf("len(todas) = %d, esperava 1 (só a inicial)", len(todas))
	}
	if *todas[0].Inicio != criadoEm {
		t.Errorf("Inicio = %q, esperava %q (dia de criação)", *todas[0].Inicio, criadoEm)
	}
	if todas[0].Fim != nil {
		t.Errorf("Fim = %q, esperava nil (aberta até ser feita)", *todas[0].Fim)
	}
	if todas[0].Status != StatusAberta {
		t.Errorf("Status = %q, esperava aberta", todas[0].Status)
	}
	// intervalo nunca vira perdida.
	vencidas := FecharVencidas(item, todas, criadoEm.AddDias(59))
	if len(vencidas) != 0 {
		t.Errorf("FecharVencidas devolveu %d, esperava 0 (intervalo nunca perde)", len(vencidas))
	}
}

func TestGerarOcorrenciasCompromisso(t *testing.T) {
	criadoEm, _ := time.Parse(layoutDia, "2026-09-26")
	data := Dia("2026-10-14")

	comData := Item{ID: 1, Tipo: TipoCompromisso, Estado: EstadoAtivo, CriadoEm: criadoEm, Data: &data}
	novas := GerarOcorrencias(comData, nil, Dia("2026-09-26"))
	if len(novas) != 1 || novas[0].Inicio == nil || *novas[0].Inicio != data || novas[0].Fim == nil || *novas[0].Fim != data {
		t.Fatalf("compromisso com data: novas = %+v", novas)
	}
	// idempotente: já existe, não gera de novo.
	if got := GerarOcorrencias(comData, novas, Dia("2026-09-27")); len(got) != 0 {
		t.Errorf("segunda chamada gerou %d, esperava 0", len(got))
	}

	semData := Item{ID: 2, Tipo: TipoCompromisso, Estado: EstadoAtivo, CriadoEm: criadoEm}
	novas2 := GerarOcorrencias(semData, nil, Dia("2026-09-26"))
	if len(novas2) != 1 || novas2[0].Inicio != nil || novas2[0].Fim != nil {
		t.Fatalf("compromisso sem data: novas = %+v", novas2)
	}
}

// TestGerarOcorrenciasPausaEArquivado cobre "item não-ativo não gera".
func TestGerarOcorrenciasPausaEArquivado(t *testing.T) {
	criadoEm, _ := time.Parse(layoutDia, "2026-09-01")
	regra := Regra{Tipo: RegraDiario}
	for _, estado := range []string{EstadoPausado, EstadoArquivado, EstadoConcluido} {
		item := Item{ID: 1, Tipo: TipoHabito, Regra: &regra, Estado: estado, CriadoEm: criadoEm}
		if got := GerarOcorrencias(item, nil, Dia("2026-09-10")); len(got) != 0 {
			t.Errorf("estado %q: gerou %d ocorrências, esperava 0", estado, len(got))
		}
	}
}

// TestGerarOcorrenciasNadaAntesDaCriacao garante que hoje < criadoEm não gera.
func TestGerarOcorrenciasNadaAntesDaCriacao(t *testing.T) {
	criadoEm, _ := time.Parse(layoutDia, "2026-09-10")
	regra := Regra{Tipo: RegraDiario}
	item := Item{ID: 1, Tipo: TipoHabito, Regra: &regra, Estado: EstadoAtivo, CriadoEm: criadoEm}
	if got := GerarOcorrencias(item, nil, Dia("2026-09-09")); len(got) != 0 {
		t.Errorf("gerou %d ocorrências para dia antes da criação, esperava 0", len(got))
	}
}

// TestGerarOcorrenciasPausaNoMeioNaoGeraNemPerde simula uma pausa real:
// hábito diário criado, gera alguns dias, é pausado, alguns dias sem
// ocorrência (neutro — decisão 2), depois reativado e volta a gerar.
func TestGerarOcorrenciasPausaNoMeioNaoGeraNemPerde(t *testing.T) {
	criadoEmDia := Dia("2026-09-01")
	criadoEm, _ := criadoEmDia.Time()
	regra := Regra{Tipo: RegraDiario}
	item := Item{ID: 1, Tipo: TipoHabito, Regra: &regra, Estado: EstadoAtivo, CriadoEm: criadoEm}

	var todas []Ocorrencia
	dia := criadoEmDia
	for i := 0; i < 3; i++ { // 01, 02, 03
		todas = append(todas, GerarOcorrencias(item, todas, dia)...)
		dia = dia.AddDias(1)
	}
	if len(todas) != 3 {
		t.Fatalf("antes da pausa: len(todas) = %d, esperava 3", len(todas))
	}

	item.Estado = EstadoPausado
	for i := 0; i < 3; i++ { // 04, 05, 06 — pausado, não gera
		todas = append(todas, GerarOcorrencias(item, todas, dia)...)
		dia = dia.AddDias(1)
	}
	if len(todas) != 3 {
		t.Fatalf("durante a pausa: len(todas) = %d, esperava continuar 3", len(todas))
	}

	item.Estado = EstadoAtivo
	todas = append(todas, GerarOcorrencias(item, todas, dia)...) // 07 — volta a gerar
	if len(todas) != 4 {
		t.Fatalf("depois da pausa: len(todas) = %d, esperava 4", len(todas))
	}
	inicios := contarInicios(todas)
	if _, ok := inicios["2026-09-07"]; !ok {
		t.Errorf("não gerou o dia da reativação (07/09); inicios=%v", inicios)
	}
	for _, dPausa := range []Dia{"2026-09-04", "2026-09-05", "2026-09-06"} {
		if _, ok := inicios[dPausa]; ok {
			t.Errorf("gerou ocorrência durante a pausa (%q), esperava nenhuma", dPausa)
		}
	}

	// os dias pausados nunca tiveram ocorrência — FecharVencidas não teria
	// nada pra fechar sobre eles, e não devem quebrar a sequência (são
	// simplesmente ausentes da lista).
	vencidas := FecharVencidas(item, todas, Dia("2026-09-08"))
	for _, v := range vencidas {
		if *v.Inicio == "2026-09-04" || *v.Inicio == "2026-09-05" || *v.Inicio == "2026-09-06" {
			t.Errorf("dia pausado %q não deveria existir para virar perdida", *v.Inicio)
		}
	}
}

func TestFecharVencidas(t *testing.T) {
	regra := Regra{Tipo: RegraDiario}
	item := Item{ID: 1, Tipo: TipoHabito, Regra: &regra, Estado: EstadoAtivo}

	ontem := Dia("2026-09-25")
	hoje := Dia("2026-09-26")
	oc := Ocorrencia{ID: 1, Inicio: &ontem, Fim: &ontem, Meta: 1, Status: StatusAberta}

	vencidas := FecharVencidas(item, []Ocorrencia{oc}, hoje)
	if len(vencidas) != 1 || vencidas[0].Status != StatusPerdida {
		t.Fatalf("FecharVencidas = %+v, esperava 1 perdida", vencidas)
	}

	// a de hoje (Fim == hoje) não vence ainda.
	ocHoje := Ocorrencia{ID: 2, Inicio: &hoje, Fim: &hoje, Meta: 1, Status: StatusAberta}
	if got := FecharVencidas(item, []Ocorrencia{ocHoje}, hoje); len(got) != 0 {
		t.Errorf("ocorrência de hoje não deveria vencer ainda: %+v", got)
	}

	// pulada não vence de novo (já não está aberta).
	ocPulada := Ocorrencia{ID: 3, Inicio: &ontem, Fim: &ontem, Meta: 1, Status: StatusPulada}
	if got := FecharVencidas(item, []Ocorrencia{ocPulada}, hoje); len(got) != 0 {
		t.Errorf("pulada não deveria reaparecer em FecharVencidas: %+v", got)
	}

	// intervalo nunca vence.
	regraIntervalo := Regra{Tipo: RegraIntervalo, Intervalo: 3}
	itemIntervalo := Item{ID: 2, Tipo: TipoHabito, Regra: &regraIntervalo, Estado: EstadoAtivo}
	ocIntervaloVencida := Ocorrencia{ID: 4, Inicio: &ontem, Fim: nil, Meta: 1, Status: StatusAberta}
	if got := FecharVencidas(itemIntervalo, []Ocorrencia{ocIntervaloVencida}, hoje.AddDias(30)); len(got) != 0 {
		t.Errorf("intervalo nunca deveria virar perdida: %+v", got)
	}

	// compromisso nunca vence.
	compromisso := Item{ID: 3, Tipo: TipoCompromisso, Estado: EstadoAtivo}
	ocCompromisso := Ocorrencia{ID: 5, Inicio: &ontem, Fim: &ontem, Meta: 1, Status: StatusAberta}
	if got := FecharVencidas(compromisso, []Ocorrencia{ocCompromisso}, hoje.AddDias(30)); len(got) != 0 {
		t.Errorf("compromisso nunca deveria virar perdida: %+v", got)
	}
}

func TestStatusApos(t *testing.T) {
	regra := Regra{Tipo: RegraDiario}
	habito := Item{Tipo: TipoHabito, Regra: &regra}
	compromisso := Item{Tipo: TipoCompromisso}
	regraIntervalo := Regra{Tipo: RegraIntervalo, Intervalo: 3}
	intervalo := Item{Tipo: TipoHabito, Regra: &regraIntervalo}

	ontem := Dia("2026-09-25")
	hoje := Dia("2026-09-26")

	casos := []struct {
		nome       string
		item       Item
		oc         Ocorrencia
		nRegistros int
		hoje       Dia
		want       string
	}{
		{"cumpre a meta", habito, Ocorrencia{Meta: 1, Status: StatusAberta, Fim: &hoje}, 1, hoje, StatusCumprida},
		{"cumpre acima da meta", habito, Ocorrencia{Meta: 1, Status: StatusAberta, Fim: &hoje}, 5, hoje, StatusCumprida},
		{"pulada continua pulada sem registro", habito, Ocorrencia{Meta: 1, Status: StatusPulada, Fim: &ontem}, 0, hoje, StatusPulada},
		{"vencida vira perdida", habito, Ocorrencia{Meta: 1, Status: StatusAberta, Fim: &ontem}, 0, hoje, StatusPerdida},
		{"nao vencida fica aberta", habito, Ocorrencia{Meta: 1, Status: StatusAberta, Fim: &hoje}, 0, hoje, StatusAberta},
		{"intervalo vencido nao perde", intervalo, Ocorrencia{Meta: 1, Status: StatusAberta, Fim: &ontem}, 0, hoje, StatusAberta},
		{"compromisso vencido nao perde", compromisso, Ocorrencia{Meta: 1, Status: StatusAberta, Fim: &ontem}, 0, hoje, StatusAberta},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got := StatusApos(c.item, c.oc, c.nRegistros, c.hoje)
			if got != c.want {
				t.Errorf("StatusApos() = %q, esperava %q", got, c.want)
			}
		})
	}
}

func TestDespular(t *testing.T) {
	regra := Regra{Tipo: RegraDiario}
	item := Item{Tipo: TipoHabito, Regra: &regra}
	ontem := Dia("2026-09-25")
	hoje := Dia("2026-09-26")

	// pulada, vencida, sem registros -> despular recalcula como se fosse
	// aberta -> vira perdida (porque já passou o Fim).
	pulada := Ocorrencia{Meta: 1, Status: StatusPulada, Fim: &ontem}
	if got := Despular(item, pulada, 0, hoje); got != StatusPerdida {
		t.Errorf("Despular(vencida) = %q, esperava perdida", got)
	}

	// pulada, ainda não vencida -> despular -> aberta.
	puladaHoje := Ocorrencia{Meta: 1, Status: StatusPulada, Fim: &hoje}
	if got := Despular(item, puladaHoje, 0, hoje); got != StatusAberta {
		t.Errorf("Despular(não vencida) = %q, esperava aberta", got)
	}

	// pulada com registro suficiente -> despular -> cumprida.
	if got := Despular(item, pulada, 1, hoje); got != StatusCumprida {
		t.Errorf("Despular(com registro) = %q, esperava cumprida", got)
	}
}

func TestOcorrenciaDoDia(t *testing.T) {
	d1 := Dia("2026-09-01")
	d7 := Dia("2026-09-07")
	oc := Ocorrencia{ID: 1, Inicio: &d1, Fim: &d7, Meta: 3}

	if got, ok := OcorrenciaDoDia([]Ocorrencia{oc}, Dia("2026-09-04")); !ok || got.ID != 1 {
		t.Errorf("dia dentro do range não achou a ocorrência: %+v, %v", got, ok)
	}
	if _, ok := OcorrenciaDoDia([]Ocorrencia{oc}, Dia("2026-08-31")); ok {
		t.Errorf("dia antes do Inicio não deveria achar nada")
	}
	if _, ok := OcorrenciaDoDia([]Ocorrencia{oc}, Dia("2026-09-08")); ok {
		t.Errorf("dia depois do Fim não deveria achar nada (registro extra)")
	}

	// dias_semana: dia sem ocorrência (ex.: terça não marcada) -> extra.
	seg := Dia("2026-09-07")
	ocSeg := Ocorrencia{ID: 2, Inicio: &seg, Fim: &seg, Meta: 1}
	if _, ok := OcorrenciaDoDia([]Ocorrencia{ocSeg}, Dia("2026-09-08")); ok {
		t.Errorf("terça sem ocorrência não deveria achar nada (regra 6: registro extra)")
	}
}

func TestRegistrarEDesfazerIntervalo(t *testing.T) {
	inicio := Dia("2026-09-10")
	aberta := Ocorrencia{ID: 1, ItemID: 9, Inicio: &inicio, Fim: nil, Meta: 1, Status: StatusAberta}

	dia := Dia("2026-09-12")
	fechada, proxima, criou := RegistrarIntervalo([]Ocorrencia{aberta}, dia, 3)
	if !criou {
		t.Fatalf("esperava criar próxima")
	}
	if fechada.Status != StatusCumprida || fechada.Fim == nil || *fechada.Fim != dia {
		t.Errorf("fechada = %+v, esperava cumprida com Fim = %q", fechada, dia)
	}
	wantProximoInicio := Dia("2026-09-15")
	if proxima.Inicio == nil || *proxima.Inicio != wantProximoInicio {
		t.Errorf("proxima.Inicio = %v, esperava %q", proxima.Inicio, wantProximoInicio)
	}
	if proxima.Fim != nil || proxima.Status != StatusAberta || proxima.Meta != 1 {
		t.Errorf("proxima = %+v, esperava aberta/Fim nil/Meta 1", proxima)
	}

	// registro antecipado: dia < inicio da aberta também fecha (decisão 4).
	diaAntecipado := Dia("2026-09-08") // antes de inicio (09-10)
	fechada2, proxima2, criou2 := RegistrarIntervalo([]Ocorrencia{aberta}, diaAntecipado, 3)
	if !criou2 || fechada2.Status != StatusCumprida || *fechada2.Fim != diaAntecipado {
		t.Errorf("registro antecipado não fechou como esperado: %+v", fechada2)
	}
	if *proxima2.Inicio != diaAntecipado.AddDias(3) {
		t.Errorf("proxima2.Inicio = %v, esperava %q", proxima2.Inicio, diaAntecipado.AddDias(3))
	}

	// segundo registro no mesmo dia (já cumprida) é extra: nada muda.
	if _, _, criouExtra := RegistrarIntervalo([]Ocorrencia{fechada, proxima}, dia, 3); criouExtra {
		t.Errorf("registro extra no mesmo dia não deveria fechar a próxima")
	}
	// retroativo anterior à última vez feita também é extra.
	if _, _, criouRetro := RegistrarIntervalo([]Ocorrencia{fechada, proxima}, dia.AddDias(-1), 3); criouRetro {
		t.Errorf("registro retroativo antes da última vez feita deveria ser extra")
	}
	// dia seguinte: fecha a próxima normalmente.
	f3, p3, criou3 := RegistrarIntervalo([]Ocorrencia{fechada, proxima}, dia.AddDias(1), 3)
	if !criou3 || *f3.Inicio != *proxima.Inicio || *p3.Inicio != dia.AddDias(4) {
		t.Errorf("registro no dia seguinte: fechada=%+v proxima=%+v", f3, p3)
	}

	// desfazer: volta a aberta, Fim nulo.
	reaberta := DesfazerRegistroIntervalo(fechada)
	if reaberta.Status != StatusAberta || reaberta.Fim != nil {
		t.Errorf("reaberta = %+v, esperava aberta/Fim nil", reaberta)
	}
	if reaberta.Inicio == nil || *reaberta.Inicio != inicio {
		t.Errorf("reaberta.Inicio mudou: %v, esperava %q", reaberta.Inicio, inicio)
	}
}

// Regra que muda para intervalo (ou N que muda) apaga a aberta; a nova
// nasce depois da última vez feita, sem colidir com inicio existente.
func TestGerarIntervaloSemAbertaComHistorico(t *testing.T) {
	criadoEm, _ := time.Parse(layoutDia, "2026-09-01")
	regra := Regra{Tipo: RegraIntervalo, Intervalo: 3}
	item := Item{ID: 1, Tipo: TipoHabito, Regra: &regra, Estado: EstadoAtivo, CriadoEm: criadoEm}
	d := func(s string) *Dia { x := Dia(s); return &x }
	casos := []struct {
		nome       string
		existentes []Ocorrencia
		want       Dia
	}{
		{"diarias antigas", []Ocorrencia{
			{Inicio: d("2026-09-08"), Fim: d("2026-09-08"), Status: StatusCumprida},
			{Inicio: d("2026-09-09"), Fim: d("2026-09-09"), Status: StatusPerdida},
		}, "2026-09-11"},
		{"perdida depois da cumprida", []Ocorrencia{
			{Inicio: d("2026-09-07"), Fim: d("2026-09-07"), Status: StatusCumprida},
			{Inicio: d("2026-09-10"), Fim: d("2026-09-10"), Status: StatusCumprida},
			{Inicio: d("2026-09-12"), Fim: d("2026-09-12"), Status: StatusPerdida},
		}, "2026-09-13"},
		{"com aberta", []Ocorrencia{{Inicio: d("2026-09-10"), Status: StatusAberta}}, ""},
	}
	for _, c := range casos {
		got := GerarOcorrencias(item, c.existentes, Dia("2026-09-12"))
		if c.want == "" {
			if len(got) != 0 {
				t.Errorf("%s: gerou %d, esperava 0", c.nome, len(got))
			}
			continue
		}
		if len(got) != 1 || *got[0].Inicio != c.want || got[0].Fim != nil {
			t.Errorf("%s: got %+v, esperava inicio %s", c.nome, got, c.want)
		}
	}
}
