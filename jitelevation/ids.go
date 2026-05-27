package jitelevation

// UserID identifiziert einen Benutzer im Host-System. Das Modul behandelt den
// Wert als undurchsichtig (opaque) und interpretiert ihn nicht.
type UserID string

// GrantID identifiziert eine einzelne Rechte-Erhoehung. Sie wird beim Anlegen
// vom Kern als 128-bit-Zufallswert erzeugt und ist fuer den gesamten
// Lebenszyklus stabil.
type GrantID string
