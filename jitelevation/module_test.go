package jitelevation_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	jit "github.com/drluggels/jitelevation"
	"github.com/drluggels/jitelevation/adapter/memory"
)

// --- Test-Doubles -----------------------------------------------------------

// fakeClock ist eine manuell steuerbare Zeitquelle.
type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// fakeTOTP akzeptiert genau validCode und ordnet jeden gueltigen Code demselben
// Zeitfenster timestep zu (so laesst sich Replay deterministisch testen).
type fakeTOTP struct {
	validCode string
	timestep  uint64
}

func (f *fakeTOTP) Verify(_ context.Context, _ jit.UserID, code string) (uint64, error) {
	if code != f.validCode {
		return 0, jit.ErrInvalidTOTP
	}
	return f.timestep, nil
}

// recordingAudit sammelt alle Ereignisse und kann optional Fehler simulieren.
// Der Mutex ist noetig, weil AuditSink.Emit nebenlaeufig aufgerufen wird.
type recordingAudit struct {
	mu     sync.Mutex
	events []jit.AuditEvent
	err    error
}

func (r *recordingAudit) Emit(_ context.Context, ev jit.AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.events = append(r.events, ev)
	return nil
}

// brokenStore laesst ActiveGrants fehlschlagen, um fail-closed zu pruefen.
type brokenStore struct{ *memory.Store }

func (brokenStore) ActiveGrants(context.Context, jit.UserID, string) ([]jit.Grant, error) {
	return nil, errors.New("datenbank nicht erreichbar")
}

// --- Test-Harness -----------------------------------------------------------

type harness struct {
	mod   *jit.Module
	clock *fakeClock
	totp  *fakeTOTP
	audit *recordingAudit
	store *memory.Store
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	clock := &fakeClock{t: time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)}
	totp := &fakeTOTP{validCode: "123456", timestep: 1000}
	audit := &recordingAudit{}
	store := memory.NewStore()
	policy := jit.NewStaticPolicy(map[jit.UserID]jit.Allowance{
		"alice": {MaxRole: "admin", MaxDuration: time.Hour},
		"bob":   {MaxRole: "viewer", MaxDuration: 10 * time.Minute},
	})
	mod, err := jit.New(jit.Dependencies{
		Store: store, TOTP: totp, Audit: audit, Policy: policy, Clock: clock,
	}, jit.Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return &harness{mod, clock, totp, audit, store}
}

// --- Tests ------------------------------------------------------------------

func TestRequestConfirmCheck_HappyPath(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	id, err := h.mod.Request(ctx, jit.RequestInput{
		User: "alice", Scope: "project:1", RequestedRole: "admin", Reason: "incident-response",
	})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	g, err := h.mod.Confirm(ctx, id, "123456")
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if g.State != jit.StateActive {
		t.Fatalf("State = %q, will %q", g.State, jit.StateActive)
	}

	// admin deckt editor ab (Rollen-Hierarchie).
	ok, err := h.mod.Check(ctx, "alice", "project:1", "editor")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !ok {
		t.Fatal("Check = false, will true")
	}
}

func TestConfirm_WrongTOTP_DeniesAndFailsClosed(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	id, err := h.mod.Request(ctx, jit.RequestInput{User: "alice", Scope: "project:1", RequestedRole: "admin"})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	if _, err := h.mod.Confirm(ctx, id, "000000"); !errors.Is(err, jit.ErrInvalidTOTP) {
		t.Fatalf("Confirm err = %v, will ErrInvalidTOTP", err)
	}

	ok, err := h.mod.Check(ctx, "alice", "project:1", "admin")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if ok {
		t.Fatal("Check = true nach fehlgeschlagenem TOTP, will false")
	}
}

func TestConfirm_ReplayedCodeRejected(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	id1, _ := h.mod.Request(ctx, jit.RequestInput{User: "alice", Scope: "project:1", RequestedRole: "admin"})
	id2, _ := h.mod.Request(ctx, jit.RequestInput{User: "alice", Scope: "project:2", RequestedRole: "admin"})

	if _, err := h.mod.Confirm(ctx, id1, "123456"); err != nil {
		t.Fatalf("erster Confirm: %v", err)
	}
	// Gleicher Code, gleiches Zeitfenster -> Replay.
	if _, err := h.mod.Confirm(ctx, id2, "123456"); !errors.Is(err, jit.ErrTOTPReplay) {
		t.Fatalf("zweiter Confirm err = %v, will ErrTOTPReplay", err)
	}
}

func TestConfirm_AfterDeadlineRejected(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	id, _ := h.mod.Request(ctx, jit.RequestInput{User: "alice", Scope: "project:1", RequestedRole: "admin"})
	h.clock.advance(jit.DefaultConfirmTTL + time.Second)

	if _, err := h.mod.Confirm(ctx, id, "123456"); !errors.Is(err, jit.ErrConfirmExpired) {
		t.Fatalf("Confirm err = %v, will ErrConfirmExpired", err)
	}
}

func TestExpireDue_DeactivatesActiveGrant(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	id, _ := h.mod.Request(ctx, jit.RequestInput{
		User: "alice", Scope: "project:1", RequestedRole: "admin", Duration: 30 * time.Minute,
	})
	if _, err := h.mod.Confirm(ctx, id, "123456"); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	h.clock.advance(31 * time.Minute)
	n, err := h.mod.ExpireDue(ctx)
	if err != nil {
		t.Fatalf("ExpireDue: %v", err)
	}
	if n != 1 {
		t.Fatalf("ExpireDue = %d, will 1", n)
	}

	ok, err := h.mod.Check(ctx, "alice", "project:1", "admin")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if ok {
		t.Fatal("Check = true nach Ablauf, will false")
	}
}

func TestRequest_PolicyDenied(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// bob darf hoechstens viewer, fragt aber admin.
	if _, err := h.mod.Request(ctx, jit.RequestInput{User: "bob", Scope: "project:1", RequestedRole: "admin"}); !errors.Is(err, jit.ErrPolicyDenied) {
		t.Fatalf("Request(bob, admin) err = %v, will ErrPolicyDenied", err)
	}
	// mallory ist gar nicht eingetragen.
	if _, err := h.mod.Request(ctx, jit.RequestInput{User: "mallory", Scope: "project:1", RequestedRole: "viewer"}); !errors.Is(err, jit.ErrPolicyDenied) {
		t.Fatalf("Request(mallory) err = %v, will ErrPolicyDenied", err)
	}
}

func TestRevoke_DeactivatesGrant(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	id, _ := h.mod.Request(ctx, jit.RequestInput{User: "alice", Scope: "project:1", RequestedRole: "admin"})
	if _, err := h.mod.Confirm(ctx, id, "123456"); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if err := h.mod.Revoke(ctx, id, "security-officer"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	ok, err := h.mod.Check(ctx, "alice", "project:1", "admin")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if ok {
		t.Fatal("Check = true nach Revoke, will false")
	}
}

func TestCheck_FailsClosedOnStoreError(t *testing.T) {
	policy := jit.NewStaticPolicy(map[jit.UserID]jit.Allowance{
		"alice": {MaxRole: "admin", MaxDuration: time.Hour},
	})
	mod, err := jit.New(jit.Dependencies{
		Store:  brokenStore{memory.NewStore()},
		TOTP:   &fakeTOTP{validCode: "123456", timestep: 1},
		Audit:  &recordingAudit{},
		Policy: policy,
		Clock:  &fakeClock{t: time.Now()},
	}, jit.Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ok, err := mod.Check(context.Background(), "alice", "project:1", "admin")
	if err == nil {
		t.Fatal("Check err = nil bei Store-Fehler, will Fehler")
	}
	if ok {
		t.Fatal("Check = true bei Store-Fehler, will false (fail-closed)")
	}
}

func TestConfirm_ConcurrentlySafe(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	id, _ := h.mod.Request(ctx, jit.RequestInput{User: "alice", Scope: "project:1", RequestedRole: "admin"})

	// Viele gleichzeitige Bestaetigungen desselben Antrags. Egal ob ein
	// Konkurrent am Replay-Schutz oder an der atomaren Aktivierung scheitert:
	// hoechstens einer darf erfolgreich sein. Mit "go test -race" ausgefuehrt
	// belegt der Test zugleich die Datenrennen-Freiheit.
	const n = 20
	var wg sync.WaitGroup
	var successes int64
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if _, err := h.mod.Confirm(ctx, id, "123456"); err == nil {
				atomic.AddInt64(&successes, 1)
			}
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Fatalf("erfolgreiche Confirms = %d, will genau 1", successes)
	}
}

func TestNew_RejectsMissingDependencies(t *testing.T) {
	if _, err := jit.New(jit.Dependencies{}, jit.Config{}); !errors.Is(err, jit.ErrInvalidConfig) {
		t.Fatalf("New err = %v, will ErrInvalidConfig", err)
	}
}

func TestAuditChain_DetectsTampering(t *testing.T) {
	e1 := jit.AuditEvent{GrantID: "g1", Event: jit.EventRequested, Actor: "alice", OccurredAt: time.Unix(1000, 0)}
	e2 := jit.AuditEvent{GrantID: "g1", Event: jit.EventConfirmed, Actor: "alice", OccurredAt: time.Unix(1005, 0)}

	h1 := e1.ChainHash(jit.GenesisHash)
	h2 := e2.ChainHash(h1)

	// Ein Angreifer veraendert den ersten Eintrag nachtraeglich.
	tampered := e1
	tampered.Actor = "mallory"
	h1Tampered := tampered.ChainHash(jit.GenesisHash)

	if h1Tampered == h1 {
		t.Fatal("Hash unveraendert trotz Manipulation des Eintrags")
	}
	if e2.ChainHash(h1Tampered) == h2 {
		t.Fatal("Kette nicht gebrochen: Folgeeintrag haengt nicht am Vorgaenger-Hash")
	}
}
