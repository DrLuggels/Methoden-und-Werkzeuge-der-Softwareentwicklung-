package jitelevation

import (
	"context"
	"time"
)

// Clock liefert die aktuelle Zeit. Die Indirektion erlaubt deterministische
// Tests ohne echte Wartezeiten.
type Clock interface {
	Now() time.Time
}

// Store kapselt die Persistenz der Grants sowie den Replay-Schutz des
// Zweitfaktors. Alle Methoden muessen nebenlaeufig sicher sein.
//
// Die Methoden ActivateGrant, RevokeGrant und ExpireDue muessen ihre
// Zustandsuebergaenge atomar ausfuehren (z. B. per "UPDATE ... WHERE
// state = 'pending'" oder SELECT ... FOR UPDATE), damit konkurrierende Aufrufe
// keinen Grant doppelt aktivieren koennen.
type Store interface {
	// CreateGrant legt einen neuen Grant an. Die ID ist bereits gesetzt.
	CreateGrant(ctx context.Context, g Grant) error

	// GetGrant liefert den Grant zur ID oder ErrGrantNotFound.
	GetGrant(ctx context.Context, id GrantID) (Grant, error)

	// ActivateGrant fuehrt den Uebergang pending -> active atomar aus. Die zu
	// gewaehrende Rolle steht bereits im pending-Grant (GrantedRole). Ist der
	// Grant nicht (mehr) pending, liefert die Methode ErrInvalidState.
	ActivateGrant(ctx context.Context, id GrantID, confirmedAt, expiresAt time.Time) error

	// DenyGrant fuehrt den Uebergang pending -> denied atomar aus.
	DenyGrant(ctx context.Context, id GrantID, at time.Time) error

	// RevokeGrant fuehrt den Uebergang active -> revoked atomar aus.
	RevokeGrant(ctx context.Context, id GrantID, by UserID, at time.Time) error

	// ActiveGrants liefert alle aktuell als aktiv markierten Grants eines
	// Benutzers im angegebenen Scope. Zeitliche Gueltigkeit prueft der Aufrufer
	// zusaetzlich ueber Grant.IsActive.
	ActiveGrants(ctx context.Context, user UserID, scope string) ([]Grant, error)

	// ExpireDue markiert alle faelligen Grants als expired und liefert deren
	// IDs zurueck. Faellig sind pending-Grants nach Ablauf der
	// Bestaetigungsfrist und active-Grants nach Ablauf der Gueltigkeit.
	ExpireDue(ctx context.Context, now time.Time) ([]GrantID, error)

	// MarkTOTPUsed vermerkt ein TOTP-Zeitfenster (timestep) als verbraucht.
	// Rueckgabe false bedeutet: das Zeitfenster war fuer diesen Benutzer
	// bereits vergeben (Replay). Die Operation muss atomar (test-and-set) sein.
	MarkTOTPUsed(ctx context.Context, user UserID, timestep uint64) (firstUse bool, err error)
}

// TOTPVerifier prueft den zeitbasierten Einmalcode (RFC 6238) eines Benutzers.
// Die Verwaltung der Seeds (verschluesselte Ablage, QR-Provisionierung) liegt
// bewusst in der Adapter-Implementierung, nicht im Kern.
type TOTPVerifier interface {
	// Verify prueft code fuer user. Bei Erfolg liefert die Methode das
	// Zeitfenster (timestep = floor(unixtime / period)), zu dem der Code
	// gehoert; der Kern nutzt es fuer den Replay-Schutz. Bei falschem Code
	// liefert sie ErrInvalidTOTP.
	Verify(ctx context.Context, user UserID, code string) (timestep uint64, err error)
}

// AuditSink nimmt Protokollereignisse entgegen. Eine produktive Implementierung
// schreibt sie append-only und verkettet sie ueber AuditEvent.ChainHash.
// Emit muss nebenlaeufig sicher sein, da es aus parallelen Aufrufen stammen kann.
type AuditSink interface {
	Emit(ctx context.Context, ev AuditEvent) error
}

// Decision ist das Ergebnis einer Policy-Pruefung.
type Decision struct {
	Allow       bool
	GrantedRole string        // tatsaechlich zu gewaehrende Rolle (kann <= angefragt sein)
	MaxDuration time.Duration // obere Schranke; 0 bedeutet "Modul-Default verwenden"
}

// PolicyEngine entscheidet ueber Antraege und kennt die Rollen-Hierarchie des
// Host-Systems.
type PolicyEngine interface {
	// Allow entscheidet, ob user im Scope die Rolle role fuer die gewuenschte
	// Dauer want erhalten darf.
	Allow(ctx context.Context, user UserID, scope, role string, want time.Duration) (Decision, error)

	// Satisfies meldet, ob die gewaehrte Rolle die geforderte Rolle abdeckt.
	// Hier lebt die Rollen-Hierarchie (z. B. owner deckt editor ab).
	Satisfies(grantedRole, requiredRole string) bool
}
