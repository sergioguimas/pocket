// Serve o SPA (React + Vite, etapa 6) embutido no binário. O
// //go:embed all:dist fica em main.go — dist/ é vizinho dele
// (servicos/pocket/dist), não de internal/httpapi, e go:embed não aceita
// caminhos "..". main.go passa o fs.FS já reduzido à raiz de dist/
// (fs.Sub(distFS, "dist")) para SetEstatico.
package httpapi

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// SetEstatico injeta o dist/ embutido — chamado por main.go depois de
// New(...). Sem chamada, handleEstatico devolve 404 (comportamento das
// etapas 1–5 preservado nos testes que não constroem o front).
func (s *Server) SetEstatico(dist fs.FS) {
	s.estatico = dist
}

// handleEstatico serve os arquivos do build (index.html, /assets/*.js|css
// com hash, sw.js, manifest.webmanifest, ícones) com fallback de SPA:
// qualquer rota que não seja um arquivo real do dist/ cai para index.html
// (roteamento client-side — refresh em /habitos precisa continuar servindo
// a SPA). sw.js e manifest.webmanifest saem sempre com Cache-Control
// no-cache (CONTRATO §6/checklist etapa 6: "sw.js sem cache") e o
// Content-Type correto do manifest (application/manifest+json).
func (s *Server) handleEstatico(w http.ResponseWriter, r *http.Request) {
	if s.estatico == nil {
		http.NotFound(w, r)
		return
	}

	// /api/* que não bateu em nenhuma rota acima é 404 de verdade, nunca a
	// SPA (evita devolver HTML para um endpoint de API inexistente/errado).
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}

	nome := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if nome == "" || nome == "." {
		nome = "index.html"
	}

	aplicarCabecalhosEspeciais(w, nome)

	f, err := s.estatico.Open(nome)
	if err != nil {
		// fallback de SPA.
		nome = "index.html"
		w.Header().Set("Cache-Control", "no-cache")
		f, err = s.estatico.Open(nome)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	rs, ok := f.(io.ReadSeeker)
	if !ok {
		// embed.FS (produção) e fstest.MapFS (testes) sempre implementam
		// io.ReadSeeker; isto só cobre um fs.FS exótico que não implemente.
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = io.Copy(w, f)
		return
	}
	http.ServeContent(w, r, nome, info.ModTime(), rs)
}

// aplicarCabecalhosEspeciais cobre os dois arquivos que o PWA precisa
// sempre buscar frescos: sw.js (senão o navegador demora a pegar uma nova
// versão do worker) e o manifest (Content-Type exigido pelos navegadores
// para instalar o app corretamente).
func aplicarCabecalhosEspeciais(w http.ResponseWriter, nome string) {
	switch nome {
	case "sw.js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
	case "manifest.webmanifest":
		w.Header().Set("Content-Type", "application/manifest+json")
		w.Header().Set("Cache-Control", "no-cache")
	}
}
