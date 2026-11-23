// date.go resolves the optional date argument of the dated tools (day, planning).
// An empty date must mean "today" for the token owner, not an empty string propagated
// to the Garmin API and SQLite (issue #131).
package mcp

import (
	"fmt"
	"strings"
	"time"
)

// resolveDate resolves the optional date of a dated tool. Empty or blank means today in
// the owner's timezone (the server's local zone when the profile holds none); a provided
// date must be a valid YYYY-MM-DD, otherwise the error is returned to the LLM as an
// explicit tool error — the SDK answers with IsError and the message as content. The
// resolved date is always the canonical YYYY-MM-DD form.
func resolveDate(now time.Time, tz, raw string, fr bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return now.In(dateLoc(tz)).Format("2006-01-02"), nil
	}
	d, err := time.Parse("2006-01-02", raw)
	if err != nil {
		example := now.In(dateLoc(tz)).Format("2006-01-02")
		if fr {
			return "", fmt.Errorf("date %q invalide : le format attendu est AAAA-MM-JJ, par exemple %s", raw, example)
		}
		return "", fmt.Errorf("invalid date %q: expected YYYY-MM-DD, e.g. %s", raw, example)
	}
	return d.Format("2006-01-02"), nil
}

// dateLoc loads the profile timezone, falling back to the server's local zone when it is
// missing or unknown.
func dateLoc(tz string) *time.Location {
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	return time.Local
}
