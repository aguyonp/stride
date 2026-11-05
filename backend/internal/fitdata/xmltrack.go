package fitdata

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Track is a GPX/TCX file converted to the internal series.
type Track struct {
	Name string
	Type string // GPX <type> or TCX Sport attribute, lower-cased
	Act  *Activity
}

type xmlPoint struct {
	t                                            time.Time
	lat, lon, ele, dist, hr, cad, power, speedMS float64
}

func newXMLPoint() xmlPoint {
	return xmlPoint{lat: NaN, lon: NaN, ele: NaN, dist: NaN, hr: NaN, cad: NaN, power: NaN, speedMS: NaN}
}

func parseNum(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return NaN
	}
	return v
}

// DecodeXML parses GPX (trkpt) or TCX (Trackpoint) files, namespace-agnostic: HR, cadence, power,
// altitude, distance and speed are read when present.
func DecodeXML(b []byte) (*Track, error) {
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.Strict = false
	var stack []string
	var pts []xmlPoint
	var cur *xmlPoint
	tr := &Track{}
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			stack = append(stack, name)
			switch name {
			case "trkpt", "Trackpoint":
				p := newXMLPoint()
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "lat":
						p.lat = parseNum(a.Value)
					case "lon":
						p.lon = parseNum(a.Value)
					}
				}
				cur = &p
			case "Activity":
				for _, a := range t.Attr {
					if a.Name.Local == "Sport" && tr.Type == "" {
						tr.Type = strings.ToLower(a.Value)
					}
				}
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			if (t.Name.Local == "trkpt" || t.Name.Local == "Trackpoint") && cur != nil {
				if !cur.t.IsZero() {
					pts = append(pts, *cur)
				}
				cur = nil
			}
		case xml.CharData:
			if len(stack) == 0 {
				continue
			}
			text := strings.TrimSpace(string(t))
			if text == "" {
				continue
			}
			el := stack[len(stack)-1]
			parent := ""
			if len(stack) > 1 {
				parent = stack[len(stack)-2]
			}
			if cur == nil {
				switch {
				case el == "name" && tr.Name == "":
					tr.Name = text
				case el == "type" && parent == "trk" && tr.Type == "":
					tr.Type = strings.ToLower(text)
				}
				continue
			}
			switch el {
			case "time", "Time":
				if ts, err := time.Parse(time.RFC3339Nano, text); err == nil {
					cur.t = ts.UTC()
				}
			case "ele", "AltitudeMeters":
				cur.ele = parseNum(text)
			case "LatitudeDegrees":
				cur.lat = parseNum(text)
			case "LongitudeDegrees":
				cur.lon = parseNum(text)
			case "DistanceMeters":
				cur.dist = parseNum(text)
			case "hr", "heartrate", "HeartRate":
				cur.hr = parseNum(text)
			case "Value":
				if parent == "HeartRateBpm" {
					cur.hr = parseNum(text)
				}
			case "cad", "cadence", "Cadence", "RunCadence":
				cur.cad = parseNum(text)
			case "power", "Watts", "watts":
				cur.power = parseNum(text)
			case "Speed", "speed":
				cur.speedMS = parseNum(text)
			}
		}
	}
	if len(pts) < 2 {
		return nil, errors.New("aucun point horodaté")
	}
	sort.SliceStable(pts, func(i, j int) bool { return pts[i].t.Before(pts[j].t) })
	tr.Act = pointsToActivity(pts, isRunningType(tr.Type))
	return tr, nil
}

func isRunningType(t string) bool {
	t = strings.ToLower(t)
	return t == "" || strings.Contains(t, "run") || strings.Contains(t, "course") || t == "9"
}

func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371000.0
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}

func pointsToActivity(pts []xmlPoint, running bool) *Activity {
	out := &Activity{Source: "import", Start: pts[0].t, Session: EmptySession()}
	if running {
		out.Sport = "running"
	}
	// running cadence is stored in strides/min (rpm) by most exporters
	var cads []float64
	for _, p := range pts {
		if valid(p.cad) && p.cad > 0 {
			cads = append(cads, p.cad)
		}
	}
	double := false
	if running && len(cads) > 0 {
		sort.Float64s(cads)
		double = cads[len(cads)/2] < 120
	}
	dist := 0.0
	for i, p := range pts {
		r := emptyRecord(p.t.Sub(out.Start).Seconds())
		r.Lat, r.Lon = degrees(p.lat), degrees(p.lon)
		r.AltM, r.PowerW = p.ele, p.power
		if valid(p.hr) && p.hr > 0 {
			r.HR = p.hr
		}
		if valid(p.cad) && p.cad > 0 {
			r.CadenceSpm = p.cad
			if double {
				r.CadenceSpm *= 2
			}
		}
		switch {
		case valid(p.dist):
			dist = p.dist
		case i > 0 && valid(r.Lat) && valid(pts[i-1].lat) && valid(r.Lon) && valid(pts[i-1].lon):
			dist += haversine(pts[i-1].lat, pts[i-1].lon, p.lat, p.lon)
		}
		r.DistM = dist
		r.SpeedMS = p.speedMS
		if !valid(r.SpeedMS) && i > 0 {
			if dt := r.T - out.Records[i-1].T; dt > 0 {
				r.SpeedMS = (r.DistM - out.Records[i-1].DistM) / dt
			}
		}
		out.Records = append(out.Records, r)
	}
	last := out.Records[len(out.Records)-1]
	out.Session.ElapsedS, out.Session.DistM = last.T, last.DistM
	if len(out.Records) > 0 && valid(out.Records[0].Lat) {
		out.Session.StartLat, out.Session.StartLon = out.Records[0].Lat, out.Records[0].Lon
	}
	return out
}

func valid(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
