// Package filestore implementiert jitelevation.Store mit Persistenz in einer
// JSON-Datei. Er dient als realistischerer Adapter als der In-Memory-Store:
// die Daten ueberleben einen Neustart. Alle Zugriffe sind ueber eine Mutex
// serialisiert; nach jeder Mutation wird der Zustand atomar (Temp-Datei +
// Rename) auf die Platte geschrieben. Fuer hohe Last ist eine echte Datenbank
// (z. B. MariaDB) vorzuziehen; das Adapter-Muster macht den Austausch trivial.
package filestore

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	jit "github.com/drluggels/jitelevation"
)

// Store haelt Grants und verbrauchte TOTP-Zeitfenster und persistiert sie.
type Store struct {
	mu       sync.Mutex
	path     string
	grants   map[jit.GrantID]jit.Grant
	usedTOTP map[totpKey]struct{}
}

type totpKey struct {
	user     jit.UserID
	timestep uint64
}

// persistedState ist die serialisierbare Form des Zustands.
type persistedState struct {
	Grants   map[jit.GrantID]jit.Grant `json:"grants"`
	UsedTOTP []usedEntry               `json:"used_totp"`
}

type usedEntry struct {
	User     jit.UserID `json:"user"`
	Timestep uint64     `json:"timestep"`
}

// Open laedt einen Store aus path. Existiert die Datei nicht, wird ein leerer
// Store angelegt.
func Open(path string) (*Store, error) {
	s := &Store{
		path:     path,
		grants:   make(map[jit.GrantID]jit.Grant),
		usedTOTP: make(map[totpKey]struct{}),
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var ps persistedState
	if err := json.Unmarshal(data, &ps); err != nil {
		return nil, err
	}
	if ps.Grants != nil {
		s.grants = ps.Grants
	}
	for _, e := range ps.UsedTOTP {
		s.usedTOTP[totpKey{user: e.User, timestep: e.Timestep}] = struct{}{}
	}
	return s, nil
}

// flush schreibt den aktuellen Zustand atomar auf die Platte. Aufrufer halten
// bereits die Mutex.
func (s *Store) flush() error {
	ps := persistedState{Grants: s.grants}
	for k := range s.usedTOTP {
		ps.UsedTOTP = append(ps.UsedTOTP, usedEntry{User: k.user, Timestep: k.timestep})
	}
	data, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) CreateGrant(_ context.Context, g jit.Grant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.grants[g.ID] = g
	return s.flush()
}

func (s *Store) GetGrant(_ context.Context, id jit.GrantID) (jit.Grant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.grants[id]
	if !ok {
		return jit.Grant{}, jit.ErrGrantNotFound
	}
	return g, nil
}

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
	return s.flush()
}

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
	return s.flush()
}

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
	return s.flush()
}

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
			due = !now.Before(g.ExpiresAt)
		}
		if due {
			g.State = jit.StateExpired
			s.grants[id] = g
			expired = append(expired, id)
		}
	}
	if len(expired) > 0 {
		if err := s.flush(); err != nil {
			return nil, err
		}
	}
	return expired, nil
}

func (s *Store) MarkTOTPUsed(_ context.Context, user jit.UserID, timestep uint64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := totpKey{user: user, timestep: timestep}
	if _, used := s.usedTOTP[k]; used {
		return false, nil
	}
	s.usedTOTP[k] = struct{}{}
	return true, s.flush()
}

// ListAll liefert alle Grants eines Benutzers (jeden Zustand), sortiert ist
// nicht garantiert. Praktisch fuer die Status-Anzeige der Demo-App.
func (s *Store) ListAll(_ context.Context, user jit.UserID) ([]jit.Grant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []jit.Grant
	for _, g := range s.grants {
		if g.User == user {
			out = append(out, g)
		}
	}
	return out, nil
}
