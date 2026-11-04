package frfmt

import (
	"math"
	"testing"
)

func TestFormats(t *testing.T) {
	nan := math.NaN()
	cases := []struct{ got, want string }{
		{Dec(8.149, 1), "8,1"},
		{Dec(7.6, 1), "7,6"},
		{Dec(-0.04, 1), "0,0"},
		{Dec(nan, 1), "N/D"},
		{Int(8432), "8 432"},
		{Int(1234567), "1 234 567"},
		{Int(999), "999"},
		{Int(-1500), "-1 500"},
		{Signed(3.24, 1), "+3,2"},
		{Signed(-1, 1), "-1,0"},
		{Signed(4, 0), "+4"},
		{Duration(2530), "42:10"},
		{Duration(3730), "1:02:10"},
		{Duration(60), "1:00"},
		{Duration(59.6), "1:00"},
		{Duration(nan), "N/D"},
		{Pace(343.4), "5:43/km"},
		{Pace(360), "6:00/km"},
		{PaceShort(215), "3:35"},
		{Pace(0), "N/D"},
		{SleepHM(4920), "1h22"},
		{SleepHM(25920), "7h12"},
		{SleepHM(3360), "0h56"},
		{Km(7020, 2), "7,02 km"},
		{Km(6100, 1), "6,1 km"},
		{DateTime("2026-11-02T18:30:12"), "02/11/2026 18:30"},
		{DateTime("2026-11-02 18:30:12"), "02/11/2026 18:30"},
		{Date("2026-11-03"), "03/11/2026"},
		{DayMonth("2026-10-28"), "28/10"},
		{Num(9, 1), "9"},
		{Num(1.5, 1), "1,5"},
		{Num(30, 1), "30"},
		{Unit(nan, "ms"), "N/D"},
		{Unit(248.4, "ms"), "248 ms"},
		{UnitDec(1.049, 2, "m"), "1,05 m"},
	}
	for i, c := range cases {
		if c.got != c.want {
			t.Errorf("cas %d : obtenu %q, attendu %q", i, c.got, c.want)
		}
	}
}
