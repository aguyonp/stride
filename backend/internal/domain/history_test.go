package domain

import "testing"

func TestHistoryWindowsCoverTheRangeBackwards(t *testing.T) {
	w := historyWindows("2025-01-01", "2026-01-10")
	if len(w) != 3 || w[0][1] != "2026-01-10" || w[0][0] != "2025-07-15" || w[2][0] != "2025-01-01" {
		t.Fatalf("windows: %v", w)
	}
	for i := 1; i < len(w); i++ {
		if addDays(w[i][1], 1) != w[i-1][0] {
			t.Fatalf("gap between %v and %v", w[i], w[i-1])
		}
	}
	if len(historyWindows("2026-02-01", "2026-01-31")) != 0 {
		t.Fatal("empty range")
	}
}

func TestDateFilter(t *testing.T) {
	for q, want := range map[string]int{"14 avril": 2, "1er mai 2025": 3, "avril 2025": 2, "14/04/19": 3, "14/04": 2, "2025-04": 1, "marathon": 0, "14/13": 0} {
		cond, args := dateFilter(q)
		if len(args) != want || (want == 0) != (cond == "") {
			t.Fatalf("%q: %q %v", q, cond, args)
		}
	}
	if _, args := dateFilter("14/04/19"); args[2] != "2019" {
		t.Fatalf("two-digit year: %v", args)
	}
}
