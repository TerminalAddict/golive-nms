package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"testing"
)

func TestCSRParsingShape(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, e := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "agent"}}, key)
	if e != nil {
		t.Fatal(e)
	}
	block, _ := pem.Decode(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
	csr, e := x509.ParseCertificateRequest(block.Bytes)
	if e != nil || csr.CheckSignature() != nil {
		t.Fatal("CSR verification failed")
	}
}
