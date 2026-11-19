package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/babo96/tradepost/server/internal/store"
)

const (
	loginCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no I/O/0/1
	loginCodeLength   = 8
	loginCodeTTL      = 10 * time.Minute
	sessionCookie     = "bf_session"
	sessionTTL        = 30 * 24 * time.Hour
	maxWatchItems     = 1000
)

func (s *Server) handleAuthCode(w http.ResponseWriter, r *http.Request) {
	client, err := s.authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var body struct {
		Character string `json:"character"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	character, err := normalizeCharacter(body.Character)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	code, err := makeLoginCode()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "code generation failed")
		return
	}
	expires := time.Now().Add(loginCodeTTL)
	sum := sha256.Sum256([]byte(code))
	if err := s.store.CreateLoginCode(r.Context(), sum[:], client.ID, character, expires); err != nil {
		writeError(w, http.StatusInternalServerError, "could not store code")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"code":       code,
		"character":  character,
		"expires_at": expires.Unix(),
	})
}

func (s *Server) handleAuthRedeem(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	code := normalizeLoginCode(body.Code)
	if len(code) != loginCodeLength {
		writeError(w, http.StatusUnauthorized, "invalid or expired code")
		return
	}
	sum := sha256.Sum256([]byte(code))
	grant, err := s.store.RedeemLoginCode(r.Context(), sum[:], time.Now())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired code")
		return
	}

	token, err := randomToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "session generation failed")
		return
	}
	tokenSum := sha256.Sum256([]byte(token))
	expires := time.Now().Add(sessionTTL)
	if err := s.store.CreateWebSession(r.Context(), tokenSum[:], grant.ClientID, grant.Character, expires); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	setSessionCookie(w, r, token, expires)
	writeJSON(w, http.StatusOK, map[string]any{
		"character":  grant.Character,
		"expires_at": expires.Unix(),
	})
}

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	session, _, ok := s.sessionFromRequest(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"character":  session.Character,
		"expires_at": session.ExpiresAt.Unix(),
	})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil && cookie.Value != "" {
		sum := sha256.Sum256([]byte(cookie.Value))
		_ = s.store.DeleteWebSession(r.Context(), sum[:])
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true,
		MaxAge: -1, SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleWatchGet(w http.ResponseWriter, r *http.Request) {
	clientID, character, err := s.watchIdentity(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	list, err := s.store.WatchList(r.Context(), clientID, character)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "watchlist failed")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleWatchPut(w http.ResponseWriter, r *http.Request) {
	clientID, character, err := s.watchIdentity(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	var body struct {
		Items []string `json:"items"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(body.Items) > maxWatchItems {
		writeError(w, http.StatusBadRequest, "too many watch items")
		return
	}
	seen := map[string]bool{}
	items := make([]string, 0, len(body.Items))
	for _, key := range body.Items {
		key = strings.TrimSpace(key)
		if key == "" || seen[key] {
			continue
		}
		if !itemRE.MatchString(key) {
			writeError(w, http.StatusBadRequest, "invalid item key")
			return
		}
		seen[key] = true
		items = append(items, key)
	}
	list, err := s.store.SetWatchList(r.Context(), clientID, character, items)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "watchlist update failed")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// watchIdentity accepts either a companion bearer token plus ?character=, or
// a browser session cookie.
func (s *Server) watchIdentity(r *http.Request) (clientID, character string, err error) {
	if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		client, authErr := s.authenticate(r)
		if authErr != nil {
			return "", "", authErr
		}
		character, err = normalizeCharacter(r.URL.Query().Get("character"))
		if err != nil {
			return "", "", err
		}
		return client.ID, character, nil
	}
	session, _, ok := s.sessionFromRequest(r)
	if !ok {
		return "", "", store.ErrNotFound
	}
	return session.ClientID, session.Character, nil
}

func (s *Server) sessionFromRequest(r *http.Request) (store.WebSession, string, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return store.WebSession{}, "", false
	}
	sum := sha256.Sum256([]byte(cookie.Value))
	session, err := s.store.WebSession(r.Context(), sum[:], time.Now())
	if err != nil {
		return store.WebSession{}, "", false
	}
	return session, cookie.Value, true
}

func normalizeCharacter(value string) (string, error) {
	character := strings.TrimSpace(value)
	if character == "" || len(character) > 120 {
		return "", errors.New("invalid character")
	}
	if strings.ContainsAny(character, "\r\n\t") {
		return "", errors.New("invalid character")
	}
	return character, nil
}

func normalizeLoginCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "")
	return replacer.Replace(value)
}

func makeLoginCode() (string, error) {
	buf := make([]byte, loginCodeLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, loginCodeLength)
	for i, value := range buf {
		out[i] = loginCodeAlphabet[int(value)&31]
	}
	return string(out), nil
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
	})
}

func decodeJSON(r *http.Request, value any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	return decoder.Decode(value)
}
