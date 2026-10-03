package captura

import "context"

// FakeTranscritor é um Transcritor de teste (app/httpapi não precisam de
// rede para testar o resto do fluxo).
type FakeTranscritor struct {
	Texto string
	Err   error
	// Chamadas guarda os áudios recebidos, para os testes conferirem que o
	// app passou o bytes/mime certos.
	Chamadas []FakeTranscricaoChamada
}

type FakeTranscricaoChamada struct {
	Audio []byte
	Mime  string
}

var _ Transcritor = (*FakeTranscritor)(nil)

func (f *FakeTranscritor) Transcrever(_ context.Context, audio []byte, mime string) (string, error) {
	f.Chamadas = append(f.Chamadas, FakeTranscricaoChamada{Audio: audio, Mime: mime})
	if f.Err != nil {
		return "", f.Err
	}
	return f.Texto, nil
}

// FakeInterpretador é um Interpretador de teste: devolve ModelJSON cru (o
// mesmo formato que Normalizar espera) ou Err.
type FakeInterpretador struct {
	ModelJSON []byte
	Uso       Uso
	Err       error
	// Chamadas guarda os contextos/textos recebidos.
	Chamadas []FakeInterpretacaoChamada
}

type FakeInterpretacaoChamada struct {
	Contexto Contexto
	Texto    string
}

var _ Interpretador = (*FakeInterpretador)(nil)

func (f *FakeInterpretador) Interpretar(_ context.Context, contexto Contexto, texto string) ([]byte, Uso, error) {
	f.Chamadas = append(f.Chamadas, FakeInterpretacaoChamada{Contexto: contexto, Texto: texto})
	if f.Err != nil {
		return nil, Uso{}, f.Err
	}
	return f.ModelJSON, f.Uso, nil
}
