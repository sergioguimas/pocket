package domain

import "testing"

// ocDia monta uma ocorrência de um único dia (Inicio=Fim=dia) já fechada
// com o status dado — atalho para os testes de métrica abaixo.
func ocDia(dia Dia, status string) Ocorrencia {
	d := dia
	return Ocorrencia{Inicio: &d, Fim: &d, Meta: 1, Status: status}
}

func TestSequenciaAtual(t *testing.T) {
	casos := []struct {
		nome string
		ocs  []Ocorrencia
		want int
	}{
		{
			"todas cumpridas",
			[]Ocorrencia{
				ocDia("2026-09-01", StatusCumprida),
				ocDia("2026-09-02", StatusCumprida),
				ocDia("2026-09-03", StatusCumprida),
			},
			3,
		},
		{
			"quebra no meio conta só depois da perdida",
			[]Ocorrencia{
				ocDia("2026-09-01", StatusCumprida),
				ocDia("2026-09-02", StatusPerdida),
				ocDia("2026-09-03", StatusCumprida),
				ocDia("2026-09-04", StatusCumprida),
			},
			2,
		},
		{
			"pulada é transparente (não quebra, não conta)",
			[]Ocorrencia{
				ocDia("2026-09-01", StatusCumprida),
				ocDia("2026-09-02", StatusPulada),
				ocDia("2026-09-03", StatusCumprida),
			},
			2,
		},
		{
			"periodo corrente aberto não quebra nem conta",
			[]Ocorrencia{
				ocDia("2026-09-01", StatusCumprida),
				ocDia("2026-09-02", StatusCumprida),
				ocDia("2026-09-03", StatusAberta),
			},
			2,
		},
		{
			"periodo corrente aberto depois de uma perdida",
			[]Ocorrencia{
				ocDia("2026-09-01", StatusCumprida),
				ocDia("2026-09-02", StatusPerdida),
				ocDia("2026-09-03", StatusAberta),
			},
			0,
		},
		{
			"vazio",
			nil,
			0,
		},
		{
			"perdida mais recente zera",
			[]Ocorrencia{
				ocDia("2026-09-01", StatusCumprida),
				ocDia("2026-09-02", StatusCumprida),
				ocDia("2026-09-03", StatusPerdida),
			},
			0,
		},
		{
			"ordem de entrada não importa (ordena por Inicio)",
			[]Ocorrencia{
				ocDia("2026-09-03", StatusCumprida),
				ocDia("2026-09-01", StatusCumprida),
				ocDia("2026-09-02", StatusCumprida),
			},
			3,
		},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := SequenciaAtual(c.ocs); got != c.want {
				t.Errorf("SequenciaAtual() = %d, esperava %d", got, c.want)
			}
		})
	}
}

func TestMelhorSequencia(t *testing.T) {
	ocs := []Ocorrencia{
		ocDia("2026-09-01", StatusCumprida),
		ocDia("2026-09-02", StatusCumprida),
		ocDia("2026-09-03", StatusPerdida),
		ocDia("2026-09-04", StatusCumprida),
		ocDia("2026-09-05", StatusCumprida),
		ocDia("2026-09-06", StatusCumprida),
		ocDia("2026-09-07", StatusPulada), // transparente: não interrompe a sequência de 3
		ocDia("2026-09-08", StatusCumprida),
		ocDia("2026-09-09", StatusAberta), // corrente, transparente
	}
	// melhor sequência: 04,05,06 (pulada em 07 transparente) + 08 = 4
	if got := MelhorSequencia(ocs); got != 4 {
		t.Errorf("MelhorSequencia() = %d, esperava 4", got)
	}

	if got := MelhorSequencia(nil); got != 0 {
		t.Errorf("MelhorSequencia(nil) = %d, esperava 0", got)
	}
}

func TestTaxa30d(t *testing.T) {
	hoje := Dia("2026-09-30")

	t.Run("sem ocorrências fechadas: ok=false", func(t *testing.T) {
		if _, ok := Taxa30d(nil, hoje); ok {
			t.Errorf("esperava ok=false com denominador 0")
		}
	})

	t.Run("só aberta futura, sem fechadas: ok=false", func(t *testing.T) {
		ocs := []Ocorrencia{ocDia(hoje, StatusAberta)}
		if _, ok := Taxa30d(ocs, hoje); ok {
			t.Errorf("esperava ok=false: aberta não conta")
		}
	})

	t.Run("3 cumpridas e 1 perdida na janela", func(t *testing.T) {
		ocs := []Ocorrencia{
			ocDia(hoje.AddDias(-1), StatusCumprida),
			ocDia(hoje.AddDias(-2), StatusCumprida),
			ocDia(hoje.AddDias(-3), StatusCumprida),
			ocDia(hoje.AddDias(-4), StatusPerdida),
		}
		taxa, ok := Taxa30d(ocs, hoje)
		if !ok {
			t.Fatalf("esperava ok=true")
		}
		if want := 0.75; taxa != want {
			t.Errorf("taxa = %v, esperava %v", taxa, want)
		}
	})

	t.Run("fora da janela de 30 dias não conta", func(t *testing.T) {
		ocs := []Ocorrencia{
			ocDia(hoje.AddDias(-31), StatusPerdida), // fora (hoje-30 é o limite)
			ocDia(hoje.AddDias(-1), StatusCumprida),
		}
		taxa, ok := Taxa30d(ocs, hoje)
		if !ok || taxa != 1.0 {
			t.Errorf("taxa = %v, ok = %v; esperava 1.0, true (a de -31 dias não deveria contar)", taxa, ok)
		}
	})

	t.Run("corrente cumprida hoje conta mesmo fora da janela [hoje-30,hoje-1]", func(t *testing.T) {
		ocs := []Ocorrencia{
			ocDia(hoje.AddDias(-1), StatusPerdida),
			ocDia(hoje, StatusCumprida), // Fim == hoje
		}
		taxa, ok := Taxa30d(ocs, hoje)
		if !ok || taxa != 0.5 {
			t.Errorf("taxa = %v, ok = %v; esperava 0.5, true", taxa, ok)
		}
	})

	t.Run("pulada não entra no cálculo", func(t *testing.T) {
		ocs := []Ocorrencia{
			ocDia(hoje.AddDias(-1), StatusCumprida),
			ocDia(hoje.AddDias(-2), StatusPulada),
		}
		taxa, ok := Taxa30d(ocs, hoje)
		if !ok || taxa != 1.0 {
			t.Errorf("taxa = %v, ok = %v; esperava 1.0, true (pulada não entra)", taxa, ok)
		}
	})
}

func TestEstadoIntervalo(t *testing.T) {
	inicio := Dia("2026-09-10")

	casos := []struct {
		nome string
		hoje Dia
		want EstadoIntervaloInfo
	}{
		{"faltam dias (em dia)", "2026-09-07", EstadoIntervaloInfo{EmDia: true, Dias: 3}},
		{"vence hoje (0 = atrasado 0)", "2026-09-10", EstadoIntervaloInfo{EmDia: false, Dias: 0}},
		{"atrasado 1 dia", "2026-09-11", EstadoIntervaloInfo{EmDia: false, Dias: 1}},
		{"atrasado 5 dias", "2026-09-15", EstadoIntervaloInfo{EmDia: false, Dias: 5}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			aberta := Ocorrencia{Inicio: &inicio, Fim: nil, Meta: 1, Status: StatusAberta}
			got := EstadoIntervalo(aberta, c.hoje)
			if got != c.want {
				t.Errorf("EstadoIntervalo() = %+v, esperava %+v", got, c.want)
			}
		})
	}
}
