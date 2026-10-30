package fence

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	kit "desk-kit"
)

const MaxInput = 1 << 20
const MaxPoints = 4096

type Point struct {
	X     float64
	Y     float64
	Label string
	Nano  uint64
	Start uint64
}
type Bucket struct{ Low, High, Count float64 }
type Series struct {
	Name, Unit, Kind string
	Points           []Point
	Buckets          []Bucket
	Delta            bool
	Latest           uint64
	Columns          []string
	Rows             [][]string
}

// Num accepts the numbers and quoted int64 values used by OTLP/JSON.
type Num float64

func (n *Num) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Errorf("invalid finite number %q", s)
	}
	*n = Num(v)
	return nil
}

type Nanos uint64

func (n *Nanos) UnmarshalJSON(b []byte) error {
	v, e := strconv.ParseUint(strings.Trim(string(b), `"`), 10, 64)
	*n = Nanos(v)
	return e
}

type OTLPPoint struct {
	Attributes json.RawMessage `json:"attributes"`
	Time       Nanos           `json:"timeUnixNano"`
	Start      Nanos           `json:"startTimeUnixNano"`
	Double     *Num            `json:"asDouble"`
	Int        *Num            `json:"asInt"`
	Count      Num             `json:"count"`
	Bounds     []Num           `json:"explicitBounds"`
	Counts     []Num           `json:"bucketCounts"`
	Scale      int             `json:"scale"`
	Zero       Num             `json:"zeroCount"`
	Threshold  Num             `json:"zeroThreshold"`
	Positive   struct {
		Offset int   `json:"offset"`
		Counts []Num `json:"bucketCounts"`
	} `json:"positive"`
	Negative struct {
		Offset int   `json:"offset"`
		Counts []Num `json:"bucketCounts"`
	} `json:"negative"`
	Flags uint32 `json:"flags"`
}
type MetricData struct {
	Points      []OTLPPoint     `json:"dataPoints"`
	Monotonic   bool            `json:"isMonotonic"`
	Temporality json.RawMessage `json:"aggregationTemporality"`
}
type Metric struct {
	Name        string      `json:"name"`
	Unit        string      `json:"unit"`
	Sum         *MetricData `json:"sum"`
	Gauge       *MetricData `json:"gauge"`
	Histogram   *MetricData `json:"histogram"`
	Exponential *MetricData `json:"exponentialHistogram"`
}
type OTLP struct {
	Resources []struct {
		Resource json.RawMessage `json:"resource"`
		Scopes   []struct {
			Scope   json.RawMessage `json:"scope"`
			Metrics []Metric        `json:"metrics"`
		} `json:"scopeMetrics"`
	} `json:"resourceMetrics"`
}

func refuseSecrets(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, v := range x {
			if kit.SecretRefused([]kit.Field{{Name: k}}) {
				return true
			}
			if k == "type" || k == "key" || k == "name" {
				if s, ok := v.(string); ok && kit.SecretRefused([]kit.Field{{Name: func() string {
					if k == "type" {
						return ""
					}
					return s
				}(), Type: func() string {
					if k == "type" {
						return s
					}
					return ""
				}()}}) {
					return true
				}
			}
			if refuseSecrets(v) {
				return true
			}
		}
	case []any:
		for _, v := range x {
			if refuseSecrets(v) {
				return true
			}
		}
	}
	return false
}

func ParseData(raw []byte, format string) ([]Series, error) {
	if len(raw) > MaxInput {
		return nil, fmt.Errorf("input exceeds 1 MiB")
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("input is empty")
	}
	if format == "" || format == "auto" {
		if raw[0] == '{' || raw[0] == '[' {
			format = "json"
			if !json.Valid(raw) {
				format = "ndjson"
			}
		} else {
			format = "csv"
		}
	}
	var docs []any
	switch format {
	case "json", "otlp":
		var v any
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		if e := d.Decode(&v); e != nil {
			return nil, e
		}
		var extra any
		if d.Decode(&extra) != io.EOF {
			return nil, fmt.Errorf("expected one JSON value")
		}
		docs = []any{v}
	case "ndjson":
		for _, line := range bytes.Split(raw, []byte{'\n'}) {
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var v any
			d := json.NewDecoder(bytes.NewReader(line))
			d.UseNumber()
			if e := d.Decode(&v); e != nil {
				return nil, e
			}
			var extra any
			if d.Decode(&extra) != io.EOF {
				return nil, fmt.Errorf("one JSON value per NDJSON line required")
			}
			docs = append(docs, v)
		}
	case "csv", "tsv":
		r := csv.NewReader(bytes.NewReader(raw))
		if format == "tsv" {
			r.Comma = '\t'
		}
		rows, e := r.ReadAll()
		if e != nil {
			return nil, e
		}
		if len(rows) < 2 {
			return nil, fmt.Errorf("CSV needs a header and data")
		}
		for _, row := range rows[1:] {
			m := map[string]any{}
			for i, h := range rows[0] {
				m[h] = row[i]
			}
			docs = append(docs, m)
		}
	default:
		return nil, fmt.Errorf("unknown format %q", format)
	}
	var result []Series
	var generic []any
	for _, v := range docs {
		if refuseSecrets(v) {
			return nil, fmt.Errorf("%s", kit.ErrSecret)
		}
		if m, ok := v.(map[string]any); ok && m["resourceMetrics"] != nil {
			b, _ := json.Marshal(m)
			var o OTLP
			if e := json.Unmarshal(b, &o); e != nil {
				return nil, e
			}
			ss, e := parseOTLP(o)
			if e != nil {
				return nil, e
			}
			result = append(result, ss...)
		} else if a, ok := v.([]any); ok {
			generic = append(generic, a...)
		} else {
			generic = append(generic, v)
		}
	}
	if len(generic) > 0 {
		ss, e := parseGeneric(generic)
		if e != nil {
			return nil, e
		}
		result = append(result, ss...)
	}
	// Merge repeated OTLP exports while keeping distinct streams apart.
	merged := []Series{}
	indices := map[string]int{}
	for _, s := range result {
		key := s.Name + "\x00" + s.Unit + "\x00" + s.Kind + fmt.Sprint(s.Delta)
		if i, ok := indices[key]; ok {
			merged[i].Points = append(merged[i].Points, s.Points...)
			if (s.Kind == "histogram" || s.Kind == "exponentialhistogram") && s.Latest >= merged[i].Latest {
				merged[i].Buckets = s.Buckets
				merged[i].Latest = s.Latest
			}
		} else {
			indices[key] = len(merged)
			merged = append(merged, s)
		}
	}
	total := 0
	for i := range merged {
		s := &merged[i]
		total += len(s.Points) + len(s.Buckets) + len(s.Rows)
		sort.SliceStable(s.Points, func(i, j int) bool {
			if s.Points[i].Nano != 0 || s.Points[j].Nano != 0 {
				return s.Points[i].Nano < s.Points[j].Nano
			}
			return s.Points[i].X < s.Points[j].X
		})
	}
	if len(merged) == 0 || len(merged) > 128 || total > MaxPoints {
		return nil, fmt.Errorf("expected 1–128 Series and at most %d points/buckets", MaxPoints)
	}
	return merged, nil
}
func number(v any) (float64, error) {
	s := fmt.Sprint(v)
	f, e := strconv.ParseFloat(s, 64)
	if e != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("expected finite number, got %q", s)
	}
	return f, nil
}
func parseGeneric(rows []any) ([]Series, error) {
	// Mixed textual columns form a generic table; numeric records remain plots.
	columns := map[string]bool{}
	textual := false
	for _, row := range rows {
		if m, ok := row.(map[string]any); ok {
			for k, v := range m {
				columns[k] = true
				if f, e := strconv.ParseFloat(fmt.Sprint(v), 64); e == nil && (math.IsNaN(f) || math.IsInf(f, 0)) {
					return nil, fmt.Errorf("non-finite number")
				}
				if k != "name" && k != "time" && k != "timestamp" && k != "label" && k != "x" {
					if _, e := number(v); e != nil {
						textual = true
					}
				}
			}
		}
	}
	if textual {
		table := Series{Name: "Data", Kind: "table"}
		for k := range columns {
			table.Columns = append(table.Columns, k)
		}
		sort.Strings(table.Columns)
		for _, row := range rows {
			m, ok := row.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("table rows must be objects")
			}
			cells := []string{}
			for _, k := range table.Columns {
				v := m[k]
				cell := ""
				if v != nil {
					switch v.(type) {
					case map[string]any, []any:
						b, _ := json.Marshal(v)
						cell = string(b)
					default:
						cell = fmt.Sprint(v)
					}
				}
				cells = append(cells, cell)
			}
			table.Rows = append(table.Rows, cells)
		}
		return []Series{table}, nil
	}

	out := []Series{}
	index := map[string]int{}
	add := func(name string, p Point) {
		i, ok := index[name]
		if !ok {
			i = len(out)
			index[name] = i
			out = append(out, Series{Name: name, Kind: "line"})
		}
		out[i].Points = append(out[i].Points, p)
	}
	for i, v := range rows {
		if m, ok := v.(map[string]any); ok {
			label := fmt.Sprint(i + 1)
			x := float64(i)
			for _, k := range []string{"time", "timestamp", "x", "label"} {
				if z, ok := m[k]; ok {
					label = fmt.Sprint(z)
					if t, e := time.Parse(time.RFC3339Nano, label); e == nil {
						x = float64(t.UnixNano()) / 1e9
					} else if f, e := number(z); e == nil {
						x = f
					}
					break
				}
			}
			keys := []string{}
			for k := range m {
				if k != "time" && k != "timestamp" && k != "x" && k != "label" && k != "name" {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			found := false
			for _, k := range keys {
				f, e := number(m[k])
				if e != nil {
					return nil, fmt.Errorf("column %s: %w", k, e)
				}
				name := k
				if n, ok := m["name"].(string); ok && k == "value" {
					name = n
				}
				add(name, Point{X: x, Y: f, Label: label})
				found = true
			}
			if !found {
				return nil, fmt.Errorf("row has no numeric columns")
			}
		} else {
			f, e := number(v)
			if e != nil {
				return nil, e
			}
			add("value", Point{X: float64(i), Y: f, Label: strconv.Itoa(i + 1)})
		}
	}
	return out, nil
}
func canonical(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	_ = json.Unmarshal(raw, &v)
	if a, ok := v.([]any); ok {
		sort.Slice(a, func(i, j int) bool { return fmt.Sprint(a[i]) < fmt.Sprint(a[j]) })
	}
	b, _ := json.Marshal(v)
	return string(b)
}
func parseOTLP(o OTLP) ([]Series, error) {
	var out []Series
	for _, r := range o.Resources {
		for _, sc := range r.Scopes {
			for _, m := range sc.Metrics {
				kind := ""
				var d *MetricData
				switch {
				case m.Sum != nil:
					d = m.Sum
					kind = "counter"
					if !d.Monotonic {
						kind = "updowncounter"
					}
				case m.Gauge != nil:
					d = m.Gauge
					kind = "gauge"
				case m.Histogram != nil:
					d = m.Histogram
					kind = "histogram"
				case m.Exponential != nil:
					d = m.Exponential
					kind = "exponentialhistogram"
				default:
					return nil, fmt.Errorf("unsupported Metric %q", m.Name)
				}
				groups := map[string]int{}
				delta := string(d.Temporality) == "1" || string(d.Temporality) == `"AGGREGATION_TEMPORALITY_DELTA"`
				sort.SliceStable(d.Points, func(i, j int) bool { return d.Points[i].Time < d.Points[j].Time })
				for _, p := range d.Points {
					if p.Flags&1 != 0 {
						continue
					}
					identity := canonical(r.Resource) + canonical(sc.Scope) + canonical(p.Attributes)
					name := m.Name
					if identity != "" {
						name += " · " + identity
					}
					idx, ok := groups[name]
					if !ok {
						idx = len(out)
						groups[name] = idx
						out = append(out, Series{Name: name, Unit: m.Unit, Kind: kind, Delta: delta})
					}
					s := &out[idx]
					if kind == "histogram" || kind == "exponentialhistogram" {
						b, e := HistBuckets(p, kind)
						if e != nil {
							return nil, fmt.Errorf("%s: %w", m.Name, e)
						}
						s.Buckets = b
						s.Latest = uint64(p.Time)
						continue
					}
					val := p.Double
					if val == nil {
						val = p.Int
					}
					if val == nil {
						return nil, fmt.Errorf("%s: missing numeric value", m.Name)
					}
					if kind == "counter" && *val < 0 {
						return nil, fmt.Errorf("negative monotonic sum")
					}
					s.Points = append(s.Points, Point{X: float64(p.Time) / 1e9, Y: float64(*val), Nano: uint64(p.Time), Start: uint64(p.Start), Label: time.Unix(0, int64(p.Time)).UTC().Format(time.RFC3339Nano)})
				}
			}
		}
	}
	return out, nil
}
func HistBuckets(p OTLPPoint, kind string) ([]Bucket, error) {
	var out []Bucket
	if kind == "histogram" {
		if len(p.Counts) != len(p.Bounds)+1 {
			return nil, fmt.Errorf("histogram needs bounds + 1 counts")
		}
		lo := math.Inf(-1)
		for i, c := range p.Counts {
			hi := math.Inf(1)
			if i < len(p.Bounds) {
				hi = float64(p.Bounds[i])
				if hi <= lo {
					return nil, fmt.Errorf("bounds must increase")
				}
			}
			out = append(out, Bucket{lo, hi, float64(c)})
			lo = hi
		}
	} else {
		if p.Scale < -10 || p.Scale > 20 || p.Threshold < 0 {
			return nil, fmt.Errorf("invalid exponential histogram scale/threshold")
		}
		step := math.Exp2(-float64(p.Scale))
		for sign, bs := range []struct {
			Offset int
			Counts []Num
		}{{p.Negative.Offset, p.Negative.Counts}, {p.Positive.Offset, p.Positive.Counts}} {
			for i, c := range bs.Counts {
				lo := math.Exp2(float64(bs.Offset+i) * step)
				hi := math.Exp2(float64(bs.Offset+i+1) * step)
				if math.IsInf(hi, 0) || lo == 0 || lo == hi {
					return nil, fmt.Errorf("exponential Bucket outside representable range")
				}
				if sign == 0 {
					lo, hi = -hi, -lo
				}
				out = append(out, Bucket{lo, hi, float64(c)})
			}
		}
		if p.Zero > 0 {
			out = append(out, Bucket{-float64(p.Threshold), float64(p.Threshold), float64(p.Zero)})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Low < out[j].Low })
	}
	total := 0.0
	for _, b := range out {
		if b.Count < 0 || math.Trunc(b.Count) != b.Count {
			return nil, fmt.Errorf("invalid Bucket count")
		}
		total += b.Count
	}
	if total != float64(p.Count) {
		return nil, fmt.Errorf("Bucket counts do not match count")
	}
	return out, nil
}

// Quantiles are linear estimates within finite buckets. Open tails are reported
// as a bound rather than a fabricated finite percentile.
func Quantile(bs []Bucket, q float64) string {
	total := 0.0
	for _, b := range bs {
		total += b.Count
	}
	if total == 0 {
		return "—"
	}
	target := q * total
	sum := 0.0
	for _, b := range bs {
		if b.Count > 0 && sum+b.Count >= target {
			if math.IsInf(b.Low, -1) {
				return "≤ " + fmtNum(b.High)
			}
			if math.IsInf(b.High, 1) {
				return "≥ " + fmtNum(b.Low)
			}
			return "≈ " + fmtNum(b.Low+(b.High-b.Low)*(target-sum)/b.Count)
		}
		sum += b.Count
	}
	return "—"
}
func RatePoints(s Series) []Point {
	var out []Point
	for i, p := range s.Points {
		var dt, v float64
		if s.Delta {
			if p.Nano > p.Start && p.Start != 0 {
				dt = float64(p.Nano-p.Start) / 1e9
			} else if i > 0 {
				dt = elapsed(s.Points[i-1], p)
			}
			v = p.Y
		} else if i > 0 {
			prev := s.Points[i-1]
			dt = elapsed(prev, p)
			v = p.Y - prev.Y
			if v < 0 || (p.Start != 0 && p.Start != prev.Start) {
				if p.Start == 0 || p.Nano <= p.Start {
					continue
				}
				v = p.Y
				dt = float64(p.Nano-p.Start) / 1e9
			}
		}
		if dt > 0 {
			p.Y = v / dt
			out = append(out, p)
		}
	}
	return out
}
func elapsed(a, b Point) float64 {
	if a.Nano != 0 || b.Nano != 0 {
		if b.Nano <= a.Nano {
			return 0
		}
		return float64(b.Nano-a.Nano) / 1e9
	}
	return b.X - a.X
}
