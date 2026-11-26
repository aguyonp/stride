package updates

import (
	"strconv"
	"strings"
)

// Version parsing for the release check (§22). The tags of this repository are SemVer without the
// `v` prefix (`VERSION` = tag), but `v4.7.0` is accepted: GitHub users tag both ways.

// Comparable reports whether the running version can be compared at all: a local build (`dev`) or
// anything unparsable means no alert, never a false one.
func Comparable(v string) bool {
	_, ok := normalize(v)
	return ok
}

// Newer reports whether tag is strictly newer than current. Both must normalize; a pre-release or
// a build suffix is not a stable release and never counts as newer.
func Newer(tag, current string) bool {
	a, ok := normalize(tag)
	if !ok {
		return false
	}
	b, ok := normalize(current)
	if !ok {
		return false
	}
	if a == b {
		return false
	}
	am, an, ap := parts(a)
	bm, bn, bp := parts(b)
	switch {
	case am != bm:
		return am > bm
	case an != bn:
		return an > bn
	default:
		return ap > bp
	}
}

// normalize trims the `v` prefix, rejects pre-releases and build metadata, and returns the
// canonical major.minor.patch.
func normalize(v string) (string, bool) {
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), "v"))
	if v == "" || strings.ContainsAny(v, "-+") {
		return "", false
	}
	core := v
	// A trailing `.0` or missing patch is tolerated (`4.7` → `4.7.0`).
	fields := strings.Split(core, ".")
	if len(fields) < 2 || len(fields) > 3 {
		return "", false
	}
	for len(fields) < 3 {
		fields = append(fields, "0")
	}
	for _, f := range fields {
		if f == "" {
			return "", false
		}
		if _, err := strconv.Atoi(f); err != nil {
			return "", false
		}
	}
	return strings.Join(fields, "."), true
}

func parts(v string) (int, int, int) {
	f := strings.Split(v, ".")
	maj, _ := strconv.Atoi(f[0])
	min, _ := strconv.Atoi(f[1])
	pat, _ := strconv.Atoi(f[2])
	return maj, min, pat
}
