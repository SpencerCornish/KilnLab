package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strings"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, status int, msg string, err error) {
	attrs := []any{"method", r.Method, "path", r.URL.Path, "status", status, "msg", msg}
	if err != nil {
		attrs = append(attrs, "err", err)
	}
	if status >= 500 {
		s.logger.Error("api_error", attrs...)
	} else {
		s.logger.Warn("api_error", attrs...)
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

const maxJSONBodyBytes = 1 << 20

// readJSON decodes a single JSON object, rejecting unknown fields and trailing data.
// It writes the error response itself and returns false on failure.
// readOptionalJSON accepts an empty body (Content-Length 0) as an empty struct.
func (s *Server) readOptionalJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.ContentLength == 0 {
		return true
	}
	return s.readJSON(w, r, v)
}

func (s *Server) readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(v)
	if err == nil {
		if dec.Decode(&struct{}{}) != io.EOF {
			err = errors.New("trailing data in JSON body")
		}
	}
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			s.writeError(w, r, http.StatusRequestEntityTooLarge, "request body too large", err)
		} else {
			s.writeError(w, r, http.StatusBadRequest, "invalid request body", err)
		}
		return false
	}
	return true
}

// randomToken returns 32 random bytes, hex encoded.
func randomToken() string {
	var b [32]byte
	_, _ = rand.Read(b[:]) // never fails on supported platforms (Go 1.24+ panics instead)
	return hex.EncodeToString(b[:])
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// clientIP returns the caller's IP, believing X-Forwarded-For only from trusted proxies.
func (s *Server) clientIP(r *http.Request) string {
	remote, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	addr := remote.Addr().Unmap()
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" || !s.isTrustedProxy(addr) {
		return addr.String()
	}
	// Walk right to left, skipping trusted hops; the first untrusted one is the client.
	hops := strings.Split(xff, ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		hop = hop.Unmap()
		if !s.isTrustedProxy(hop) {
			return hop.String()
		}
		addr = hop
	}
	return addr.String()
}

func (s *Server) isTrustedProxy(a netip.Addr) bool {
	for _, p := range s.trustedProxies {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
