package analysis

import (
	"fmt"
	"math"
	"sort"

	"stride/internal/fitdata"
)

const minSegmentS = 20.0

// windowStats : statistiques recalculées sur les records d'une fenêtre [t0, t1].
type windowStats struct {
	avgHR, maxHR, avgPower, avgCad, gct, vo, bal float64
	distM, ascent, descent                       float64
	n                                            int
}

func (s *session) window(t0, t1 float64, runOnly bool) windowStats {
	var hr, pw, cad, gct, vo, bal wacc
	ws := windowStats{maxHR: nan, distM: nan, ascent: nan, descent: nan}
	d0, d1 := nan, nan
	prevAlt := nan
	asc, desc := 0.0, 0.0
	for i, r := range s.act.Records {
		if r.T < t0 || r.T > t1 {
			continue
		}
		if runOnly && !s.run[i] {
			continue
		}
		w := s.w[i]
		ws.n++
		hr.add(r.HR, w)
		if valid(r.HR) && (!valid(ws.maxHR) || r.HR > ws.maxHR) {
			ws.maxHR = r.HR
		}
		pw.add(r.PowerW, w)
		if valid(r.CadenceSpm) && r.CadenceSpm > 0 {
			cad.add(r.CadenceSpm, w)
		}
		gct.add(r.GCTMs, w)
		vo.add(r.VOCm, w)
		bal.add(r.GCTBalLeftPct, w)
		if valid(r.DistM) {
			if !valid(d0) {
				d0 = r.DistM
			}
			d1 = r.DistM
		}
		if valid(r.AltM) {
			// hystérésis de 1 m pour limiter le bruit barométrique
			if !valid(prevAlt) {
				prevAlt = r.AltM
			} else if dz := r.AltM - prevAlt; math.Abs(dz) >= 1 {
				if dz > 0 {
					asc += dz
				} else {
					desc -= dz
				}
				prevAlt = r.AltM
			}
		}
	}
	ws.avgHR, ws.avgPower, ws.avgCad = hr.mean(), pw.mean(), cad.mean()
	ws.gct, ws.vo, ws.bal = gct.mean(), vo.mean(), bal.mean()
	if valid(d0) && valid(d1) {
		ws.distM = d1 - d0
	}
	if valid(prevAlt) {
		ws.ascent, ws.descent = asc, desc
	}
	return ws
}

func pauseKindLabel(kind string) string {
	if kind == "stand" {
		return "Arrêt"
	}
	return "Marche"
}

// buildSegment assemble un segment à partir d'une fenêtre et des valeurs de résumé.
func (s *session) buildSegment(kind, label string, t0, t1, durS, distM, asc, desc float64) Segment {
	ws := s.window(t0, t1, false)
	if !valid(distM) {
		distM = ws.distM
	}
	if !valid(distM) || distM < 0 {
		distM = 0
	}
	if !valid(durS) || durS <= 0 {
		durS = math.Max(0, t1-t0)
	}
	seg := Segment{
		Kind:              kind,
		Label:             label,
		StartOffsetS:      roundTo(math.Max(0, t0), 0),
		DurationS:         roundTo(durS, 0),
		DistanceM:         roundTo(distM, 1),
		AvgHR:             num(ws.avgHR, 0),
		MaxHR:             num(ws.maxHR, 0),
		AvgPower:          num(ws.avgPower, 0),
		AvgCadenceSpm:     num(ws.avgCad, 0),
		GctMs:             num(ws.gct, 0),
		VoCm:              num(ws.vo, 1),
		GctBalanceLeftPct: num(ws.bal, 1),
		ElevGainM:         num(first(asc, ws.ascent), 0),
		ElevLossM:         num(first(desc, ws.descent), 0),
	}
	if kind != "stand" && distM >= 10 && durS > 0 {
		seg.PaceSecPerKm = num(durS/distM*1000, 0)
	}
	return seg
}

// rawBlock : fenêtre course/pause avant consolidation.
type rawBlock struct {
	kind            string // run | walk | stand
	t0, t1          float64
	durS, distM     float64
	ascent, descent float64
	walkS, standS   float64 // pour choisir le type dominant d'une pause fusionnée
}

func (r rawBlock) isPause() bool { return r.kind != "run" }

func addNaN(a, b float64) float64 {
	switch {
	case !valid(a):
		return b
	case !valid(b):
		return a
	}
	return a + b
}

// merge fusionne une suite de blocs en un seul, de nature `kind`.
func mergeRaw(kind string, bs ...rawBlock) rawBlock {
	out := rawBlock{kind: kind, t0: bs[0].t0, t1: bs[len(bs)-1].t1, distM: nan, ascent: nan, descent: nan}
	for _, b := range bs {
		out.durS += first(b.durS, 0)
		out.distM = addNaN(out.distM, b.distM)
		out.ascent = addNaN(out.ascent, b.ascent)
		out.descent = addNaN(out.descent, b.descent)
		out.walkS += b.walkS
		out.standS += b.standS
	}
	if kind != "run" {
		out.kind = "stand"
		if out.walkS >= out.standS {
			out.kind = "walk"
		}
	}
	return out
}

// Seuils de consolidation des splits rwd_* (Garmin découpe des micro-pauses de 1-3 s).
const (
	minPauseS = 10.0 // pause plus courte → absorbée dans la course
	minRunS   = 20.0 // course plus courte entre deux pauses → absorbée dans la pause
)

// consolidate : fusionne les pauses adjacentes, absorbe les micro-pauses dans la course
// et les micro-blocs de course entre deux pauses.
func consolidate(in []rawBlock) []rawBlock {
	bs := append([]rawBlock{}, in...)
	for changed := true; changed; {
		changed = false
		// 1) pauses adjacentes → une seule pause ; courses adjacentes → une seule course
		for i := 1; i < len(bs); i++ {
			if bs[i].isPause() == bs[i-1].isPause() {
				kind := bs[i].kind
				if !bs[i].isPause() {
					kind = "run"
				}
				bs[i-1] = mergeRaw(kind, bs[i-1], bs[i])
				bs = append(bs[:i], bs[i+1:]...)
				changed = true
				i--
			}
		}
		// 2) micro-pause → absorbée par la course voisine (et fusion course+course)
		for i := 0; i < len(bs) && len(bs) > 1; i++ {
			if !bs[i].isPause() || bs[i].durS >= minPauseS {
				continue
			}
			switch {
			case i > 0 && i+1 < len(bs):
				bs[i-1] = mergeRaw("run", bs[i-1], bs[i], bs[i+1])
				bs = append(bs[:i], bs[i+2:]...)
			case i > 0:
				bs[i-1] = mergeRaw("run", bs[i-1], bs[i])
				bs = bs[:i]
			default:
				bs[1] = mergeRaw("run", bs[0], bs[1])
				bs = bs[1:]
			}
			changed = true
			break
		}
		if changed {
			continue
		}
		// 3) micro-bloc de course coincé entre deux pauses → absorbé dans la pause
		for i := 1; i+1 < len(bs); i++ {
			if bs[i].isPause() || bs[i].durS >= minRunS || !bs[i-1].isPause() || !bs[i+1].isPause() {
				continue
			}
			bs[i-1] = mergeRaw("walk", bs[i-1], bs[i], bs[i+1])
			bs = append(bs[:i], bs[i+2:]...)
			changed = true
			break
		}
	}
	return bs
}

// blocksFromSplits : blocs course/marche/arrêt depuis les splits FIT rwd_*.
func blocksFromSplits(s *session) []Segment {
	var sp []fitdata.Split
	for _, x := range s.act.Splits {
		switch x.Type {
		case "rwd_run", "rwd_walk", "rwd_stand":
			if valid(x.StartT) {
				sp = append(sp, x)
			}
		}
	}
	if len(sp) == 0 {
		return nil
	}
	sort.SliceStable(sp, func(i, j int) bool { return sp[i].StartT < sp[j].StartT })
	raws := make([]rawBlock, 0, len(sp))
	for _, x := range sp {
		kind := map[string]string{"rwd_run": "run", "rwd_walk": "walk", "rwd_stand": "stand"}[x.Type]
		t1 := x.EndT
		if !valid(t1) {
			t1 = x.StartT + first(x.ElapsedS, x.TimerS, 0)
		}
		dur := first(x.TimerS, t1-x.StartT)
		if kind != "run" {
			dur = first(x.ElapsedS, x.TimerS, t1-x.StartT) // durée réelle de la pause (auto-pause)
		}
		rb := rawBlock{kind: kind, t0: x.StartT, t1: t1, durS: dur, distM: x.DistM, ascent: x.AscentM, descent: x.DescentM}
		switch kind {
		case "walk":
			rb.walkS = dur
		case "stand":
			rb.standS = dur
		}
		raws = append(raws, rb)
	}
	return s.labelBlocks(consolidate(alignPauses(raws)))
}

// alignPauses : une pause couvre tout le temps réel entre les blocs de course voisins,
// arrêt du chrono compris (les splits rwd_stand ne couvrent que la fin de l'arrêt et leur
// « elapsed » ne correspond à aucune fenêtre). Les fins de split sont inclusives (1 s d'écart
// entre deux splits contigus) : seul un vrai trou (> maxSplitGapS) est comblé.
func alignPauses(bs []rawBlock) []rawBlock {
	const maxSplitGapS = 2.0
	for i := range bs {
		if !bs[i].isPause() {
			continue
		}
		extended := false
		if i > 0 && bs[i].t0-bs[i-1].t1 > maxSplitGapS {
			bs[i].t0, extended = bs[i-1].t1, true
		}
		if i+1 < len(bs) && bs[i+1].t0-bs[i].t1 > maxSplitGapS {
			bs[i].t1, extended = bs[i+1].t0, true
		}
		if !extended {
			continue
		}
		bs[i].durS = math.Max(first(bs[i].durS, 0), bs[i].t1-bs[i].t0)
		switch bs[i].kind {
		case "walk":
			bs[i].walkS = bs[i].durS
		case "stand":
			bs[i].standS = bs[i].durS
		}
	}
	return bs
}

// labelBlocks construit les segments « Bloc n » / « Pause n ».
func (s *session) labelBlocks(raws []rawBlock) []Segment {
	var out []Segment
	nRun, nPause := 0, 0
	for _, r := range raws {
		var label string
		if r.kind == "run" {
			nRun++
			label = fmt.Sprintf("Bloc %d", nRun)
		} else {
			nPause++
			label = fmt.Sprintf("Pause %d", nPause)
		}
		out = append(out, s.buildSegment(r.kind, label, r.t0, r.t1, r.durS, r.distM, r.ascent, r.descent))
	}
	for i := range out {
		out[i].Index = i + 1
	}
	return out
}

// detectBlocks : détection maison course/marche/arrêt sur les records (lissage 10 s,
// segments ≥ 20 s, trous d'enregistrement > 10 s = arrêt).
func detectBlocks(s *session) []Segment {
	recs := s.act.Records
	if len(recs) < 30 {
		return nil
	}
	type raw struct {
		kind   string
		t0, t1 float64
	}
	classify := func(i int) string {
		var spd, cad wacc
		for j := i; j >= 0 && recs[i].T-recs[j].T < 10; j-- {
			spd.add(recs[j].SpeedMS, 1)
			cad.add(recs[j].CadenceSpm, 1)
		}
		v, c := spd.mean(), cad.mean()
		switch {
		case valid(v) && v < 0.5, !valid(v) && valid(c) && c < 30:
			return "stand"
		case (valid(c) && c < runMinCadenceSpm) || (valid(v) && v < runMinSpeedMS):
			return "walk"
		case !valid(v) && !valid(c):
			return "stand"
		}
		return "run"
	}
	var segs []raw
	for i := range recs {
		k := classify(i)
		t := recs[i].T
		if i > 0 && t-recs[i-1].T > 10 {
			segs = append(segs, raw{kind: "stand", t0: recs[i-1].T, t1: t})
		}
		if n := len(segs); n > 0 && segs[n-1].kind == k {
			segs[n-1].t1 = t
			continue
		}
		start := t
		if n := len(segs); n > 0 {
			start = segs[n-1].t1
		}
		segs = append(segs, raw{kind: k, t0: start, t1: t})
	}
	// fusion des segments trop courts dans le voisin le plus long
	for changed := true; changed && len(segs) > 1; {
		changed = false
		for i := 0; i < len(segs); i++ {
			if segs[i].t1-segs[i].t0 >= minSegmentS {
				continue
			}
			j := i - 1
			if j < 0 || (i+1 < len(segs) && segs[i+1].t1-segs[i+1].t0 > segs[j].t1-segs[j].t0) {
				j = i + 1
			}
			if j < i {
				segs[j].t1 = segs[i].t1
			} else {
				segs[j].t0 = segs[i].t0
			}
			segs = append(segs[:i], segs[i+1:]...)
			changed = true
			break
		}
		// re-fusion des voisins de même nature
		for i := 1; i < len(segs); i++ {
			if segs[i].kind == segs[i-1].kind {
				segs[i-1].t1 = segs[i].t1
				segs = append(segs[:i], segs[i+1:]...)
				i--
			}
		}
	}
	hasRun, hasPause := false, false
	raws := make([]rawBlock, 0, len(segs))
	for _, r := range segs {
		if r.kind == "run" {
			hasRun = true
		} else {
			hasPause = true
		}
		rb := rawBlock{kind: r.kind, t0: r.t0, t1: r.t1, durS: r.t1 - r.t0, distM: nan, ascent: nan, descent: nan}
		if r.kind == "walk" {
			rb.walkS = rb.durS
		} else if r.kind == "stand" {
			rb.standS = rb.durS
		}
		raws = append(raws, rb)
	}
	if !hasRun {
		return nil
	}
	if !hasPause {
		raws = raws[:1]
	}
	return s.labelBlocks(raws)
}

// lapsFromFIT : laps montre (« Km n » si déclenchés à la distance, sinon « Lap n »).
func lapsFromFIT(s *session) []Segment {
	laps := s.act.Laps
	if len(laps) == 0 {
		return syntheticKmLaps(s)
	}
	allDist := true
	var out []Segment
	nRest := 0
	for i, l := range laps {
		var kind, label string
		switch l.Intensity {
		case "warmup":
			kind, label = "warmup", "Échauffement"
		case "cooldown":
			kind, label = "cooldown", "Retour au calme"
		case "rest", "recovery":
			nRest++
			kind, label = "rest", fmt.Sprintf("Récup %d", nRest)
		default:
			kind = "lap"
			if l.HasWorkoutStep && (l.Intensity == "active" || l.Intensity == "interval") {
				kind = "active"
			}
			isDist := l.Trigger == "distance" || (l.Trigger == "session_end" && allDist && i > 0)
			if isDist {
				label = fmt.Sprintf("Km %d", i+1)
			} else {
				label = fmt.Sprintf("Lap %d", i+1)
			}
		}
		if l.Trigger != "distance" {
			allDist = false
		}
		t0, t1 := l.StartT, l.EndT
		if !valid(t1) {
			t1 = t0 + first(l.ElapsedS, l.TimerS, 0)
		}
		seg := s.buildSegment(kind, label, t0, t1, first(l.TimerS, l.ElapsedS), l.DistM, l.AscentM, l.DescentM)
		// valeurs de résumé du lap, prioritaires quand présentes
		if v := num(l.AvgHR, 0); v != nil {
			seg.AvgHR = v
		}
		if v := num(l.MaxHR, 0); v != nil {
			seg.MaxHR = v
		}
		if v := num(l.AvgPower, 0); v != nil {
			seg.AvgPower = v
		}
		if v := num(l.AvgCadSpm, 0); v != nil && *v > 0 {
			seg.AvgCadenceSpm = v
		}
		if v := num(l.AvgGCTMs, 0); v != nil {
			seg.GctMs = v
		}
		if v := num(l.AvgVOCm, 1); v != nil {
			seg.VoCm = v
		}
		if v := num(l.AvgGCTBal, 1); v != nil {
			seg.GctBalanceLeftPct = v
		}
		out = append(out, seg)
	}
	for i := range out {
		out[i].Index = i + 1
	}
	return out
}

// syntheticKmLaps : découpage par kilomètre quand aucun lap n'est disponible (source details).
func syntheticKmLaps(s *session) []Segment {
	recs := s.act.Records
	if len(recs) == 0 {
		return nil
	}
	var out []Segment
	startIdx := 0
	nextKm := 1000.0
	d0 := nan
	for i, r := range recs {
		if !valid(r.DistM) {
			continue
		}
		if !valid(d0) {
			d0 = r.DistM
		}
		if r.DistM-d0 >= nextKm || i == len(recs)-1 {
			t0, t1 := recs[startIdx].T, r.T
			n := len(out) + 1
			out = append(out, s.buildSegment("lap", fmt.Sprintf("Km %d", n), t0, t1, s.timerBetween(startIdx, i), nan, nan, nan))
			startIdx = i
			nextKm += 1000
		}
	}
	for i := range out {
		out[i].Index = i + 1
	}
	return out
}

func (s *session) timerBetween(i0, i1 int) float64 {
	t := 0.0
	for i := i0; i < i1 && i < len(s.w); i++ {
		t += s.w[i]
	}
	return t
}

// buildSegments choisit blocs et laps ; primary = blocks si ≥ 2 segments course/marche.
func buildSegments(s *session) Segments {
	seg := Segments{Primary: "laps", Blocks: []Segment{}, Laps: []Segment{}}
	if s == nil {
		return seg
	}
	// Blocs course/marche : course à pied uniquement (pas de sens pour vélo, kayak, ski…).
	var blocks []Segment
	if s.isRunning {
		blocks = blocksFromSplits(s)
		if blocks == nil {
			blocks = detectBlocks(s)
		}
	}
	if blocks != nil {
		seg.Blocks = blocks
	}
	if laps := lapsFromFIT(s); laps != nil {
		seg.Laps = laps
	}
	if len(seg.Blocks) >= 2 || len(seg.Laps) == 0 && len(seg.Blocks) > 0 {
		seg.Primary = "blocks"
	}
	return seg
}

// primarySegments retourne la liste utilisée par le rapport.
func (sg Segments) primarySegments() []Segment {
	if sg.Primary == "blocks" {
		return sg.Blocks
	}
	return sg.Laps
}

// fadeEnd : allure du dernier bloc de course > 8 % plus lente que la moyenne des
// précédents avec une FC ≥ → retourne le % de ralentissement (sinon NaN).
func fadeEnd(sg Segments) float64 {
	var runs []Segment
	for _, b := range sg.Blocks {
		if b.Kind == "run" && b.PaceSecPerKm != nil && b.DistanceM >= 200 {
			runs = append(runs, b)
		}
	}
	if len(runs) < 2 {
		runs = runs[:0]
		for _, l := range sg.Laps {
			if (l.Kind == "lap" || l.Kind == "active") && l.PaceSecPerKm != nil && l.DistanceM >= 500 {
				runs = append(runs, l)
			}
		}
	}
	if len(runs) < 2 {
		return nan
	}
	last := runs[len(runs)-1]
	prevT, prevD := 0.0, 0.0
	var hr wacc
	for _, r := range runs[:len(runs)-1] {
		prevT += r.DurationS
		prevD += r.DistanceM
		if r.AvgHR != nil {
			hr.add(*r.AvgHR, r.DurationS)
		}
	}
	if prevD <= 0 || last.AvgHR == nil || !valid(hr.mean()) {
		return nan
	}
	prevPace := prevT / prevD * 1000
	slow := (*last.PaceSecPerKm - prevPace) / prevPace * 100
	if slow > 8 && *last.AvgHR >= math.Round(hr.mean()) {
		return slow
	}
	return nan
}
