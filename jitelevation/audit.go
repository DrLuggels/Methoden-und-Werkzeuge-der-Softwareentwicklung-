package jitelevation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Audit-Ereignistypen. Jeder Zustandsuebergang und jede Nutzung eines aktiven
// Grants erzeugt genau ein Ereignis.
const (
	EventRequested = "requested"
	EventConfirmed = "confirmed"
	EventDenied    = "denied"
	EventUsed      = "used"
	EventExpired   = "expired"
	EventRevoked   = "revoked"
	// EventReplayBlocked: ein bereits verwendeter Zweitfaktor-Code wurde
	// abgewiesen. Dies ist ein Sicherheitsereignis ohne Zustandsuebergang.
	EventReplayBlocked = "replay_blocked"
)

// AuditEvent ist ein einzelner, unveraenderlicher Protokolleintrag.
type AuditEvent struct {
	GrantID    GrantID
	Event      string // eine der Event*-Konstanten
	Actor      UserID
	OccurredAt time.Time
	Details    map[string]string
}

// GenesisHash ist der Vorgaenger-Hash des allerersten Kettenglieds.
const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// ChainHash berechnet den SHA-256-Verkettungshash dieses Ereignisses auf Basis
// des Hashes seines Vorgaengers. Die Ereignisse bilden so eine Hash-Kette:
// wird ein gespeicherter Eintrag nachtraeglich veraendert, passt sein Hash
// nicht mehr zum prevHash des Folgeeintrags und die Manipulation faellt auf.
//
// Die Serialisierung ist deterministisch (Detail-Schluessel sortiert), damit
// derselbe Eintrag stets denselben Hash ergibt.
func (e AuditEvent) ChainHash(prevHash string) string {
	var b strings.Builder
	b.WriteString(prevHash)
	b.WriteByte('|')
	b.WriteString(string(e.GrantID))
	b.WriteByte('|')
	b.WriteString(e.Event)
	b.WriteByte('|')
	b.WriteString(string(e.Actor))
	b.WriteByte('|')
	b.WriteString(e.OccurredAt.UTC().Format(time.RFC3339Nano))
	b.WriteByte('|')
	for _, k := range sortedKeys(e.Details) {
		fmt.Fprintf(&b, "%s=%s;", k, e.Details[k])
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
