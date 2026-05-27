package jitelevation

import "time"

// State ist der Zustand eines Grants in der State-Machine (siehe doc.go).
type State string

const (
	// StatePending: Antrag angelegt, wartet auf Zwei-Faktor-Bestaetigung.
	StatePending State = "pending"
	// StateActive: bestaetigt und derzeit gueltig.
	StateActive State = "active"
	// StateExpired: Frist abgelaufen (Bestaetigungs- oder Gueltigkeitsfrist).
	StateExpired State = "expired"
	// StateRevoked: vorzeitig widerrufen.
	StateRevoked State = "revoked"
	// StateDenied: durch Policy oder fehlgeschlagenen Zweitfaktor abgelehnt.
	StateDenied State = "denied"
)

// Grant beschreibt eine einzelne Rechte-Erhoehung ueber ihren gesamten
// Lebenszyklus. Die Zeitfelder sind nur in den jeweils passenden Zustaenden
// gesetzt (z. B. ExpiresAt erst ab StateActive).
type Grant struct {
	ID            GrantID
	User          UserID
	Scope         string // z. B. "project:42" oder "core"; vom Host interpretiert
	RequestedRole string
	GrantedRole   string // von der Policy zugesagte Rolle; ab StateActive wirksam
	State         State
	Reason        string // Begruendung des Antragstellers (fuer das Audit)

	Duration        time.Duration // gewuenschte Gueltigkeitsdauer ab Bestaetigung
	RequestedAt     time.Time
	ConfirmDeadline time.Time // bis hierhin muss bestaetigt werden
	ConfirmedAt     time.Time // gesetzt bei Confirm
	ExpiresAt       time.Time // gesetzt bei Confirm
	RevokedAt       time.Time // gesetzt bei Revoke
	RevokedBy       UserID    // gesetzt bei Revoke
}

// IsActive meldet, ob der Grant zum Zeitpunkt now tatsaechlich gueltig ist.
// Die Pruefung ist bewusst konjunktiv: nur ein als aktiv markierter und
// zeitlich noch gueltiger Grant zaehlt. Damit ist die Methode fail-closed.
func (g Grant) IsActive(now time.Time) bool {
	return g.State == StateActive && now.Before(g.ExpiresAt)
}
