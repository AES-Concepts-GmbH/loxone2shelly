package shelly

import (
	"crypto/md5" //nolint:gosec // RFC 7616 digest auth, required by Gen1 compatibility
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"
)

// digestChallenge is a parsed WWW-Authenticate: Digest header.
type digestChallenge struct {
	realm     string
	nonce     string
	qop       string
	algorithm string
	opaque    string
}

// parseDigestChallenge parses the Digest parameters of a WWW-Authenticate header.
func parseDigestChallenge(header string) (*digestChallenge, bool) {
	const prefix = "digest "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return nil, false
	}
	params := parseAuthParams(header[len(prefix):])
	ch := &digestChallenge{
		realm:     params["realm"],
		nonce:     params["nonce"],
		qop:       params["qop"],
		algorithm: params["algorithm"],
		opaque:    params["opaque"],
	}
	if ch.nonce == "" {
		return nil, false
	}
	if ch.algorithm == "" {
		ch.algorithm = "MD5"
	}
	return ch, true
}

// parseAuthParams splits `k="v", k2=v2` into a map. Quoted values may contain commas.
func parseAuthParams(s string) map[string]string {
	out := make(map[string]string)
	for len(s) > 0 {
		s = strings.TrimLeft(s, " ,")
		eq := strings.IndexByte(s, '=')
		if eq < 0 {
			break
		}
		key := strings.ToLower(strings.TrimSpace(s[:eq]))
		s = s[eq+1:]
		var val string
		if strings.HasPrefix(s, `"`) {
			end := strings.IndexByte(s[1:], '"')
			if end < 0 {
				val = s[1:]
				s = ""
			} else {
				val = s[1 : end+1]
				s = s[end+2:]
			}
		} else {
			end := strings.IndexByte(s, ',')
			if end < 0 {
				val = s
				s = ""
			} else {
				val = s[:end]
				s = s[end+1:]
			}
		}
		out[key] = strings.TrimSpace(val)
	}
	return out
}

// digestAuthorization builds the Authorization header for a challenge
// (RFC 7616 with qop=auth; SHA-256 as used by Shelly Gen2+, MD5 as fallback).
func digestAuthorization(ch *digestChallenge, username, password, method, uri string) (string, error) {
	cnonceBytes := make([]byte, 8)
	if _, err := rand.Read(cnonceBytes); err != nil {
		return "", err
	}
	return digestAuthorizationWithNonce(ch, username, password, method, uri, hex.EncodeToString(cnonceBytes), "00000001")
}

func digestAuthorizationWithNonce(ch *digestChallenge, username, password, method, uri, cnonce, nc string) (string, error) {
	var newHash func() hash.Hash
	switch strings.ToUpper(ch.algorithm) {
	case "SHA-256":
		newHash = sha256.New
	case "MD5":
		newHash = md5.New
	default:
		return "", fmt.Errorf("unsupported digest algorithm %q", ch.algorithm)
	}
	h := func(parts ...string) string {
		hh := newHash()
		hh.Write([]byte(strings.Join(parts, ":")))
		return hex.EncodeToString(hh.Sum(nil))
	}

	ha1 := h(username, ch.realm, password)
	ha2 := h(method, uri)

	qop := ""
	if ch.qop != "" {
		// Server may offer "auth,auth-int"; we only do "auth".
		for _, q := range strings.Split(ch.qop, ",") {
			if strings.TrimSpace(q) == "auth" {
				qop = "auth"
				break
			}
		}
	}
	var response string
	if qop == "auth" {
		response = h(ha1, ch.nonce, nc, cnonce, qop, ha2)
	} else {
		response = h(ha1, ch.nonce, ha2)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, `Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s", algorithm=%s`,
		username, ch.realm, ch.nonce, uri, response, ch.algorithm)
	if qop != "" {
		fmt.Fprintf(&sb, `, qop=%s, nc=%s, cnonce="%s"`, qop, nc, cnonce)
	}
	if ch.opaque != "" {
		fmt.Fprintf(&sb, `, opaque="%s"`, ch.opaque)
	}
	return sb.String(), nil
}
