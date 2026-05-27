# jitelevation

Just-in-Time-Privilege-Elevation als eigenständiges Go-Modul: zeitlich
befristete, per Zwei-Faktor (TOTP, RFC 6238) bestätigte Rechte-Erhöhungen
für Benutzer eines beliebigen Host-Systems.

Statt Benutzern dauerhaft erhöhte Rechte zu geben, beantragen sie eine
Erhöhung nur für den Moment, in dem sie sie brauchen. Der Antrag wird erst
nach einer erneuten Zwei-Faktor-Bestätigung (Step-Up-Authentication)
wirksam und läuft nach einer festen Frist automatisch ab.

## Architektur

Das Modul folgt dem Ports-and-Adapters-Muster. Der Kern (`Module`) kennt das
Host-System nicht, sondern spricht ausschließlich gegen Interfaces:

| Port | Aufgabe | Beispiel-Adapter |
| --- | --- | --- |
| `Store` | Persistenz der Grants + Replay-Schutz | `adapter/memory`, produktiv MariaDB |
| `TOTPVerifier` | Prüfung des Zweitfaktors | `pquerna/otp` |
| `AuditSink` | manipulationssicheres Protokoll | append-only Log mit Hash-Kette |
| `PolicyEngine` | wer darf was wie lange anfordern | `StaticPolicy`, produktiv OPA |
| `Clock` | Zeitquelle (in Tests ersetzbar) | Systemuhr |

Dadurch ist der Kern unabhängig vom Trägersystem testbar und in jede
Go-Anwendung integrierbar.

## Lebenszyklus eines Grants

```
pending --Confirm(TOTP ok)--> active --Ablauf-->  expired
   |                            |
   |--Frist/TOTP-Fehler-->      |--Revoke------>   revoked
          denied/expired
```

Alle Endzustände sind terminal: ein abgelaufener, widerrufener oder
abgelehnter Grant wird nie wieder aktiv (fail-closed).

## Integration (Beispiel)

```go
package main

import (
	"context"
	"log"
	"net/http"
	"time"

	jit "github.com/drluggels/jitelevation"
	"github.com/drluggels/jitelevation/adapter/memory"
)

func main() {
	mod, err := jit.New(jit.Dependencies{
		Store:  memory.NewStore(),      // produktiv: MariaDB-Adapter
		TOTP:   myTOTPVerifier{},        // z. B. auf pquerna/otp
		Audit:  myAuditSink{},           // append-only + Hash-Kette
		Policy: jit.NewStaticPolicy(map[jit.UserID]jit.Allowance{
			"alice": {MaxRole: "admin", MaxDuration: time.Hour},
		}),
	}, jit.Config{}) // Nullwerte -> sichere Defaults (5 min / 30 min / 4 h)
	if err != nil {
		log.Fatal(err)
	}

	// Privilegierten Endpunkt schützen: bei (false, _) oder Fehler -> 403.
	http.HandleFunc("/api/admin/delete", func(w http.ResponseWriter, r *http.Request) {
		ok, err := mod.Check(r.Context(), currentUser(r), "project:1", "admin")
		if err != nil || !ok {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		// ... privilegierte Aktion ...
	})

	// Hintergrund-Ticker lässt abgelaufene Grants verfallen.
	go func() {
		for range time.Tick(time.Minute) {
			_, _ = mod.ExpireDue(context.Background())
		}
	}()

	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

## Tests

```bash
go test ./... -race -cover
```

Die Tests decken Happy-Path, falschen Zweitfaktor, Replay, Fristablauf,
Widerruf, Policy-Ablehnung, fail-closed bei Store-Fehler, Nebenläufigkeit
und die Integrität der Audit-Hash-Kette ab.
