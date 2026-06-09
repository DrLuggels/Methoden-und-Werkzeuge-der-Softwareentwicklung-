package main

import (
	"context"
	"encoding/base32"
	"fmt"
	"net/url"

	jit "github.com/drluggels/jitelevation"
)

// demoSeeds haelt feste TOTP-Seeds fuer die Demo-Benutzer. Feste Seeds im
// Quelltext sind ausschliesslich fuer eine Demonstration vertretbar; produktiv
// werden Seeds pro Benutzer zufaellig erzeugt und verschluesselt abgelegt.
type demoSeeds struct {
	seeds map[jit.UserID][]byte
}

func newDemoSeeds() *demoSeeds {
	return &demoSeeds{
		seeds: map[jit.UserID][]byte{
			"alice": []byte("alice-demo-seed-1234"),
			"bob":   []byte("bob-demo-seed-567890"),
		},
	}
}

func (d *demoSeeds) Seed(_ context.Context, user jit.UserID) ([]byte, error) {
	s, ok := d.seeds[user]
	if !ok {
		return nil, fmt.Errorf("kein TOTP-Seed fuer %q", user)
	}
	return s, nil
}

// provisioning liefert otpauth-URL und Base32-Secret fuer die Authenticator-App.
func (d *demoSeeds) provisioning(user jit.UserID) (otpauth, secret string, err error) {
	s, ok := d.seeds[user]
	if !ok {
		return "", "", fmt.Errorf("kein TOTP-Seed fuer %q", user)
	}
	secret = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(s)
	label := url.PathEscape("JITPE:" + string(user))
	otpauth = fmt.Sprintf(
		"otpauth://totp/%s?secret=%s&issuer=JITPE&period=30&digits=6&algorithm=SHA1",
		label, secret)
	return otpauth, secret, nil
}
