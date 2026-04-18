package pve

import (
	"crypto/x509"
	"encoding/pem"
	"strings"
)

// EnrichCertificateFromPEM parses the PEM field (if present) and populates the
// server-side parsed fields on the NodeCertificate: Serial, SignatureAlgorithm,
// KeyUsage, ExtendedKeyUsage, IsCA, IsSelfSigned.
//
// PVE's `/nodes/{node}/certificates/info` endpoint doesn't return these
// natively; we decode the PEM locally. Failures are ignored (the extra fields
// simply stay empty) so a malformed cert never breaks the listing.
func EnrichCertificateFromPEM(c *NodeCertificate) {
	if c == nil || c.PEM == "" {
		return
	}
	block, _ := pem.Decode([]byte(c.PEM))
	if block == nil {
		return
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return
	}
	if cert.SerialNumber != nil {
		c.Serial = colonHex(cert.SerialNumber.Bytes())
	}
	c.SignatureAlgorithm = cert.SignatureAlgorithm.String()
	c.KeyUsage = keyUsageStrings(cert.KeyUsage)
	c.ExtendedKeyUsage = extKeyUsageStrings(cert.ExtKeyUsage)
	c.IsCA = cert.IsCA
	c.IsSelfSigned = cert.Issuer.String() == cert.Subject.String()
}

// colonHex formats bytes as "AB:CD:EF" — the conventional display form
// for cert serial numbers and fingerprints.
func colonHex(b []byte) string {
	const hexChars = "0123456789abcdef"
	if len(b) == 0 {
		return ""
	}
	out := make([]byte, 0, len(b)*3-1)
	for i, v := range b {
		if i > 0 {
			out = append(out, ':')
		}
		out = append(out, hexChars[v>>4], hexChars[v&0x0f])
	}
	return strings.ToUpper(string(out))
}

// keyUsageStrings converts a KeyUsage bitmask into readable flag names.
func keyUsageStrings(u x509.KeyUsage) []string {
	var out []string
	flags := []struct {
		bit  x509.KeyUsage
		name string
	}{
		{x509.KeyUsageDigitalSignature, "digitalSignature"},
		{x509.KeyUsageContentCommitment, "contentCommitment"},
		{x509.KeyUsageKeyEncipherment, "keyEncipherment"},
		{x509.KeyUsageDataEncipherment, "dataEncipherment"},
		{x509.KeyUsageKeyAgreement, "keyAgreement"},
		{x509.KeyUsageCertSign, "certSign"},
		{x509.KeyUsageCRLSign, "crlSign"},
		{x509.KeyUsageEncipherOnly, "encipherOnly"},
		{x509.KeyUsageDecipherOnly, "decipherOnly"},
	}
	for _, f := range flags {
		if u&f.bit != 0 {
			out = append(out, f.name)
		}
	}
	return out
}

// extKeyUsageStrings converts an ExtKeyUsage slice into readable OID names.
func extKeyUsageStrings(usages []x509.ExtKeyUsage) []string {
	names := map[x509.ExtKeyUsage]string{
		x509.ExtKeyUsageAny:                            "any",
		x509.ExtKeyUsageServerAuth:                     "serverAuth",
		x509.ExtKeyUsageClientAuth:                     "clientAuth",
		x509.ExtKeyUsageCodeSigning:                    "codeSigning",
		x509.ExtKeyUsageEmailProtection:                "emailProtection",
		x509.ExtKeyUsageIPSECEndSystem:                 "ipsecEndSystem",
		x509.ExtKeyUsageIPSECTunnel:                    "ipsecTunnel",
		x509.ExtKeyUsageIPSECUser:                      "ipsecUser",
		x509.ExtKeyUsageTimeStamping:                   "timeStamping",
		x509.ExtKeyUsageOCSPSigning:                    "ocspSigning",
		x509.ExtKeyUsageMicrosoftServerGatedCrypto:     "msServerGatedCrypto",
		x509.ExtKeyUsageNetscapeServerGatedCrypto:      "nsServerGatedCrypto",
		x509.ExtKeyUsageMicrosoftCommercialCodeSigning: "msCommercialCodeSigning",
		x509.ExtKeyUsageMicrosoftKernelCodeSigning:     "msKernelCodeSigning",
	}
	out := make([]string, 0, len(usages))
	for _, u := range usages {
		if n, ok := names[u]; ok {
			out = append(out, n)
		}
	}
	return out
}
