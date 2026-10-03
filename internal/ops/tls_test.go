package ops

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// gerarCertTeste escreve um par certificado/chave autoassinado válido de
// agora até validade, para os testes do CertLoader sem depender de rede
// nem do acme.sh real.
func gerarCertTeste(t *testing.T, dir string, validade time.Time) (certPath, keyPath string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gerar chave: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "pocket-teste"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     validade,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("criar certificado: %v", err)
	}
	keyDer, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("empacotar chave: %v", err)
	}

	certPath = filepath.Join(dir, "fullchain.pem")
	keyPath = filepath.Join(dir, "key.pem")
	certOut, err := os.Create(certPath)
	if err != nil {
		t.Fatalf("criar arquivo de cert: %v", err)
	}
	defer certOut.Close()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("escrever cert: %v", err)
	}
	keyOut, err := os.Create(keyPath)
	if err != nil {
		t.Fatalf("criar arquivo de chave: %v", err)
	}
	defer keyOut.Close()
	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDer}); err != nil {
		t.Fatalf("escrever chave: %v", err)
	}
	return certPath, keyPath
}

func TestCertLoaderGetEValidade(t *testing.T) {
	dir := t.TempDir()
	validade := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second)
	certPath, keyPath := gerarCertTeste(t, dir, validade)

	l := NewCertLoader(certPath, keyPath)
	cert, err := l.Get(nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cert == nil {
		t.Fatal("Get devolveu certificado nil sem erro")
	}

	got, err := l.Validade()
	if err != nil {
		t.Fatalf("Validade: %v", err)
	}
	if !got.Equal(validade) {
		t.Errorf("Validade = %v, want %v", got, validade)
	}
}

func TestCertLoaderRecarregaAoMudarArquivo(t *testing.T) {
	dir := t.TempDir()
	v1 := time.Now().Add(10 * 24 * time.Hour).Truncate(time.Second)
	certPath, keyPath := gerarCertTeste(t, dir, v1)

	l := NewCertLoader(certPath, keyPath)
	if _, err := l.Get(nil); err != nil {
		t.Fatalf("Get inicial: %v", err)
	}
	got1, _ := l.Validade()
	if !got1.Equal(v1) {
		t.Fatalf("validade inicial = %v, want %v", got1, v1)
	}

	// acme.sh troca os arquivos por um par novo; mtime precisa avançar para
	// o loader notar (alguns filesystems têm resolução de 1s).
	time.Sleep(1100 * time.Millisecond)
	v2 := time.Now().Add(90 * 24 * time.Hour).Truncate(time.Second)
	gerarCertTeste(t, dir, v2)

	if _, err := l.Get(nil); err != nil {
		t.Fatalf("Get após troca: %v", err)
	}
	got2, err := l.Validade()
	if err != nil {
		t.Fatalf("Validade após troca: %v", err)
	}
	if got2.Equal(v1) {
		t.Fatal("Validade não recarregou após o arquivo mudar")
	}
	if !got2.Equal(v2) {
		t.Errorf("Validade após troca = %v, want %v", got2, v2)
	}
}

func TestCertLoaderGetSemArquivo(t *testing.T) {
	dir := t.TempDir()
	l := NewCertLoader(filepath.Join(dir, "nao-existe.pem"), filepath.Join(dir, "nao-existe-key.pem"))
	if _, err := l.Get(nil); err == nil {
		t.Fatal("esperava erro sem os arquivos existirem")
	}
}
