package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	jit "github.com/drluggels/jitelevation"
	"github.com/drluggels/jitelevation/adapter/filestore"
	"github.com/drluggels/jitelevation/adapter/totp"
)

// generateTOTP berechnet den gueltigen 6-stelligen Code (RFC 6238, SHA-1, 30 s)
// fuer den Test. Bewusst unabhaengig vom Adapter implementiert, damit der Test
// die Verifier-Implementierung gegen eine zweite, eigenstaendige HOTP-Berechnung
// prueft.
func generateTOTP(secret []byte, at time.Time) string {
	counter := uint64(at.Unix()) / 30
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := (uint32(sum[off]&0x7f)<<24 | uint32(sum[off+1])<<16 |
		uint32(sum[off+2])<<8 | uint32(sum[off+3])) % 1000000
	return fmt.Sprintf("%06d", code)
}

func newTestServer(t *testing.T) (*server, *demoSeeds) {
	t.Helper()
	dir := t.TempDir()
	store, err := filestore.Open(dir + "/state.json")
	if err != nil {
		t.Fatalf("filestore: %v", err)
	}
	seeds := newDemoSeeds()
	audit, err := newFileAudit(dir + "/audit.log")
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	policy := jit.NewStaticPolicy(map[jit.UserID]jit.Allowance{
		"alice": {MaxRole: "admin", MaxDuration: time.Hour},
		"bob":   {MaxRole: "viewer", MaxDuration: 10 * time.Minute},
	})
	mod, err := jit.New(jit.Dependencies{
		Store: store, TOTP: totp.New(seeds, totp.Config{Skew: 1}), Audit: audit, Policy: policy,
	}, jit.Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return &server{mod: mod, store: store, seeds: seeds}, seeds
}

// post/get senden JSON und liefern Status + dekodierten Body.
func do(t *testing.T, c *http.Client, method, url string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, url, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func TestFullstackElevationFlow(t *testing.T) {
	srv, seeds := newTestServer(t)
	ts := httptest.NewServer(srv.routes())
	defer ts.Close()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}

	// 1) Anmelden als alice.
	if code, _ := do(t, c, "POST", ts.URL+"/api/login", map[string]string{"username": "alice"}); code != 200 {
		t.Fatalf("login: status %d, will 200", code)
	}

	// 2) Vor der Erhoehung ist die geschuetzte Aktion gesperrt (fail-closed).
	if code, _ := do(t, c, "GET", ts.URL+"/api/admin/data", nil); code != 403 {
		t.Fatalf("admin/data vor Erhoehung: status %d, will 403", code)
	}

	// 3) Erhoehung beantragen.
	code, body := do(t, c, "POST", ts.URL+"/api/elevate/request",
		map[string]any{"role": "admin", "reason": "test", "duration_minutes": 30})
	if code != 200 {
		t.Fatalf("request: status %d, will 200", code)
	}
	grantID, _ := body["grant_id"].(string)
	if grantID == "" {
		t.Fatal("request: keine grant_id erhalten")
	}

	// 4) Mit gueltigem TOTP bestaetigen (Step-Up).
	seed, _ := seeds.Seed(context.Background(), "alice")
	totpCode := generateTOTP(seed, time.Now())
	if code, _ := do(t, c, "POST", ts.URL+"/api/elevate/confirm",
		map[string]string{"grant_id": grantID, "code": totpCode}); code != 200 {
		t.Fatalf("confirm: status %d, will 200", code)
	}

	// 5) Jetzt ist die geschuetzte Aktion erlaubt.
	if code, _ := do(t, c, "GET", ts.URL+"/api/admin/data", nil); code != 200 {
		t.Fatalf("admin/data nach Erhoehung: status %d, will 200", code)
	}

	// 6) Falscher Code wird abgewiesen (neuer Antrag).
	_, body2 := do(t, c, "POST", ts.URL+"/api/elevate/request",
		map[string]any{"role": "admin", "reason": "test2", "duration_minutes": 30})
	gid2, _ := body2["grant_id"].(string)
	if code, _ := do(t, c, "POST", ts.URL+"/api/elevate/confirm",
		map[string]string{"grant_id": gid2, "code": "000000"}); code != 401 {
		t.Fatalf("confirm falscher Code: status %d, will 401", code)
	}

	// 7) Widerruf entzieht die Rechte sofort wieder.
	if code, _ := do(t, c, "POST", ts.URL+"/api/elevate/revoke",
		map[string]string{"grant_id": grantID}); code != 200 {
		t.Fatalf("revoke: status %d, will 200", code)
	}
	if code, _ := do(t, c, "GET", ts.URL+"/api/admin/data", nil); code != 403 {
		t.Fatalf("admin/data nach Revoke: status %d, will 403", code)
	}
}

func TestPolicyDeniesBobAdmin(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.routes())
	defer ts.Close()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}

	do(t, c, "POST", ts.URL+"/api/login", map[string]string{"username": "bob"})
	// bob darf hoechstens viewer -> admin-Antrag wird von der Policy abgelehnt.
	if code, _ := do(t, c, "POST", ts.URL+"/api/elevate/request",
		map[string]any{"role": "admin", "reason": "x", "duration_minutes": 10}); code != 403 {
		t.Fatalf("request bob/admin: status %d, will 403", code)
	}
}
