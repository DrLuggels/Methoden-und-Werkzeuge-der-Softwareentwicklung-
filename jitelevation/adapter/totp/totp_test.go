package totp

import (
	"context"
	"errors"
	"testing"
	"time"

	jit "github.com/drluggels/jitelevation"
)

// staticSeed liefert fuer jeden Benutzer denselben Seed.
type staticSeed []byte

func (s staticSeed) Seed(context.Context, jit.UserID) ([]byte, error) { return s, nil }

// rfcSeed ist der ASCII-Seed aus RFC 4226/6238 ("12345678901234567890").
var rfcSeed = []byte("12345678901234567890")

// TestHOTP_RFC4226Vectors prueft die HOTP-Kernfunktion gegen die Testvektoren
// aus RFC 4226, Anhang D (6 Ziffern, SHA-1).
func TestHOTP_RFC4226Vectors(t *testing.T) {
	want := []string{
		"755224", "287082", "359152", "969429", "338314",
		"254676", "287922", "162583", "399871", "520489",
	}
	for counter, exp := range want {
		if got := hotp(rfcSeed, uint64(counter), 6); got != exp {
			t.Errorf("hotp(counter=%d) = %s, will %s", counter, got, exp)
		}
	}
}

// TestVerify_RFC6238Vectors prueft den TOTP-Verifier gegen die Testvektoren aus
// RFC 6238, Anhang B (8 Ziffern, SHA-1, 30 s Periode).
func TestVerify_RFC6238Vectors(t *testing.T) {
	vectors := []struct {
		unixTime int64
		code     string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
		{20000000000, "65353130"},
	}
	for _, vec := range vectors {
		at := vec.unixTime
		v := New(staticSeed(rfcSeed), Config{
			Digits: 8,
			Period: 30,
			Skew:   0,
			Now:    func() time.Time { return time.Unix(at, 0) },
		})
		ts, err := v.Verify(context.Background(), "user", vec.code)
		if err != nil {
			t.Errorf("T=%d: Verify err = %v, will Erfolg", vec.unixTime, err)
			continue
		}
		if want := uint64(at) / 30; ts != want {
			t.Errorf("T=%d: timestep = %d, will %d", vec.unixTime, ts, want)
		}
	}
}

func TestVerify_RejectsWrongCode(t *testing.T) {
	v := New(staticSeed(rfcSeed), Config{
		Digits: 8,
		Now:    func() time.Time { return time.Unix(59, 0) },
	})
	if _, err := v.Verify(context.Background(), "user", "00000000"); !errors.Is(err, jit.ErrInvalidTOTP) {
		t.Fatalf("Verify err = %v, will ErrInvalidTOTP", err)
	}
}

func TestVerify_ToleratesClockSkew(t *testing.T) {
	// Code gehoert zu T=59 (Zeitfenster 1); die Verifier-Uhr steht bei T=89
	// (Zeitfenster 2). Mit Skew=1 muss der Code dennoch akzeptiert werden.
	v := New(staticSeed(rfcSeed), Config{
		Digits: 8,
		Skew:   1,
		Now:    func() time.Time { return time.Unix(89, 0) },
	})
	ts, err := v.Verify(context.Background(), "user", "94287082")
	if err != nil {
		t.Fatalf("Verify err = %v, will Erfolg trotz Drift", err)
	}
	if ts != 1 {
		t.Fatalf("timestep = %d, will 1 (Fenster des Codes)", ts)
	}
}
