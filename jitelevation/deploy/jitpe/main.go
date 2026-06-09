// Command jitpe ist der zentrale Just-in-Time-Privilege-Elevation-Dienst der
// Demo-Topologie. Er nutzt das jitelevation-Modul, stellt den Benutzer-Flow
// (Antrag, TOTP-Bestaetigung, Status, Widerruf) samt Frontend bereit und bietet
// zusaetzlich einen Service-zu-Service-Endpunkt /api/verify, ueber den die
// prod- und dev-Backends vor einer privilegierten Aktion nachfragen.
//
// Im Gegensatz zur Einzel-App-Demo (examples/demo) ist der Scope hier ein
// Parameter: derselbe Dienst verwaltet Erhoehungen fuer mehrere Zielsysteme
// (z. B. "prod" und "dev").
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

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type server struct {
	mod   *jit.Module
	store *filestore.Store
	seeds *demoSeeds
}

func main() {
	store, err := filestore.Open(envOr("JIT_STATE_FILE", "/data/state.json"))
	if err != nil {
		log.Fatalf("Store oeffnen: %v", err)
	}
	seeds := newDemoSeeds()
	audit, err := newFileAudit(envOr("JIT_AUDIT_FILE", "/data/audit.log"))
	if err != nil {
		log.Fatalf("Audit oeffnen: %v", err)
	}
	// alice darf in jedem Scope bis admin, bob nur bis viewer.
	policy := jit.NewStaticPolicy(map[jit.UserID]jit.Allowance{
		"alice": {MaxRole: "admin", MaxDuration: time.Hour},
		"bob":   {MaxRole: "viewer", MaxDuration: 10 * time.Minute},
	})
	mod, err := jit.New(jit.Dependencies{
		Store: store, TOTP: totp.New(seeds, totp.Config{Skew: 1}), Audit: audit, Policy: policy,
	}, jit.Config{})
	if err != nil {
		log.Fatalf("Modul erstellen: %v", err)
	}
	srv := &server{mod: mod, store: store, seeds: seeds}

	go func() {
		for range time.Tick(15 * time.Second) {
			if n, err := mod.ExpireDue(context.Background()); err == nil && n > 0 {
				log.Printf("ExpireDue: %d Grant(s) abgelaufen", n)
			}
		}
	}()

	addr := envOr("JIT_ADDR", ":8090")
	log.Printf("JITPE-Dienst laeuft auf %s", addr)
	log.Fatal(http.ListenAndServe(addr, srv.routes()))
}

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
	mux.HandleFunc("POST /api/elevate/cancel", s.handleCancel)
	// Service-zu-Service: von prod-/dev-Backend aufgerufen.
	mux.HandleFunc("GET /api/verify", s.handleVerify)
	mux.Handle("GET /", http.FileServer(http.Dir("web")))
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

// --- Service-zu-Service-Verifikation ----------------------------------------

// handleVerify wird von einem Backend aufgerufen, das vor einer privilegierten
// Aktion pruefen will, ob der Benutzer eine aktive Erhoehung im eigenen Scope
// besitzt. Fail-closed: bei jedem Fehler oder fehlendem Grant -> 403.
func (s *server) handleVerify(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	user := jit.UserID(q.Get("user"))
	scope := q.Get("scope")
	role := q.Get("role")
	if user == "" || scope == "" || role == "" {
		errorJSON(w, http.StatusBadRequest, "user, scope und role erforderlich")
		return
	}
	ok, err := s.mod.Check(r.Context(), user, scope, role)
	if err != nil || !ok {
		errorJSON(w, http.StatusForbidden, "keine aktive Erhoehung")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"allowed": true})
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
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
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
		Scope           string `json:"scope"`
		Role            string `json:"role"`
		Reason          string `json:"reason"`
		DurationMinutes int    `json:"duration_minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorJSON(w, http.StatusBadRequest, "ungueltige Anfrage")
		return
	}
	if req.Scope == "" {
		errorJSON(w, http.StatusBadRequest, "scope erforderlich (prod oder dev)")
		return
	}
	id, err := s.mod.Request(r.Context(), jit.RequestInput{
		User:          user,
		Scope:         req.Scope,
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
	if currentUser(r) == "" {
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
			"state": string(g.State), "scope": g.Scope, "role": g.GrantedRole,
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

func (s *server) handleCancel(w http.ResponseWriter, r *http.Request) {
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
	if err := s.mod.Cancel(r.Context(), jit.GrantID(req.GrantID), user); err != nil {
		errorJSON(w, http.StatusConflict, "Antrag nicht abbrechbar (nur offene Antraege)")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "abgelehnt"})
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
		Scope       string `json:"scope"`
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
			ID: string(g.ID), Scope: g.Scope, Role: g.GrantedRole,
			State: string(g.State), Reason: g.Reason, SecondsLeft: secs,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
