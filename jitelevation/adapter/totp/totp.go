// Package totp implementiert einen jitelevation.TOTPVerifier nach RFC 6238
// (zeitbasiertes Einmalpasswort) auf Basis von RFC 4226 (HOTP). Es verwendet
// ausschliesslich die Standardbibliothek und ist gegen die offiziellen
// Testvektoren beider RFCs validiert (siehe totp_test.go).
//
// Hinweis zur Einordnung: Die eigenstaendige Implementierung dient der
// Veranschaulichung des Verfahrens. Produktiv empfiehlt sich eine auditierte
// Bibliothek (z. B. github.com/pquerna/otp). Die kryptografische Primitive
// (HMAC-SHA1) stammt aus crypto/hmac und wird nicht selbst nachgebaut; der Code
// vergleicht zudem zeitkonstant (crypto/subtle), um Timing-Angriffe zu
// vermeiden.
package totp

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	jit "github.com/drluggels/jitelevation"
)

// SeedSource liefert den entschluesselten TOTP-Seed eines Benutzers (die rohen
// Schluesselbytes). Produktiv entschluesselt die Implementierung ihn aus dem
// SecretStore (z. B. mit AES-256-GCM); der Seed verlaesst die Persistenz nur
// fluechtig.
type SeedSource interface {
	Seed(ctx context.Context, user jit.UserID) ([]byte, error)
}

// Config steuert die TOTP-Parameter. Nullwerte werden in New durch die ueblichen
// Vorgaben ersetzt (30 s Periode, 6 Ziffern).
type Config struct {
	Period uint64           // Sekunden pro Zeitfenster
	Digits int              // Anzahl der Code-Ziffern
	Skew   uint64           // erlaubte Fenster vor/nach (Uhren-Drift)
	Now    func() time.Time // Zeitquelle; nil bedeutet time.Now (testbar)
}

// Verifier prueft TOTP-Codes gegen die Seeds aus einer SeedSource.
type Verifier struct {
	seeds  SeedSource
	period uint64
	digits int
	skew   uint64
	now    func() time.Time
}

// New erzeugt einen Verifier mit sinnvollen Vorgaben.
func New(seeds SeedSource, cfg Config) *Verifier {
	if cfg.Period == 0 {
		cfg.Period = 30
	}
	if cfg.Digits == 0 {
		cfg.Digits = 6
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Verifier{
		seeds:  seeds,
		period: cfg.Period,
		digits: cfg.Digits,
		skew:   cfg.Skew,
		now:    now,
	}
}

// Verify prueft code fuer user. Bei Erfolg liefert die Methode das Zeitfenster
// (timestep), zu dem der akzeptierte Code gehoert; jitelevation nutzt es als
// Replay-Schluessel. Ein unbekannter oder falscher Code ergibt
// jit.ErrInvalidTOTP. Zur Uhren-Drift-Toleranz werden neben dem aktuellen auch
// bis zu skew benachbarte Fenster geprueft.
func (v *Verifier) Verify(ctx context.Context, user jit.UserID, code string) (uint64, error) {
	secret, err := v.seeds.Seed(ctx, user)
	if err != nil {
		return 0, fmt.Errorf("totp: seed laden: %w", err)
	}
	code = strings.TrimSpace(code)

	current := uint64(v.now().Unix()) / v.period
	low := uint64(0)
	if current > v.skew {
		low = current - v.skew
	}
	high := current + v.skew

	for ts := low; ts <= high; ts++ {
		expected := hotp(secret, ts, v.digits)
		// Zeitkonstanter Vergleich gegen Timing-Angriffe.
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return ts, nil
		}
	}
	return 0, jit.ErrInvalidTOTP
}

// hotp berechnet das HMAC-basierte Einmalpasswort nach RFC 4226 fuer den
// Zaehler counter und gibt es als nullgepolsterte Dezimalziffernfolge der Laenge
// digits zurueck.
func hotp(secret []byte, counter uint64, digits int) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)

	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)

	// Dynamic Truncation (RFC 4226, Abschnitt 5.3): das niederwertigste
	// Nibble waehlt den 4-Byte-Ausschnitt, dessen oberstes Bit maskiert wird.
	offset := sum[len(sum)-1] & 0x0f
	truncated := (uint32(sum[offset]&0x7f) << 24) |
		(uint32(sum[offset+1]) << 16) |
		(uint32(sum[offset+2]) << 8) |
		uint32(sum[offset+3])

	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, truncated%mod)
}
