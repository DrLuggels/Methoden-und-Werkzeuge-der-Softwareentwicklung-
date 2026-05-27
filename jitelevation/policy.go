package jitelevation

import (
	"context"
	"time"
)

// StaticPolicy ist eine einfache, konfigurationsbasierte PolicyEngine. Sie
// dient als Referenz-Implementierung; produktiv kann der Host eine eigene
// Engine (z. B. Open Policy Agent) anbinden.
//
// Das Modell:
//   - Hierarchy ordnet Rollen einen Rang zu. Eine Rolle deckt jede Rolle mit
//     niedrigerem oder gleichem Rang ab.
//   - Allowed legt pro Benutzer fest, welche Rolle er hoechstens anfordern darf
//     und wie lange.
type StaticPolicy struct {
	// Hierarchy bildet Rollenname -> Rang ab (hoeher = maechtiger).
	Hierarchy map[string]int
	// Allowed bildet Benutzer -> erlaubte Erhoehung ab.
	Allowed map[UserID]Allowance
}

// Allowance beschreibt, was ein Benutzer maximal anfordern darf.
type Allowance struct {
	MaxRole     string
	MaxDuration time.Duration
}

// NewStaticPolicy erzeugt eine StaticPolicy mit der ueblichen Harbor-Hierarchie
// (viewer < deployer < editor < owner < admin) als Vorgabe.
func NewStaticPolicy(allowed map[UserID]Allowance) *StaticPolicy {
	return &StaticPolicy{
		Hierarchy: map[string]int{
			"viewer":   1,
			"deployer": 2,
			"editor":   3,
			"owner":    4,
			"admin":    5,
		},
		Allowed: allowed,
	}
}

// Allow erlaubt den Antrag, wenn der Benutzer eingetragen ist und die
// angefragte Rolle seinen Hoechstrang nicht ueberschreitet. Die Dauer wird auf
// das erlaubte Maximum gedeckelt.
func (p *StaticPolicy) Allow(_ context.Context, user UserID, _ string, role string, want time.Duration) (Decision, error) {
	allowance, ok := p.Allowed[user]
	if !ok {
		return Decision{Allow: false}, nil
	}
	if p.Hierarchy[role] == 0 || p.Hierarchy[role] > p.Hierarchy[allowance.MaxRole] {
		return Decision{Allow: false}, nil
	}

	maxDur := allowance.MaxDuration
	if want > 0 && want < maxDur {
		maxDur = want
	}
	return Decision{Allow: true, GrantedRole: role, MaxDuration: maxDur}, nil
}

// Satisfies meldet, ob grantedRole mindestens den Rang von requiredRole hat.
func (p *StaticPolicy) Satisfies(grantedRole, requiredRole string) bool {
	return p.Hierarchy[grantedRole] >= p.Hierarchy[requiredRole] && p.Hierarchy[requiredRole] > 0
}
