package session

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

// JWTExpiry reads when a token that is a JWT runs out, without checking its
// signature. A token that cannot be read gives the zero time, which a
// Session takes to mean that the service did not say.
func JWTExpiry(token string) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(claims.Exp, 0)
}
