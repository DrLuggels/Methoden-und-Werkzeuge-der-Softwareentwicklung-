// Package jitelevation implementiert Just-in-Time-Privilege-Elevation:
// zeitlich befristete, per Zwei-Faktor (TOTP, RFC 6238) bestaetigte
// Rechte-Erhoehungen fuer Benutzer eines beliebigen Host-Systems.
//
// # Idee
//
// Statt Benutzern dauerhaft erhoehte Rechte zu geben, beantragt ein Benutzer
// eine Erhoehung nur fuer den Moment, in dem er sie braucht. Der Antrag wird
// erst nach einer erneuten Zwei-Faktor-Bestaetigung (Step-Up-Authentication)
// wirksam und laeuft nach einer festen Frist automatisch ab.
//
// # Architektur
//
// Das Paket folgt dem Ports-and-Adapters-Muster (hexagonale Architektur).
// Der Kern (Module) kennt das Host-System nicht, sondern spricht ausschliesslich
// gegen Interfaces (siehe ports.go):
//
//   - Store        Persistenz der Grants und des Replay-Schutzes
//   - TOTPVerifier  Pruefung des Zweitfaktors
//   - AuditSink     manipulationssicheres Protokoll
//   - PolicyEngine  Entscheidung, wer was wie lange anfordern darf
//   - Clock         Zeitquelle (in Tests ersetzbar)
//
// Dadurch ist der Kern unabhaengig vom Traegersystem testbar und in jede
// Go-Anwendung integrierbar. Ein In-Memory-Adapter liegt unter adapter/memory.
//
// # Lebenszyklus eines Grants
//
//	pending --Confirm(TOTP ok)--> active --Ablauf-->  expired
//	   |                            |
//	   |--Frist/TOTP-Fehler-->      |--Revoke------>   revoked
//	          denied/expired
//
// Alle Endzustaende sind terminal: ein abgelaufener, widerrufener oder
// abgelehnter Grant wird nie wieder aktiv (fail-closed).
package jitelevation
