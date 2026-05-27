package jitelevation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Vorgabewerte fuer die Konfiguration.
const (
	// DefaultConfirmTTL: so lange bleibt ein Antrag bestaetigbar.
	DefaultConfirmTTL = 5 * time.Minute
	// DefaultGrantTTL: Gueltigkeitsdauer einer Erhoehung, wenn weder Antrag
	// noch Policy etwas anderes vorgeben.
	DefaultGrantTTL = 30 * time.Minute
	// DefaultMaxGrantTTL: harte Obergrenze fuer die Gueltigkeitsdauer.
	DefaultMaxGrantTTL = 4 * time.Hour
)

// Dependencies buendelt die vom Host bereitzustellenden Adapter. Clock ist
// optional; fehlt sie, verwendet das Modul die Systemuhr.
type Dependencies struct {
	Store  Store
	TOTP   TOTPVerifier
	Audit  AuditSink
	Policy PolicyEngine
	Clock  Clock
}

// Config steuert die Fristen. Nullwerte werden in New durch die Default*-
// Konstanten ersetzt.
type Config struct {
	ConfirmTTL      time.Duration
	DefaultGrantTTL time.Duration
	MaxGrantTTL     time.Duration
}

// Module ist der Kern der JIT-Elevation. Eine Instanz ist nach New
// unveraenderlich und nebenlaeufig nutzbar (sofern die Adapter es sind).
type Module struct {
	store  Store
	totp   TOTPVerifier
	audit  AuditSink
	policy PolicyEngine
	clock  Clock
	cfg    Config
}

// RequestInput beschreibt einen Erhoehungsantrag.
type RequestInput struct {
	User          UserID
	Scope         string
	RequestedRole string
	Reason        string
	// Duration ist die gewuenschte Gueltigkeitsdauer. 0 bedeutet
	// "Modul-Default". Policy und MaxGrantTTL deckeln den Wert.
	Duration time.Duration
}

// New erzeugt ein Module. Es liefert ErrInvalidConfig, wenn ein Pflicht-Adapter
// fehlt oder die Fristen widerspruechlich sind.
func New(deps Dependencies, cfg Config) (*Module, error) {
	if deps.Store == nil || deps.TOTP == nil || deps.Audit == nil || deps.Policy == nil {
		return nil, fmt.Errorf("%w: Store, TOTP, Audit und Policy sind pflicht", ErrInvalidConfig)
	}
	if cfg.ConfirmTTL <= 0 {
		cfg.ConfirmTTL = DefaultConfirmTTL
	}
	if cfg.DefaultGrantTTL <= 0 {
		cfg.DefaultGrantTTL = DefaultGrantTTL
	}
	if cfg.MaxGrantTTL <= 0 {
		cfg.MaxGrantTTL = DefaultMaxGrantTTL
	}
	if cfg.DefaultGrantTTL > cfg.MaxGrantTTL {
		return nil, fmt.Errorf("%w: DefaultGrantTTL > MaxGrantTTL", ErrInvalidConfig)
	}
	clock := deps.Clock
	if clock == nil {
		clock = systemClock{}
	}
	return &Module{
		store:  deps.Store,
		totp:   deps.TOTP,
		audit:  deps.Audit,
		policy: deps.Policy,
		clock:  clock,
		cfg:    cfg,
	}, nil
}

// Request legt einen Antrag an. Lehnt die Policy ab, liefert die Methode
// ErrPolicyDenied, ohne einen aktivierbaren Grant zu erzeugen. Andernfalls
// entsteht ein pending-Grant, der per Confirm bestaetigt werden muss.
func (m *Module) Request(ctx context.Context, in RequestInput) (GrantID, error) {
	decision, err := m.policy.Allow(ctx, in.User, in.Scope, in.RequestedRole, in.Duration)
	if err != nil {
		return "", fmt.Errorf("policy-pruefung: %w", err)
	}
	now := m.clock.Now()
	if !decision.Allow {
		// Ablehnungspfad: best-effort protokollieren, dann verweigern.
		m.auditBestEffort(ctx, AuditEvent{
			Event:      EventDenied,
			Actor:      in.User,
			OccurredAt: now,
			Details:    map[string]string{"scope": in.Scope, "rolle": in.RequestedRole, "grund": "policy"},
		})
		return "", ErrPolicyDenied
	}

	duration := decision.MaxDuration
	if duration <= 0 {
		duration = m.cfg.DefaultGrantTTL
	}
	if duration > m.cfg.MaxGrantTTL {
		duration = m.cfg.MaxGrantTTL
	}

	id, err := newGrantID()
	if err != nil {
		return "", fmt.Errorf("id-erzeugung: %w", err)
	}
	g := Grant{
		ID:              id,
		User:            in.User,
		Scope:           in.Scope,
		RequestedRole:   in.RequestedRole,
		GrantedRole:     decision.GrantedRole,
		State:           StatePending,
		Reason:          in.Reason,
		Duration:        duration,
		RequestedAt:     now,
		ConfirmDeadline: now.Add(m.cfg.ConfirmTTL),
	}
	if err := m.store.CreateGrant(ctx, g); err != nil {
		return "", fmt.Errorf("grant anlegen: %w", err)
	}
	m.auditBestEffort(ctx, AuditEvent{
		GrantID:    id,
		Event:      EventRequested,
		Actor:      in.User,
		OccurredAt: now,
		Details:    map[string]string{"scope": in.Scope, "rolle": decision.GrantedRole},
	})
	return id, nil
}

// Confirm ist der Step-Up-Schritt: der Antrag wird nur dann aktiv, wenn der
// Zweitfaktor stimmt und der Code nicht bereits verwendet wurde. Jeder
// Fehlerpfad fuehrt zu KEINER Erhoehung (fail-closed).
func (m *Module) Confirm(ctx context.Context, id GrantID, totpCode string) (Grant, error) {
	g, err := m.store.GetGrant(ctx, id)
	if err != nil {
		return Grant{}, err // u. a. ErrGrantNotFound
	}
	if g.State != StatePending {
		return Grant{}, ErrInvalidState
	}
	now := m.clock.Now()
	if now.After(g.ConfirmDeadline) {
		return Grant{}, ErrConfirmExpired
	}

	// 1) Zweitfaktor pruefen. Ein falscher Code verbrennt den Antrag; ein
	//    Infrastrukturfehler laesst ihn bestehen (der Benutzer darf erneut).
	timestep, err := m.totp.Verify(ctx, g.User, totpCode)
	if errors.Is(err, ErrInvalidTOTP) {
		m.denyBestEffort(ctx, id, g.User, now, "totp_falsch")
		return Grant{}, ErrInvalidTOTP
	}
	if err != nil {
		return Grant{}, fmt.Errorf("totp-pruefung: %w", err)
	}

	// 2) Replay-Schutz: das Zeitfenster atomar als verbraucht markieren. Ein
	//    bereits verwendeter Code verwirft den Antrag NICHT: laufen zwei
	//    Bestaetigungen desselben Antrags parallel, soll derjenige Konkurrent,
	//    der das Zeitfenster zuerst belegt, regulaer aktivieren koennen. Wuerde
	//    der Replay-Verlierer hier den Grant ablehnen, sabotierte er den
	//    Gewinner. Der Antrag bleibt pending und laeuft sonst regulaer ab.
	firstUse, err := m.store.MarkTOTPUsed(ctx, g.User, timestep)
	if err != nil {
		return Grant{}, fmt.Errorf("replay-pruefung: %w", err)
	}
	if !firstUse {
		m.auditBestEffort(ctx, AuditEvent{
			GrantID:    id,
			Event:      EventReplayBlocked,
			Actor:      g.User,
			OccurredAt: now,
		})
		return Grant{}, ErrTOTPReplay
	}

	// 3) Atomar aktivieren. Ein konkurrierender Confirm verliert hier mit
	//    ErrInvalidState.
	expiresAt := now.Add(g.Duration)
	if err := m.store.ActivateGrant(ctx, id, now, expiresAt); err != nil {
		return Grant{}, err
	}

	// confirmed ist sicherheitskritisch -> harte Audit-Pflicht.
	if err := m.audit.Emit(ctx, AuditEvent{
		GrantID:    id,
		Event:      EventConfirmed,
		Actor:      g.User,
		OccurredAt: now,
		Details:    map[string]string{"scope": g.Scope, "rolle": g.GrantedRole, "gueltig_bis": expiresAt.UTC().Format(time.RFC3339)},
	}); err != nil {
		return Grant{}, fmt.Errorf("audit (confirmed): %w", err)
	}

	g.State = StateActive
	g.ConfirmedAt = now
	g.ExpiresAt = expiresAt
	return g, nil
}

// Check meldet, ob der Benutzer im Scope aktuell ueber einen aktiven Grant
// verfuegt, der die geforderte Rolle abdeckt. Der Aufrufer MUSS bei (false, _)
// und bei jedem Fehler den Zugriff verweigern: die Methode ist fail-closed und
// liefert im Zweifel false.
func (m *Module) Check(ctx context.Context, user UserID, scope, requiredRole string) (bool, error) {
	grants, err := m.store.ActiveGrants(ctx, user, scope)
	if err != nil {
		return false, fmt.Errorf("aktive grants lesen: %w", err)
	}
	now := m.clock.Now()
	for _, g := range grants {
		if !g.IsActive(now) || !m.policy.Satisfies(g.GrantedRole, requiredRole) {
			continue
		}
		// used ist sicherheitskritisch -> harte Audit-Pflicht.
		if err := m.audit.Emit(ctx, AuditEvent{
			GrantID:    g.ID,
			Event:      EventUsed,
			Actor:      user,
			OccurredAt: now,
			Details:    map[string]string{"scope": scope, "rolle": requiredRole},
		}); err != nil {
			return false, fmt.Errorf("audit (used): %w", err)
		}
		return true, nil
	}
	return false, nil
}

// Revoke nimmt eine aktive Erhoehung vorzeitig zurueck.
func (m *Module) Revoke(ctx context.Context, id GrantID, by UserID) error {
	now := m.clock.Now()
	if err := m.store.RevokeGrant(ctx, id, by, now); err != nil {
		return err
	}
	m.auditBestEffort(ctx, AuditEvent{
		GrantID:    id,
		Event:      EventRevoked,
		Actor:      by,
		OccurredAt: now,
	})
	return nil
}

// ExpireDue markiert alle faelligen Grants als abgelaufen und protokolliert
// dies. Aufgerufen wird die Methode periodisch von einem Hintergrund-Ticker.
// Sie liefert die Anzahl der abgelaufenen Grants.
func (m *Module) ExpireDue(ctx context.Context) (int, error) {
	now := m.clock.Now()
	ids, err := m.store.ExpireDue(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("faellige grants: %w", err)
	}
	for _, id := range ids {
		m.auditBestEffort(ctx, AuditEvent{
			GrantID:    id,
			Event:      EventExpired,
			Actor:      "system",
			OccurredAt: now,
		})
	}
	return len(ids), nil
}

// auditBestEffort protokolliert ein Ereignis auf einem unkritischen Pfad. Ein
// Schreibfehler wird bewusst verworfen, weil die zugehoerige Operation ohnehin
// keinen Zugang gewaehrt (denied/expired/revoked) bzw. nur einen noch
// nicht nutzbaren pending-Grant betrifft (requested).
func (m *Module) auditBestEffort(ctx context.Context, ev AuditEvent) {
	_ = m.audit.Emit(ctx, ev)
}

// denyBestEffort markiert einen Grant als abgelehnt und protokolliert dies.
func (m *Module) denyBestEffort(ctx context.Context, id GrantID, actor UserID, at time.Time, reason string) {
	_ = m.store.DenyGrant(ctx, id, at)
	m.auditBestEffort(ctx, AuditEvent{
		GrantID:    id,
		Event:      EventDenied,
		Actor:      actor,
		OccurredAt: at,
		Details:    map[string]string{"grund": reason},
	})
}

// newGrantID erzeugt eine 128-bit-Zufalls-ID. Zufaellige IDs verhindern, dass
// fremde Grant-IDs erraten oder durchgezaehlt werden koennen.
func newGrantID() (GrantID, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return GrantID(hex.EncodeToString(b[:])), nil
}

// systemClock ist die produktive Zeitquelle.
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }
