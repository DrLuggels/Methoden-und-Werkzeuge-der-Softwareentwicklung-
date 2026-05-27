package jitelevation

import "errors"

// Sentinel-Errors des Kerns. Aufrufer pruefen sie mit errors.Is. Store- und
// Adapter-Fehler werden mit %w umschlossen weitergereicht, damit die konkrete
// Ursache erhalten bleibt.
var (
	// ErrPolicyDenied bedeutet: die PolicyEngine erlaubt diesen Antrag nicht.
	ErrPolicyDenied = errors.New("jitelevation: durch policy abgelehnt")

	// ErrGrantNotFound wird zurueckgegeben, wenn keine Erhoehung mit der
	// angegebenen GrantID existiert.
	ErrGrantNotFound = errors.New("jitelevation: grant nicht gefunden")

	// ErrInvalidState bedeutet: die angeforderte Transition ist aus dem
	// aktuellen Zustand des Grants nicht erlaubt (z. B. Confirm auf einen
	// bereits aktiven oder abgelaufenen Grant).
	ErrInvalidState = errors.New("jitelevation: ungueltiger zustandsuebergang")

	// ErrConfirmExpired bedeutet: die Bestaetigungsfrist (confirm deadline)
	// des pending-Grants ist verstrichen.
	ErrConfirmExpired = errors.New("jitelevation: bestaetigungsfrist abgelaufen")

	// ErrInvalidTOTP bedeutet: der uebergebene Zweitfaktor war falsch.
	ErrInvalidTOTP = errors.New("jitelevation: zweitfaktor ungueltig")

	// ErrTOTPReplay bedeutet: der Zweitfaktor wurde fuer dieses Zeitfenster
	// bereits verwendet. Schuetzt gegen die Wiedereinspielung abgefangener
	// Codes.
	ErrTOTPReplay = errors.New("jitelevation: zweitfaktor bereits verwendet (replay)")

	// ErrInvalidConfig wird vom Konstruktor New zurueckgegeben, wenn
	// Abhaengigkeiten fehlen oder die Konfiguration widerspruechlich ist.
	ErrInvalidConfig = errors.New("jitelevation: ungueltige konfiguration")
)
