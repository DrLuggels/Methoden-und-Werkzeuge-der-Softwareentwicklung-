// Command demo ist eine schlanke Fullstack-Anwendung, die das jitelevation-Modul
// in Aktion zeigt: ein Go-HTTP-Server mit den /elevate/*-Endpunkten, einem
// persistenten Datei-Store, einem echten TOTP-Verifier (RFC 6238) und einem
// statischen Vanilla-JS-Frontend unter static/.
//
// Die Benutzeranmeldung ist bewusst trivial gehalten (Cookie mit dem
// Benutzernamen, kein Passwort) -- sie ist nicht Gegenstand der Arbeit. Im
// Zentrum steht der Step-Up-Elevation-Flow.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	jit "github.com/drluggels/jitelevation"
	"github.com/drluggels/jitelevation/adapter/filestore"
	"github.com/drluggels/jitelevation/adapter/totp"
)

// envOr liefert die Umgebungsvariable key oder fallback, falls sie leer ist.
// So lassen sich Pfade und Adresse im Container ueber das Environment setzen.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

const demoScope = "demo"

type server struct {
	mod   *jit.Module
	store *filestore.Store
	seeds *demoSeeds
}

func main() {
	store, err := filestore.Open(envOr("JIT_STATE_FILE", "demo-state.json"))
	if err != nil {
		log.Fatalf("Store oeffnen: %v", err)
	}
	seeds := newDemoSeeds()
	audit, err := newFileAudit(envOr("JIT_AUDIT_FILE", "demo-audit.log"))
	if err != nil {
		log.Fatalf("Audit oeffnen: %v", err)
	}
	policy := jit.NewStaticPolicy(map[jit.UserID]jit.Allowance{
		"alice": {MaxRole: "admin", MaxDuration: time.Hour},
		"bob":   {MaxRole: "viewer", MaxDuration: 10 * time.Minute},
	})
	mod, err := jit.New(jit.Dependencies{
		Store:  store,
		TOTP:   totp.New(seeds, totp.Config{Skew: 1}),
		Audit:  audit,
		Policy: policy,
	}, jit.Config{})
	if err != nil {
		log.Fatalf("Modul erstellen: %v", err)
	}

	srv := &server{mod: mod, store: store, seeds: seeds}

	// Hintergrund-Ticker laesst faellige Grants verfallen.
	go func() {
		for range time.Tick(15 * time.Second) {
			if n, err := mod.ExpireDue(context.Background()); err == nil && n > 0 {
				log.Printf("ExpireDue: %d Grant(s) abgelaufen", n)
			}
		}
	}()

	addr := envOr("JIT_ADDR", ":8080")
	log.Printf("JIT-Demo laeuft auf %s", addr)
	log.Fatal(http.ListenAndServe(addr, srv.routes()))
}

// routes registriert alle HTTP-Endpunkte. Ausgelagert, damit der Flow im Test
// gegen einen httptest-Server gefahren werden kann.
func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)
	mux.HandleFunc("GET /api/me", s.handleMe)
	mux.HandleFunc("GET /api/totp/setup", s.handleTOTPSetup)
	mux.HandleFunc("POST /api/elevate/request", s.handleRequest)
	mux.HandleFunc("POST /api/elevate/confirm", s.handleConfirm)
	mux.HandleFunc("GET /api/elevate/status", s.handleStatus)
	mux.HandleFunc("POST /api/elevate/revoke", s.handleRevoke)
	mux.HandleFunc("GET /api/admin/data", s.handleAdminData)
	mux.Handle("GET /", http.FileServer(http.Dir("static")))
	return mux
}

// --- Hilfsfunktionen --------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func errorJSON(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func currentUser(r *http.Request) jit.UserID {
	c, err := r.Cookie("jit_user")
	if err != nil {
		return ""
	}
	return jit.UserID(c.Value)
}

// --- Authentifizierung (Demo) -----------------------------------------------

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorJSON(w, http.StatusBadRequest, "ungueltige Anfrage")
		return
	}
	if _, _, err := s.seeds.provisioning(jit.UserID(req.Username)); err != nil {
		errorJSON(w, http.StatusUnauthorized, "unbekannter Demo-Benutzer (alice oder bob)")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "jit_user", Value: req.Username, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"user": req.Username})
}

func (s *server) handleLogout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "jit_user", Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]string{"status": "abgemeldet"})
}

func (s *server) handleMe(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == "" {
		errorJSON(w, http.StatusUnauthorized, "nicht angemeldet")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"user": string(user)})
}

func (s *server) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == "" {
		errorJSON(w, http.StatusUnauthorized, "nicht angemeldet")
		return
	}
	otpauth, secret, err := s.seeds.provisioning(user)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"otpauth": otpauth, "secret": secret})
}

// --- Elevation-Flow ---------------------------------------------------------

func (s *server) handleRequest(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == "" {
		errorJSON(w, http.StatusUnauthorized, "nicht angemeldet")
		return
	}
	var req struct {
		Role            string `json:"role"`
		Reason          string `json:"reason"`
		DurationMinutes int    `json:"duration_minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorJSON(w, http.StatusBadRequest, "ungueltige Anfrage")
		return
	}
	id, err := s.mod.Request(r.Context(), jit.RequestInput{
		User:          user,
		Scope:         demoScope,
		RequestedRole: req.Role,
		Reason:        req.Reason,
		Duration:      time.Duration(req.DurationMinutes) * time.Minute,
	})
	if errors.Is(err, jit.ErrPolicyDenied) {
		errorJSON(w, http.StatusForbidden, "Policy verweigert diese Erhoehung")
		return
	}
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"grant_id": string(id)})
}

func (s *server) handleConfirm(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == "" {
		errorJSON(w, http.StatusUnauthorized, "nicht angemeldet")
		return
	}
	var req struct {
		GrantID string `json:"grant_id"`
		Code    string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorJSON(w, http.StatusBadRequest, "ungueltige Anfrage")
		return
	}
	g, err := s.mod.Confirm(r.Context(), jit.GrantID(req.GrantID), req.Code)
	switch {
	case errors.Is(err, jit.ErrInvalidTOTP):
		errorJSON(w, http.StatusUnauthorized, "Zweitfaktor ungueltig")
	case errors.Is(err, jit.ErrTOTPReplay):
		errorJSON(w, http.StatusConflict, "Code bereits verwendet (Replay)")
	case errors.Is(err, jit.ErrConfirmExpired):
		errorJSON(w, http.StatusGone, "Bestaetigungsfrist abgelaufen")
	case errors.Is(err, jit.ErrInvalidState):
		errorJSON(w, http.StatusConflict, "Antrag nicht (mehr) bestaetigbar")
	case errors.Is(err, jit.ErrGrantNotFound):
		errorJSON(w, http.StatusNotFound, "Antrag nicht gefunden")
	case err != nil:
		errorJSON(w, http.StatusInternalServerError, err.Error())
	default:
		writeJSON(w, http.StatusOK, map[string]string{
			"state":      string(g.State),
			"role":       g.GrantedRole,
			"expires_at": g.ExpiresAt.Format(time.RFC3339),
		})
	}
}

func (s *server) handleRevoke(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == "" {
		errorJSON(w, http.StatusUnauthorized, "nicht angemeldet")
		return
	}
	var req struct {
		GrantID string `json:"grant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorJSON(w, http.StatusBadRequest, "ungueltige Anfrage")
		return
	}
	if err := s.mod.Revoke(r.Context(), jit.GrantID(req.GrantID), user); err != nil {
		errorJSON(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "widerrufen"})
}

func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == "" {
		errorJSON(w, http.StatusUnauthorized, "nicht angemeldet")
		return
	}
	grants, err := s.store.ListAll(r.Context(), user)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now()
	type view struct {
		ID          string `json:"id"`
		Role        string `json:"role"`
		State       string `json:"state"`
		Reason      string `json:"reason"`
		SecondsLeft int    `json:"seconds_left"`
	}
	out := make([]view, 0, len(grants))
	for _, g := range grants {
		secs := 0
		if g.IsActive(now) {
			secs = int(time.Until(g.ExpiresAt).Seconds())
		}
		out = append(out, view{
			ID:          string(g.ID),
			Role:        g.GrantedRole,
			State:       string(g.State),
			Reason:      g.Reason,
			SecondsLeft: secs,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// --- Geschuetzte Aktion (Demo der Check-Pruefung) ---------------------------

func (s *server) handleAdminData(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == "" {
		errorJSON(w, http.StatusUnauthorized, "nicht angemeldet")
		return
	}
	ok, err := s.mod.Check(r.Context(), user, demoScope, "admin")
	if err != nil || !ok {
		// Fail-closed: bei Fehler ODER fehlendem aktiven Grant verweigern.
		errorJSON(w, http.StatusForbidden, "kein aktiver Admin-Grant -- Zugriff verweigert")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"secret": "streng geheime Administrationsdaten",
		"hinweis": "Dieser Endpunkt antwortet nur bei aktiver, per TOTP " +
			"bestaetigter Admin-Erhoehung.",
	})
}
