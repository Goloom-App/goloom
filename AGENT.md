# Workflow: Goloon Development

## Steps

1. **Planung:** Erstelle einen detaillierten Plan für die anstehenden Änderungen.
2. **Implementierung:** Lasse die Änderungen durch `gemini cli` (lokal unter `~/.local/bin/gemini`) oder `cursor` implementieren.
3. **Build & Test:** Überprüfe, ob die Builds erfolgreich durchlaufen. Bei Fehlern zurück zu Schritt 2.
4. **Sicherheits-Check:** Nutze Gemini oder Cursor, um den Code auf Sicherheitslücken zu prüfen.
    - Kritische Lücken: Sofort beheben.
    - Nicht-kritische Lücken: Als Issue im Forgejo-Projekt anlegen.
5. **Abschluss:** Commit & Push der Änderungen. Überprüfe anschließend, ob der Build (CI) erfolgreich durchgelaufen ist.

## Werkzeuge

- **Gemini CLI:** `~/.local/bin/gemini`
- **Forgejo API:** Zur Erstellung von Issues bei nicht-kritischen Sicherheitsfunden.
