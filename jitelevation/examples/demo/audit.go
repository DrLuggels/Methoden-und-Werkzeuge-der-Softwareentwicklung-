package main

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	jit "github.com/drluggels/jitelevation"
)

// fileAudit schreibt jedes Audit-Ereignis als JSON-Zeile in eine Datei und
// verkettet die Eintraege ueber jit.AuditEvent.ChainHash. Wird eine Zeile
// nachtraeglich veraendert, passt ihr Hash nicht mehr zum prev_hash der
// Folgezeile -- die Manipulation faellt auf.
type fileAudit struct {
	mu       sync.Mutex
	f        *os.File
	lastHash string
}

func newFileAudit(path string) (*fileAudit, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &fileAudit{f: f, lastHash: jit.GenesisHash}, nil
}

type auditLine struct {
	GrantID    jit.GrantID       `json:"grant_id"`
	Event      string            `json:"event"`
	Actor      jit.UserID        `json:"actor"`
	OccurredAt string            `json:"occurred_at"`
	Details    map[string]string `json:"details,omitempty"`
	PrevHash   string            `json:"prev_hash"`
	ThisHash   string            `json:"this_hash"`
}

// Emit erfuellt jitelevation.AuditSink und ist nebenlaeufig sicher.
func (a *fileAudit) Emit(_ context.Context, ev jit.AuditEvent) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	this := ev.ChainHash(a.lastHash)
	data, err := json.Marshal(auditLine{
		GrantID:    ev.GrantID,
		Event:      ev.Event,
		Actor:      ev.Actor,
		OccurredAt: ev.OccurredAt.UTC().Format(time.RFC3339Nano),
		Details:    ev.Details,
		PrevHash:   a.lastHash,
		ThisHash:   this,
	})
	if err != nil {
		return err
	}
	if _, err := a.f.Write(append(data, '\n')); err != nil {
		return err
	}
	a.lastHash = this
	return nil
}
