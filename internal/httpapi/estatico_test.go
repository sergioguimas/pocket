package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// distTeste simula um dist/ mínimo — só o suficiente para exercitar as
// regras da etapa 6 (índice em "/", fallback de SPA, cabeçalhos de sw.js e
// do manifest) sem precisar rodar `npm run build`.
func distTeste() fstest.MapFS {
	return fstest.MapFS{
		"index.html":             {Data: []byte("<html>pocket</html>")},
		"assets/index-abc123.js": {Data: []byte("console.log('pocket')")},
		"sw.js":                  {Data: []byte("self.addEventListener('push', () => {})")},
		"manifest.webmanifest":   {Data: []byte(`{"name":"Pocket"}`)},
	}
}

func TestEstaticoIndexServidoNaRaiz(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "dev")
	srv.SetEstatico(distTeste())

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != "<html>pocket</html>" {
		t.Errorf("corpo = %q", rr.Body.String())
	}
}

func TestEstaticoFallbackDeSPA(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "dev")
	srv.SetEstatico(distTeste())

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/habitos", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != "<html>pocket</html>" {
		t.Errorf("/habitos devia cair para index.html, corpo = %q", rr.Body.String())
	}
}

func TestEstaticoSemDistDevolve404(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "dev")
	// SetEstatico não chamado — mesmo comportamento das etapas 1–5.

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestEstaticoRotaApiDesconhecidaE404NaoIndex(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "dev")
	srv.SetEstatico(distTeste())

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/rota-inexistente", nil))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (não devia cair para o index.html)", rr.Code, http.StatusNotFound)
	}
}

func TestEstaticoSwJsSemCache(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "dev")
	srv.SetEstatico(distTeste())

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/sw.js", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-cache")
	}
}

func TestEstaticoManifestContentType(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "dev")
	srv.SetEstatico(distTeste())

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/manifest+json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/manifest+json")
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-cache")
	}
}

func TestEstaticoAssetComHashServidoNormalmente(t *testing.T) {
	st := abrirStoreTeste(t)
	srv := New(st, appTeste(t, st), "dev")
	srv.SetEstatico(distTeste())

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/assets/index-abc123.js", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != "console.log('pocket')" {
		t.Errorf("corpo = %q", rr.Body.String())
	}
}
