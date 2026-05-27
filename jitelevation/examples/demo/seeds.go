package main

import (
	"context"
	"encoding/base32"
	"fmt"
	"net/url"

	jit "github.com/drluggels/jitelevation"
)

// demoSeeds ist eine SeedSource mit festen TOTP-Seeds fuer die Demo-Benutzer.
// ACHTUNG: im Quelltext hinterlegte, feste Seeds sind ausschliesslich fuer eine
// Demonstration vertretbar. Produktiv werden Seeds pro Benutzer zufaellig
// erzeugt und verschluesselt (z. B. mit AES-256-GCM) abgelegt.
type demoSeeds struct {
	seeds map[jit.UserID][]byte
}

func newDemoSeeds() *demoSeeds {
	return &demoSeeds{
		seeds: map[jit.UserID][]byte{
			"alice": []byte("alice-demo-seed-1234"), // 20 Byte
			"bob":   []byte("bob-demo-seed-567890"), // 20 Byte
		},
	}
}

// Seed liefert den rohen TOTP-Seed eines Benutzers (erfuellt totp.SeedSource).
func (d *demoSeeds) Seed(_ context.Context, user jit.UserID) ([]byte, error) {
	s, ok := d.seeds[user]
	if !ok {
		return nil, fmt.Errorf("kein TOTP-Seed fuer %q", user)
	}
	return s, nil
}

// provisioning erzeugt die otpauth://-URL und den Base32-Secret, mit denen ein
// Benutzer den Faktor in einer Authenticator-App (Google Authenticator, Authy,
// ...) einrichtet.
func (d *demoSeeds) provisioning(user jit.UserID) (otpauth, secret string, err error) {
	s, ok := d.seeds[user]
	if !ok {
		return "", "", fmt.Errorf("kein TOTP-Seed fuer %q", user)
	}
	secret = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(s)
	label := url.PathEscape("JIT-Demo:" + string(user))
	otpauth = fmt.Sprintf(
		"otpauth://totp/%s?secret=%s&issuer=JIT-Demo&period=30&digits=6&algorithm=SHA1",
		label, secret)
	return otpauth, secret, nil
}
