# JIT-Elevation-Modul — Visuelle Übersicht

Diese Datei sammelt die Diagramme, die das Konzept des Moduls beschreiben.
Sie dient als visuelle Grundlage für die Projektarbeit.

---

## 1. Architektur-Übersicht (System-Kontext)

Zeigt, wie das Modul in einen Host (z. B. Harbor) eingebettet wird und
welche Verantwortung wo liegt.

```mermaid
flowchart TB
    User["Benutzer"]
    GA["Google Authenticator<br/>(TOTP, RFC 6238)"]

    subgraph host["Host-System (z. B. Harbor)"]
        direction TB
        FE["Frontend"]
        BE["Backend HTTP-Server"]
        FE --> BE
    end

    subgraph modul["JIT-Elevation-Modul (Go-Package)"]
        direction TB
        HTTP["HTTP-Handler /elevate/*"]
        CORE["Kern<br/>Request · Confirm · Check<br/>Revoke · ExpireDue"]
        HTTP --> CORE
    end

    subgraph ports["Ports (Interfaces, vom Host implementiert)"]
        direction LR
        STORE["Store"]
        TV["TOTPVerifier"]
        AS["AuditSink"]
        POL["PolicyEngine"]
        CLK["Clock"]
    end

    DB[("MariaDB<br/>jit_grants · jit_audit")]

    User -->|interagiert| FE
    User -.->|liest Code ab| GA
    BE -->|importiert + ruft| HTTP
    CORE -->|nutzt| ports
    STORE --> DB
```

**Kernaussage:** Das Modul kennt den Host nicht — es spricht ausschließlich
gegen Interfaces. Der Host liefert konkrete Implementierungen. Damit ist
das Modul wiederverwendbar und unabhängig vom Trägersystem testbar.

---

## 2. State-Machine eines Grants

Beschreibt den vollständigen Lebenszyklus einer Rechte-Erhöhung —
von Antrag bis zum terminalen Zustand.

```mermaid
stateDiagram-v2
    direction LR
    [*] --> pending: Request (Policy erlaubt)
    pending --> active: Confirm (TOTP korrekt)
    active --> revoked: Revoke
    active --> expired: Ablauf (30 min)
    pending --> denied: Confirm (TOTP falsch)
    pending --> expired: Frist (5 min)
    denied --> [*]
    revoked --> [*]
    expired --> [*]
```

**Kernaussage:** Alle Endzustände sind terminal. Ein expired/revoked/denied
Grant kann nicht reaktiviert werden — das ist die zentrale fail-closed-
Eigenschaft des Modells.

---

## 3. Hauptflow — Request, Confirm, Use, Expire

Zeigt den vollständigen Ablauf inklusive Step-Up-Authentication mit TOTP.

```mermaid
sequenceDiagram
    autonumber
    actor U as User
    participant GA as Authenticator
    participant A as App
    participant M as JIT-Modul
    participant DB as MariaDB
    participant L as Audit-Log

    Note over U,L: Phase 1 - Antrag
    U->>A: "Admin-Modus, 30 min, Grund: ..."
    A->>M: Request(user, scope, role, reason)
    M->>M: PolicyEngine: darf User?
    M->>DB: INSERT grant<br/>state=pending<br/>confirm_deadline=now+5m
    M->>L: emit("requested")
    M-->>A: grantID
    A-->>U: "Bitte 2FA-Code eingeben"

    Note over U,L: Phase 2 - Step-Up
    U->>GA: Authenticator öffnen
    GA-->>U: 6-stelliger Code
    U->>A: Code eingeben
    A->>M: Confirm(grantID, code)
    M->>DB: SELECT FOR UPDATE grant
    M->>M: TOTP verifizieren<br/>+ Replay-Check
    M->>DB: UPDATE state=active<br/>expires_at=now+30m
    M->>L: emit("confirmed")
    M-->>A: Grant aktiv
    A-->>U: "Admin-Modus für 30 min aktiv"

    Note over U,L: Phase 3 - Privilegierte Aktion
    U->>A: DELETE /api/admin/...
    A->>M: Check(user, scope, "admin")
    M->>DB: SELECT aktive Grants
    M->>L: emit("used")
    M-->>A: erlaubt
    A-->>U: 200 OK

    Note over U,L: Phase 4 - Ablauf (Background)
    M->>DB: SELECT WHERE expires_at < NOW()
    M->>DB: UPDATE state=expired
    M->>L: emit("expired")
```

**Kernaussage:** Phase 2 ist der Step-Up-Moment. Ohne
erfolgreichen TOTP entsteht kein aktiver Grant — keine Hintertür,
keine Sonderfälle.

---

## 4. Datenmodell (ER-Diagramm)

Die drei Tabellen, die das Modul anlegt (Grants, Audit-Log und die
verbrauchten TOTP-Zeitfenster für den Replay-Schutz). Die Audit-Tabelle
hat eine Hash-Chain für Integrität.

```mermaid
erDiagram
    JIT_GRANTS ||--o{ JIT_AUDIT : protokolliert

    JIT_GRANTS {
        bigint id PK
        bigint user_id
        string scope
        string requested_role
        string granted_role
        enum state "pending|active|expired|revoked|denied"
        string reason
        bigint duration_sec
        datetime requested_at
        datetime confirm_deadline
        datetime confirmed_at
        datetime expires_at
        datetime revoked_at
        bigint revoked_by
    }

    JIT_AUDIT {
        bigint id PK
        bigint grant_id FK
        int seq "Sequenz pro Grant"
        string event "requested|confirmed|denied|used|expired|revoked|replay_blocked"
        string actor
        datetime occurred_at
        json details
        string prev_hash "SHA-256 des vorigen Eintrags"
        string this_hash "SHA-256(prev || event || actor || ts || details)"
    }

    JIT_TOTP_USED {
        bigint user_id "Replay-Schutz (benutzerweit)"
        bigint timestep "verbrauchtes TOTP-Zeitfenster"
    }
```

**Kernaussage:** Die Hash-Chain in `jit_audit` macht jede nachträgliche
Manipulation eines Log-Eintrags erkennbar — der nächste Eintrag bricht
die Kette.

---

## 5. Vertrauensgrenzen (Trust Boundaries)

Wo verlässt eine Information eine Vertrauenszone? Das ist die Grundlage
für das STRIDE-Threat-Model.

```mermaid
flowchart LR
    subgraph z1["Zone 1: Benutzergerät (untrusted)"]
        B["Browser"]
        P["Smartphone mit Authenticator"]
    end

    subgraph z2["Zone 2: Netzwerk (untrusted)"]
        N["Internet / LAN"]
    end

    subgraph z3["Zone 3: App-Prozess (semi-trusted)"]
        APP["Host-App + Modul-Code"]
    end

    subgraph z4["Zone 4: Datenpersistenz (trusted)"]
        DB[("MariaDB")]
        V["TOTP-Seeds<br/>(verschlüsselt)"]
    end

    B -->|HTTPS / TLS 1.3| N
    N -->|HTTPS / TLS 1.3| APP
    APP -->|TLS oder lokaler Socket| DB
    APP -->|TLS oder lokaler Socket| V

    P -.->|TOTP-Seed einmalig<br/>via QR im Setup| APP
```

**Kernaussage:** Drei Grenzen müssen kryptografisch geschützt sein:
Browser↔Backend (TLS), Backend↔DB (TLS oder Unix-Socket), und der
TOTP-Seed verlässt nie die Datenpersistenz im Klartext.

---

## Hinweise zur Nutzung in der Arbeit

| Diagramm | Eignet sich für Kapitel |
|---|---|
| 1. Architektur | Konzept / Modul-Architektur, Adapter-Pattern |
| 2. State-Machine | Konzept / Grant-Lebenszyklus, fail-closed Argument |
| 3. Sequenzdiagramm | Konzept / Ablauf, Sicherheit / Step-Up-Moment |
| 4. ER-Diagramm | Implementierung / Datenmodell |
| 5. Trust Boundaries | Sicherheitsanalyse / STRIDE-Vorbereitung |
