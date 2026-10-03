// Package ops cobre as tarefas de operação do Pocket que não são domínio:
// certificado TLS com recarga (este arquivo), e depois renovação do
// acme.sh e backup (etapa 5).
package ops

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

// CertLoader relê o par certificado/chave quando o arquivo muda — a
// renovação do acme.sh troca os arquivos e o servidor pega sem reiniciar.
// Preservado do spike da fase 0 (CONTRATO §7).
type CertLoader struct {
	cert, key string
	mu        sync.Mutex
	mod       time.Time
	cur       *tls.Certificate
}

// NewCertLoader cria o loader para o par de arquivos em cert e key.
func NewCertLoader(cert, key string) *CertLoader {
	return &CertLoader{cert: cert, key: key}
}

// Get devolve o certificado atual, relendo do disco se o arquivo mudou
// desde a última leitura. Assinatura compatível com
// tls.Config.GetCertificate.
func (l *CertLoader) Get(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	st, err := os.Stat(l.cert)
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cur == nil || st.ModTime().After(l.mod) {
		c, err := tls.LoadX509KeyPair(l.cert, l.key)
		if err != nil {
			if l.cur != nil {
				// arquivo no meio da troca (acme.sh escrevendo): fica com o
				// certificado antigo em vez de derrubar o servidor.
				return l.cur, nil
			}
			return nil, err
		}
		l.cur, l.mod = &c, st.ModTime()
		slog.Info("certificado (re)carregado", "cert", l.cert)
	}
	return l.cur, nil
}

// Validade devolve o "not after" (data de expiração) do certificado
// atualmente carregado. Usada por GET /health (CONTRATO §5, §7).
func (l *CertLoader) Validade() (time.Time, error) {
	l.mu.Lock()
	cur := l.cur
	l.mu.Unlock()
	if cur == nil {
		return time.Time{}, fmt.Errorf("certificado ainda não carregado")
	}
	// DECISÃO: parseia o leaf explicitamente em vez de confiar em
	// tls.Certificate.Leaf (preenchido automaticamente só em versões mais
	// novas do Go) — assim funciona independente da versão do toolchain.
	leaf := cur.Leaf
	if leaf == nil {
		var err error
		leaf, err = x509.ParseCertificate(cur.Certificate[0])
		if err != nil {
			return time.Time{}, fmt.Errorf("parsear certificado: %w", err)
		}
	}
	return leaf.NotAfter, nil
}
