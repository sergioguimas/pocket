package domain

// GerarOcorrencias cria as ocorrências que faltam para o dia/período
// corrente (hoje) de item, a partir das ocorrências já existentes dele
// (existentes) — nunca duplica um Inicio já presente (idempotente, é o que
// sustenta o UNIQUE(item_id, inicio) do schema).
//
// DECISÃO (Opus, decisão 2): geração = só o dia/período corrente, sem
// catch-up de dias passados (leitura literal de §3.3 regra 1: "cria as
// ocorrências do dia e do período corrente"). Se o serviço ficou fora um
// dia, aquele dia fica sem ocorrência: neutro, não é perdida nem quebra
// sequência — mesma lógica da pausa (o banco não guarda histórico de
// pausa). Item não-ativo não gera nada; nada antes do dia de criação do
// item.
func GerarOcorrencias(item Item, existentes []Ocorrencia, hoje Dia) []Ocorrencia {
	if !item.EstaAtivo() {
		return nil
	}

	criadoEm := DiaDeTime(item.CriadoEm)
	if hoje < criadoEm {
		return nil
	}

	var novas []Ocorrencia
	switch {
	case item.Tipo == TipoCompromisso:
		novas = gerarCompromisso(existentes, item.Data)

	case item.Regra == nil:
		// defensivo: hábito sem regra não deveria existir (CHECK do schema).
		return nil

	case item.Regra.Tipo == RegraIntervalo:
		novas = gerarIntervaloInicial(existentes, criadoEm, item.Regra.Intervalo)

	case item.Regra.Tipo == RegraDiario:
		novas = gerarDiaUnico(existentes, hoje)

	case item.Regra.Tipo == RegraDiasSemana:
		if !contemInt(item.Regra.Dias, DiaSemana(hoje)) {
			return nil
		}
		novas = gerarDiaUnico(existentes, hoje)

	case item.Regra.Tipo == RegraVezesPeriodo:
		novas = gerarVezesPeriodo(*item.Regra, existentes, hoje, criadoEm)
	}

	for i := range novas {
		novas[i].ItemID = item.ID
	}
	return novas
}

// gerarCompromisso cria a única ocorrência de um compromisso (§3.3: "inicio
// = fim = data do compromisso, ou ambos nulos se sem data"), na primeira
// vez que GerarOcorrencias é chamado para o item (existentes vazio).
func gerarCompromisso(existentes []Ocorrencia, data *Dia) []Ocorrencia {
	if len(existentes) > 0 {
		return nil
	}
	oc := Ocorrencia{Meta: 1, Status: StatusAberta}
	if data != nil {
		d := *data
		oc.Inicio = &d
		oc.Fim = &d
	}
	return []Ocorrencia{oc}
}

// gerarIntervaloInicial garante uma ocorrência aberta (fim nulo) para um
// hábito intervalo. Sem histórico, nasce no dia da criação, "vence hoje"
// (decisão A2/decisão 4). As próximas nascem via RegistrarIntervalo.
//
// DECISÃO: se já há histórico mas nenhuma aberta (a regra acabou de mudar
// para intervalo, ou o N mudou e a aberta foi refeita), a nova nasce em
// max(fim da última cumprida + N, maior inicio existente + 1) — nunca
// colide no UNIQUE(item_id, inicio).
func gerarIntervaloInicial(existentes []Ocorrencia, criadoEm Dia, n int) []Ocorrencia {
	if len(existentes) == 0 {
		inicio := criadoEm
		return []Ocorrencia{{Inicio: &inicio, Fim: nil, Meta: 1, Status: StatusAberta}}
	}
	inicio := criadoEm
	for _, oc := range existentes {
		if oc.Status == StatusAberta && oc.Fim == nil {
			return nil
		}
		if oc.Status == StatusCumprida && oc.Fim != nil && oc.Fim.AddDias(n) > inicio {
			inicio = oc.Fim.AddDias(n)
		}
		if oc.Inicio != nil && oc.Inicio.AddDias(1) > inicio {
			inicio = oc.Inicio.AddDias(1)
		}
	}
	return []Ocorrencia{{Inicio: &inicio, Fim: nil, Meta: 1, Status: StatusAberta}}
}

// gerarDiaUnico cria a ocorrência de um único dia (diario/dias_semana:
// Inicio = Fim = dia, meta 1), se ainda não existir uma com esse Inicio.
func gerarDiaUnico(existentes []Ocorrencia, dia Dia) []Ocorrencia {
	for _, oc := range existentes {
		if oc.Inicio != nil && *oc.Inicio == dia {
			return nil
		}
	}
	ini, fim := dia, dia
	return []Ocorrencia{{Inicio: &ini, Fim: &fim, Meta: 1, Status: StatusAberta}}
}

// gerarVezesPeriodo cria a ocorrência do período (semana ou mês) que contém
// hoje, se ainda não existir. DECISÃO (Opus, decisão 3): item criado no meio
// do período gera uma ocorrência truncada (Inicio = dia de criação, não o
// início do período) com meta = min(vezes, dias restantes até o fim do
// período); períodos seguintes são cheios, meta = vezes.
func gerarVezesPeriodo(regra Regra, existentes []Ocorrencia, hoje, criadoEm Dia) []Ocorrencia {
	var inicioPeriodo, fimPeriodo Dia
	if regra.Periodo == PeriodoMes {
		inicioPeriodo, fimPeriodo = InicioMes(hoje), FimMes(hoje)
	} else {
		inicioPeriodo, fimPeriodo = InicioSemana(hoje), FimSemana(hoje)
	}

	inicio := inicioPeriodo
	truncado := false
	if criadoEm > inicioPeriodo {
		inicio = criadoEm
		truncado = true
	}

	for _, oc := range existentes {
		if oc.Inicio != nil && *oc.Inicio == inicio {
			return nil // já existe a ocorrência deste período
		}
	}

	meta := regra.Vezes
	if truncado {
		diasRestantes := diasEntre(inicio, fimPeriodo) + 1
		if diasRestantes < meta {
			meta = diasRestantes
		}
	}

	ini, fim := inicio, fimPeriodo
	return []Ocorrencia{{Inicio: &ini, Fim: &fim, Meta: meta, Status: StatusAberta}}
}

// podeVirarPerdida diz se ocorrências de item podem virar "perdida" (§3.3
// regra 2 é sobre hábitos; decisão 5: compromisso nunca vira perdida —
// fica aberta e aparece como atrasado; decisão 4: intervalo também nunca
// vira perdida — vira "atrasado", tratado por EstadoIntervalo).
func podeVirarPerdida(item Item) bool {
	if item.Tipo != TipoHabito {
		return false
	}
	return item.Regra == nil || item.Regra.Tipo != RegraIntervalo
}

// FecharVencidas devolve, entre ocorrencias, as que devem virar "perdida"
// porque Fim < hoje e ainda estão "aberta" (§3.3 regra 2). Hábitos
// intervalo e compromissos nunca entram aqui (podeVirarPerdida). As
// ocorrências devolvidas já vêm com Status = perdida — quem persiste é o
// store (etapa 3).
func FecharVencidas(item Item, ocorrencias []Ocorrencia, hoje Dia) []Ocorrencia {
	if !podeVirarPerdida(item) {
		return nil
	}
	var vencidas []Ocorrencia
	for _, oc := range ocorrencias {
		if oc.Status != StatusAberta || oc.Fim == nil {
			continue
		}
		if *oc.Fim < hoje {
			nova := oc
			nova.Status = StatusPerdida
			vencidas = append(vencidas, nova)
		}
	}
	return vencidas
}

// StatusApos calcula o status de oc depois de nRegistros registros no seu
// intervalo (§3.3 regras 3–4, decisão 6): n >= meta é cumprida; senão, se já
// estava pulada continua pulada (regra 4: pular não conta na taxa nem
// quebra a sequência, é desfazível); senão, Fim < hoje vira perdida — exceto
// para intervalo/compromisso (podeVirarPerdida); senão fica aberta.
func StatusApos(item Item, oc Ocorrencia, nRegistros int, hoje Dia) string {
	if nRegistros >= oc.Meta {
		return StatusCumprida
	}
	if oc.Status == StatusPulada {
		return StatusPulada
	}
	if oc.Fim != nil && *oc.Fim < hoje && podeVirarPerdida(item) {
		return StatusPerdida
	}
	return StatusAberta
}

// Despular recalcula o status de oc como se ela não estivesse pulada (a
// base para "despular" na API, §3.3 regra 4).
func Despular(item Item, oc Ocorrencia, nRegistros int, hoje Dia) string {
	base := oc
	base.Status = StatusAberta
	return StatusApos(item, base, nRegistros, hoje)
}

// OcorrenciaDoDia acha, entre as ocorrências de um item, a que cobre dia
// (Inicio <= dia <= Fim, com Fim nulo tratado como "sem limite superior") —
// usada para a marcação retroativa (§3.3 regra 6). Sem ocorrência cobrindo
// aquele dia, ok volta false e o registro é extra (não recalcula nada).
//
// DECISÃO: esta função não cobre o caso "antecipado" do hábito intervalo
// (registrar antes do Inicio da ocorrência aberta) — esse fluxo usa
// RegistrarIntervalo, que aceita dia < Inicio de propósito (§3.3 regra 5).
func OcorrenciaDoDia(ocorrencias []Ocorrencia, dia Dia) (Ocorrencia, bool) {
	for _, oc := range ocorrencias {
		if oc.Inicio == nil || dia < *oc.Inicio {
			continue
		}
		if oc.Fim != nil && dia > *oc.Fim {
			continue
		}
		return oc, true
	}
	return Ocorrencia{}, false
}

// RegistrarIntervalo aplica um registro em dia sobre as ocorrências de um
// hábito intervalo (§3.3 regra 5, decisão A2 e decisão 4): a aberta vira
// cumprida com Fim = dia — mesmo se dia for antes do Inicio ("antecipado")
// — e nasce a próxima com Inicio = dia + intervaloDias.
//
// DECISÃO: se já existe uma cumprida com Fim >= dia (segundo registro no
// mesmo dia, ou marcação retroativa anterior à última vez feita), o
// registro é extra: ok = false e nada muda. Isso mantém os Fim das
// cumpridas crescentes e garante que a próxima nunca colide no
// UNIQUE(item_id, inicio). Sem ocorrência aberta, também é extra.
func RegistrarIntervalo(ocorrencias []Ocorrencia, dia Dia, intervaloDias int) (fechada, proxima Ocorrencia, ok bool) {
	var aberta *Ocorrencia
	for i := range ocorrencias {
		oc := ocorrencias[i]
		if oc.Status == StatusCumprida && oc.Fim != nil && *oc.Fim >= dia {
			return Ocorrencia{}, Ocorrencia{}, false
		}
		if oc.Status == StatusAberta && oc.Fim == nil {
			aberta = &ocorrencias[i]
		}
	}
	if aberta == nil {
		return Ocorrencia{}, Ocorrencia{}, false
	}

	fechada = *aberta
	fim := dia
	fechada.Fim = &fim
	fechada.Status = StatusCumprida

	proximoInicio := dia.AddDias(intervaloDias)
	proxima = Ocorrencia{
		ItemID: aberta.ItemID,
		Inicio: &proximoInicio,
		Fim:    nil,
		Meta:   1,
		Status: StatusAberta,
	}
	return fechada, proxima, true
}

// DesfazerRegistroIntervalo é o inverso de RegistrarIntervalo: a ocorrência
// cumprida volta a aberta com Fim nulo. A "próxima" que RegistrarIntervalo
// tinha criado (sem registros) deve ser apagada por quem chama (store/app,
// etapa 3) — aqui não há I/O, só devolvemos a reaberta.
func DesfazerRegistroIntervalo(cumprida Ocorrencia) Ocorrencia {
	reaberta := cumprida
	reaberta.Fim = nil
	reaberta.Status = StatusAberta
	return reaberta
}
