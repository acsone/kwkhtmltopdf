package main

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Minimal Prometheus text exposition implementation.
// Supports the subset of features used by this server (CounterVec, HistogramVec).

var DefBuckets = []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 240}

type CounterOpts struct {
	Name string
	Help string
}

type HistogramOpts struct {
	Name    string
	Help    string
	Buckets []float64
}

type promMetric interface {
	writeProm(w io.Writer)
}

type registry struct {
	mu      sync.Mutex
	metrics []promMetric
}

var defaultRegistry = &registry{}

func (r *registry) register(m promMetric) {
	r.mu.Lock()
	r.metrics = append(r.metrics, m)
	r.mu.Unlock()
}

func (r *registry) writeAll(w io.Writer) {
	r.mu.Lock()
	metrics := append([]promMetric(nil), r.metrics...)
	r.mu.Unlock()
	for _, m := range metrics {
		m.writeProm(w)
	}
}

func MetricsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	defaultRegistry.writeAll(w)
}

// --- Counter ---

type CounterVec struct {
	name       string
	help       string
	labelNames []string

	mu     sync.Mutex
	series map[string]*counterSeries
}

type counterSeries struct {
	labelValues []string
	valueBits   uint64 // atomic float64
}

type Counter struct{ s *counterSeries }

func NewCounterVec(opts CounterOpts, labelNames []string) *CounterVec {
	cv := &CounterVec{
		name:       opts.Name,
		help:       opts.Help,
		labelNames: append([]string(nil), labelNames...),
		series:     make(map[string]*counterSeries),
	}
	defaultRegistry.register(cv)
	return cv
}

func (v *CounterVec) WithLabelValues(values ...string) *Counter {
	if len(values) != len(v.labelNames) {
		panic(fmt.Sprintf("%s: expected %d label values, got %d", v.name, len(v.labelNames), len(values)))
	}
	key := encodeLabelValues(values)
	v.mu.Lock()
	s := v.series[key]
	if s == nil {
		s = &counterSeries{labelValues: append([]string(nil), values...)}
		v.series[key] = s
	}
	v.mu.Unlock()
	return &Counter{s: s}
}

func (c *Counter) Inc() { c.Add(1) }

func (c *Counter) Add(v float64) {
	if v < 0 {
		// Prometheus counters are expected to be monotonic; ignore negative adds.
		return
	}
	atomicAddFloat64(&c.s.valueBits, v)
}

func (v *CounterVec) writeProm(w io.Writer) {
	fmt.Fprintf(w, "# HELP %s %s\n", v.name, escapeHelp(v.help))
	fmt.Fprintf(w, "# TYPE %s counter\n", v.name)
	keys, series := v.snapshotSeries()
	for _, k := range keys {
		s := series[k]
		val := atomicLoadFloat64(&s.valueBits)
		fmt.Fprintf(w, "%s%s %s\n", v.name, formatLabelSet(v.labelNames, s.labelValues), formatFloat(val))
	}
}

// --- Histogram ---

type HistogramVec struct {
	name       string
	help       string
	labelNames []string
	buckets    []float64

	mu     sync.Mutex
	series map[string]*histogramSeries
}

type histogramSeries struct {
	labelValues []string
	bucketCnt   []uint64 // per-bucket counts, last is +Inf
	sumBits     uint64   // atomic float64
	count       uint64   // atomic uint64
}

type Histogram struct {
	s       *histogramSeries
	buckets []float64
}

func NewHistogramVec(opts HistogramOpts, labelNames []string) *HistogramVec {
	buckets := append([]float64(nil), opts.Buckets...)
	if len(buckets) == 0 {
		buckets = append([]float64(nil), DefBuckets...)
	}
	// Ensure buckets are sorted increasing.
	sort.Float64s(buckets)

	hv := &HistogramVec{
		name:       opts.Name,
		help:       opts.Help,
		labelNames: append([]string(nil), labelNames...),
		buckets:    buckets,
		series:     make(map[string]*histogramSeries),
	}
	defaultRegistry.register(hv)
	return hv
}

func (v *HistogramVec) WithLabelValues(values ...string) *Histogram {
	if len(values) != len(v.labelNames) {
		panic(fmt.Sprintf("%s: expected %d label values, got %d", v.name, len(v.labelNames), len(values)))
	}
	key := encodeLabelValues(values)
	v.mu.Lock()
	s := v.series[key]
	if s == nil {
		s = &histogramSeries{
			labelValues: append([]string(nil), values...),
			bucketCnt:   make([]uint64, len(v.buckets)+1),
		}
		v.series[key] = s
	}
	v.mu.Unlock()
	return &Histogram{s: s, buckets: v.buckets}
}

func (h *Histogram) Observe(v float64) {
	idx := len(h.buckets) // +Inf by default
	for i, b := range h.buckets {
		if v <= b {
			idx = i
			break
		}
	}
	atomic.AddUint64(&h.s.bucketCnt[idx], 1)
	atomic.AddUint64(&h.s.count, 1)
	atomicAddFloat64(&h.s.sumBits, v)
}

func (v *HistogramVec) writeProm(w io.Writer) {
	fmt.Fprintf(w, "# HELP %s %s\n", v.name, escapeHelp(v.help))
	fmt.Fprintf(w, "# TYPE %s histogram\n", v.name)

	keys, series := v.snapshotSeries()
	for _, k := range keys {
		s := series[k]

		baseLabels := make([]string, 0, len(v.labelNames)+1)
		baseLabels = append(baseLabels, s.labelValues...)

		// Buckets are cumulative in exposition format.
		var cumulative uint64
		for i, b := range v.buckets {
			cumulative += atomic.LoadUint64(&s.bucketCnt[i])
			fmt.Fprintf(w, "%s_bucket%s %d\n", v.name, formatLabelSet(append(v.labelNames, "le"), append(baseLabels, formatFloat(b))), cumulative)
		}
		cumulative += atomic.LoadUint64(&s.bucketCnt[len(v.buckets)])
		fmt.Fprintf(w, "%s_bucket%s %d\n", v.name, formatLabelSet(append(v.labelNames, "le"), append(baseLabels, "+Inf")), cumulative)

		sum := atomicLoadFloat64(&s.sumBits)
		count := atomic.LoadUint64(&s.count)
		fmt.Fprintf(w, "%s_sum%s %s\n", v.name, formatLabelSet(v.labelNames, s.labelValues), formatFloat(sum))
		fmt.Fprintf(w, "%s_count%s %d\n", v.name, formatLabelSet(v.labelNames, s.labelValues), count)
	}
}

// --- Helpers ---

func (v *CounterVec) snapshotSeries() ([]string, map[string]*counterSeries) {
	v.mu.Lock()
	keys := make([]string, 0, len(v.series))
	for k := range v.series {
		keys = append(keys, k)
	}
	series := make(map[string]*counterSeries, len(v.series))
	for k, s := range v.series {
		series[k] = s
	}
	v.mu.Unlock()
	sort.Strings(keys)
	return keys, series
}

func (v *HistogramVec) snapshotSeries() ([]string, map[string]*histogramSeries) {
	v.mu.Lock()
	keys := make([]string, 0, len(v.series))
	for k := range v.series {
		keys = append(keys, k)
	}
	series := make(map[string]*histogramSeries, len(v.series))
	for k, s := range v.series {
		series[k] = s
	}
	v.mu.Unlock()
	sort.Strings(keys)
	return keys, series
}

func encodeLabelValues(values []string) string {
	var b strings.Builder
	for _, v := range values {
		b.WriteString(strconv.Itoa(len(v)))
		b.WriteByte(':')
		b.WriteString(v)
		b.WriteByte('|')
	}
	return b.String()
}

func escapeHelp(s string) string {
	// Prometheus help strings should not contain newlines.
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

func escapeLabelValue(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

func formatLabelSet(names, values []string) string {
	if len(names) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteByte('{')
	for i := range names {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(names[i])
		b.WriteString("=\"")
		b.WriteString(escapeLabelValue(values[i]))
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func atomicLoadFloat64(bits *uint64) float64 {
	return math.Float64frombits(atomic.LoadUint64(bits))
}

func atomicAddFloat64(bits *uint64, delta float64) {
	for {
		oldBits := atomic.LoadUint64(bits)
		old := math.Float64frombits(oldBits)
		newVal := old + delta
		newBits := math.Float64bits(newVal)
		if atomic.CompareAndSwapUint64(bits, oldBits, newBits) {
			return
		}
	}
}
