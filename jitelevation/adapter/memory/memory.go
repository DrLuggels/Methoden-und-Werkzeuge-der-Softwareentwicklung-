// Package memory stellt eine In-Memory-Implementierung von jitelevation.Store
// bereit. Sie ist fuer Tests und lokale Demonstrationen gedacht; die Daten
// gehen beim Prozessende verloren. Alle Methoden sind ueber eine einzige
// Mutex serialisiert und damit nebenlaeufig sicher; die geforderte Atomaritaet
// der Zustandsuebergaenge ergibt sich unmittelbar daraus.
package memory

import (
	"context"
	"sync"
	"time"

	jit "github.com/drluggels/jitelevation"
)

// Store haelt Grants und verbrauchte TOTP-Zeitfenster im Speicher.
type Store struct {
	mu       sync.Mutex
	grants   map[jit.GrantID]jit.Grant
	usedTOTP map[totpKey]struct{}
}

type totpKey struct {
	user     jit.UserID
	timestep uint64
}

// NewStore erzeugt einen leeren Store.
func NewStore() *Store {
	return &Store{
		grants:   make(map[jit.GrantID]jit.Grant),
		usedTOTP: make(map[totpKey]struct{}),
	}
}

// CreateGrant legt einen neuen Grant ab.
func (s *Store) CreateGrant(_ context.Context, g jit.Grant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.grants[g.ID] = g
	return nil
}

// GetGrant liefert den Grant zur ID oder jit.ErrGrantNotFound.
func (s *Store) GetGrant(_ context.Context, id jit.GrantID) (jit.Grant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.grants[id]
	if !ok {
		return jit.Grant{}, jit.ErrGrantNotFound
	}
	return g, nil
}

// ActivateGrant setzt einen pending-Grant auf active.
func (s *Store) ActivateGrant(_ context.Context, id jit.GrantID, confirmedAt, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.grants[id]
	if !ok {
		return jit.ErrGrantNotFound
	}
	if g.State != jit.StatePending {
		return jit.ErrInvalidState
	}
	g.State = jit.StateActive
	g.ConfirmedAt = confirmedAt
	g.ExpiresAt = expiresAt
	s.grants[id] = g
	return nil
}

// DenyGrant setzt einen pending-Grant auf denied.
func (s *Store) DenyGrant(_ context.Context, id jit.GrantID, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.grants[id]
	if !ok {
		return jit.ErrGrantNotFound
	}
	if g.State != jit.StatePending {
		return jit.ErrInvalidState
	}
	g.State = jit.StateDenied
	s.grants[id] = g
	return nil
}

// RevokeGrant setzt einen active-Grant auf revoked.
func (s *Store) RevokeGrant(_ context.Context, id jit.GrantID, by jit.UserID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.grants[id]
	if !ok {
		return jit.ErrGrantNotFound
	}
	if g.State != jit.StateActive {
		return jit.ErrInvalidState
	}
	g.State = jit.StateRevoked
	g.RevokedAt = at
	g.RevokedBy = by
	s.grants[id] = g
	return nil
}

// ActiveGrants liefert alle als active markierten Grants des Benutzers im Scope.
func (s *Store) ActiveGrants(_ context.Context, user jit.UserID, scope string) ([]jit.Grant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []jit.Grant
	for _, g := range s.grants {
		if g.User == user && g.Scope == scope && g.State == jit.StateActive {
			out = append(out, g)
		}
	}
	return out, nil
}

// ExpireDue markiert faellige pending- und active-Grants als expired.
func (s *Store) ExpireDue(_ context.Context, now time.Time) ([]jit.GrantID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var expired []jit.GrantID
	for id, g := range s.grants {
		var due bool
		switch g.State {
		case jit.StatePending:
			due = now.After(g.ConfirmDeadline)
		case jit.StateActive:
			due = !now.Before(g.ExpiresAt) // now >= ExpiresAt
		}
		if due {
			g.State = jit.StateExpired
			s.grants[id] = g
			expired = append(expired, id)
		}
	}
	return expired, nil
}

// MarkTOTPUsed vermerkt ein Zeitfenster als verbraucht (test-and-set).
func (s *Store) MarkTOTPUsed(_ context.Context, user jit.UserID, timestep uint64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := totpKey{user: user, timestep: timestep}
	if _, used := s.usedTOTP[k]; used {
		return false, nil
	}
	s.usedTOTP[k] = struct{}{}
	return true, nil
}
