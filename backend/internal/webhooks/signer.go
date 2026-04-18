package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// SignatureHeader is the HTTP header carrying the signature.
const SignatureHeader = "X-pCenter-Signature"

// Sign returns the header value for a request body at a given timestamp,
// using the Stripe-style format: "t=<unix>,v1=<hex-hmac-sha256(secret, t.body)>".
// The timestamp is included in the signed material so receivers can reject
// replays by checking skew.
func Sign(secret string, body []byte, ts time.Time) string {
	unix := ts.Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.", unix)
	mac.Write(body)
	return fmt.Sprintf("t=%d,v1=%s", unix, hex.EncodeToString(mac.Sum(nil)))
}
