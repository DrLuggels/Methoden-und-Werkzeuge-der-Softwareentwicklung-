// Command appbackend ist das Backend einer Ziel-Anwendung (prod bzw. dev). Es
// repraesentiert den "kompletten Stack": es verbindet sich beim Start mit seiner
// Datenbank und stellt eine oeffentliche sowie eine privilegierte Aktion bereit.
//
// Vor der privilegierten Aktion fragt es beim zentralen JITPE-Dienst nach
// (GET /api/verify). Nur bei aktiver, per Zweitfaktor bestaetigter Erhoehung im
// eigenen Scope wird die Aktion ausgefuehrt. Faellt JITPE aus oder fehlt der
// Grant, wird verweigert (fail-closed).
//
// Derselbe Code laeuft als prod- und als dev-Backend; unterschieden wird allein
// ueber die Umgebungsvariable APP_SCOPE.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type backend struct {
	scope    string // "prod" oder "dev"
	jitpeURL string // Basis-URL des JITPE-Dienstes
	dbAddr   string // host:port der Datenbank (Teil des Stacks)
	http     *http.Client
}

func main() {
	b := &backend{
		scope:    envOr("APP_SCOPE", "prod"),
		jitpeURL: envOr("JITPE_URL", "http://jitpe:8090"),
		dbAddr:   os.Getenv("DB_ADDR"),
		http:     &http.Client{Timeout: 3 * time.Second},
	}

	// Auf die Datenbank warten (Teil des vollstaendigen Stacks). Die
	// Erreichbarkeit wird ueber einen TCP-Verbindungsaufbau geprueft; das haelt
	// das Backend frei von externen Treiber-Abhaengigkeiten.
	if b.dbAddr != "" {
		if waitForTCP(b.dbAddr, 30) {
			log.Printf("[%s] Datenbank %s erreichbar", b.scope, b.dbAddr)
		} else {
			log.Printf("[%s] WARN: Datenbank %s nicht erreichbar", b.scope, b.dbAddr)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/info", b.handleInfo)
	mux.HandleFunc("POST /api/admin/action", b.handleAdminAction)
	mux.Handle("GET /", http.FileServer(http.Dir("web")))

	addr := envOr("APP_ADDR", ":8080")
	log.Printf("[%s] Backend laeuft auf %s, JITPE=%s", b.scope, addr, b.jitpeURL)
	log.Fatal(http.ListenAndServe(addr, mux))
}

// waitForTCP versucht bis zu attempts Sekunden lang einen TCP-Verbindungsaufbau.
func waitForTCP(addr string, attempts int) bool {
	for i := 0; i < attempts; i++ {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(time.Second)
	}
	return false
}

// dbReachable meldet, ob die Datenbank gerade per TCP erreichbar ist.
func (b *backend) dbReachable() bool {
	if b.dbAddr == "" {
		return false
	}
	conn, err := net.DialTimeout("tcp", b.dbAddr, time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (b *backend) user(r *http.Request) string {
	// Demo: der Benutzername kommt aus einem Header, den das Frontend setzt.
	// Produktiv liefert ihn die etablierte Session des Host-Systems.
	return r.Header.Get("X-User")
}

// handleInfo ist die oeffentliche Aktion (keine Erhoehung noetig).
func (b *backend) handleInfo(w http.ResponseWriter, _ *http.Request) {
	dbState := "nicht konfiguriert"
	if b.dbAddr != "" {
		if b.dbReachable() {
			dbState = "erreichbar"
		} else {
			dbState = "nicht erreichbar"
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"scope": b.scope, "db": dbState, "hinweis": "oeffentliche Aktion, jederzeit erlaubt",
	})
}

// handleAdminAction ist die privilegierte Aktion. Sie wird nur ausgefuehrt,
// wenn JITPE eine aktive Admin-Erhoehung des Benutzers im eigenen Scope
// bestaetigt. Fail-closed: bei jedem Zweifel 403.
func (b *backend) handleAdminAction(w http.ResponseWriter, r *http.Request) {
	user := b.user(r)
	if user == "" {
		errorJSON(w, http.StatusUnauthorized, "kein Benutzer (X-User fehlt)")
		return
	}
	allowed, reason := b.verifyWithJITPE(user)
	if !allowed {
		errorJSON(w, http.StatusForbidden, "Admin-Aktion verweigert: "+reason)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"scope":  b.scope,
		"result": fmt.Sprintf("privilegierte Aktion auf %s ausgefuehrt durch %s", b.scope, user),
	})
}

// verifyWithJITPE ruft den zentralen Dienst auf. Jeder Fehler (Netz, Timeout,
// 403) fuehrt zu "nicht erlaubt".
func (b *backend) verifyWithJITPE(user string) (bool, string) {
	url := fmt.Sprintf("%s/api/verify?user=%s&scope=%s&role=admin", b.jitpeURL, user, b.scope)
	resp, err := b.http.Get(url)
	if err != nil {
		return false, "JITPE nicht erreichbar (fail-closed)"
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return true, ""
	}
	return false, "keine aktive Erhoehung"
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func errorJSON(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
