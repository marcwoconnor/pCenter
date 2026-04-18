package pve

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

// makeSelfSignedPEM generates a fresh ECDSA P-256 self-signed cert at
// runtime so we don't hand-craft ASN.1 in test fixtures.
func makeSelfSignedPEM(t *testing.T) string {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(0xABCDEF1234),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes}))
}

func TestEnrichCertificateFromPEM_SelfSignedECDSA(t *testing.T) {
	c := &NodeCertificate{PEM: makeSelfSignedPEM(t)}
	EnrichCertificateFromPEM(c)

	if c.Serial == "" {
		t.Errorf("Serial empty — expected colon-hex value")
	}
	if c.SignatureAlgorithm == "" {
		t.Errorf("SignatureAlgorithm empty — expected ECDSA-SHA256 or similar")
	}
	if !c.IsSelfSigned {
		t.Errorf("IsSelfSigned = false — subject and issuer both CN=localhost should self-sign")
	}
	// Key usage should include at least digitalSignature (flag 0x01).
	if len(c.KeyUsage) == 0 {
		t.Errorf("KeyUsage empty — expected at least digitalSignature")
	}
	// Extended key usage includes serverAuth.
	foundServer := false
	for _, u := range c.ExtendedKeyUsage {
		if u == "serverAuth" {
			foundServer = true
		}
	}
	if !foundServer {
		t.Errorf("ExtendedKeyUsage missing serverAuth; got %v", c.ExtendedKeyUsage)
	}
}

func TestEnrichCertificateFromPEM_EmptyIsNoOp(t *testing.T) {
	c := &NodeCertificate{}
	EnrichCertificateFromPEM(c)
	if c.Serial != "" || c.SignatureAlgorithm != "" {
		t.Errorf("empty PEM should yield empty fields")
	}
}

func TestEnrichCertificateFromPEM_MalformedIsNoOp(t *testing.T) {
	c := &NodeCertificate{PEM: "not a pem"}
	EnrichCertificateFromPEM(c)
	if c.Serial != "" {
		t.Errorf("malformed PEM should not populate fields")
	}
}

func TestColonHex(t *testing.T) {
	got := colonHex([]byte{0x12, 0xab, 0xFF})
	want := "12:AB:FF"
	if got != want {
		t.Errorf("colonHex = %q, want %q", got, want)
	}
}
