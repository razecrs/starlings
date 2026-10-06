package starlings

import (
	"fmt"
	"math/bits"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

// GuardImplementation identifies the implementation a profile sample used.
type GuardImplementation string

const (
	GuardAutomatic GuardImplementation = "automatic"
	GuardManual    GuardImplementation = "manual"
)

// GuardOption tunes Starlings Guard.
type GuardOption func(*Guard)

// Guard is a small in-process profiler for manual and automatic
// implementations. It stores fixed-size histograms, never request payloads.
type Guard struct {
	mu             sync.RWMutex
	metrics        map[guardKey]*guardMetric
	comparisons    map[string]*guardComparison
	features       map[string]struct{}
	maxFeatures    int
	droppedSamples uint64
}

type guardKey struct {
	feature        string
	implementation GuardImplementation
}

type guardMetric struct {
	calls, errors uint64
	total         time.Duration
	min, max      time.Duration
	buckets       [64]uint64
}

type guardComparison struct {
	checks, mismatches uint64
}

// NewGuard creates an empty profiler.
func NewGuard(options ...GuardOption) *Guard {
	g := &Guard{
		metrics: make(map[guardKey]*guardMetric), comparisons: make(map[string]*guardComparison),
		features: make(map[string]struct{}), maxFeatures: 256,
	}
	for _, option := range options {
		option(g)
	}
	return g
}

// WithGuardMaxFeatures bounds distinct labels retained by Guard. Additional
// labels are combined under "other". The default is 256.
func WithGuardMaxFeatures(maximum int) GuardOption {
	return func(g *Guard) {
		if maximum > 0 {
			g.maxFeatures = maximum
		}
	}
}

// WithGuard enables Starlings Guard for this client.
func WithGuard(options ...GuardOption) Option {
	return func(c *Client) { c.guard = NewGuard(options...) }
}

// Guard returns this client's profiler, or nil when it was not enabled.
func (c *Client) Guard() *Guard {
	if c == nil {
		return nil
	}
	return c.rootClient().guard
}

// Measure profiles one implementation without executing it twice. Use this
// for side-effecting work such as REST calls and message sends.
func (g *Guard) Measure(feature string, implementation GuardImplementation, fn func() error) error {
	if g == nil {
		return fn()
	}
	started := time.Now()
	err := fn()
	g.observe(feature, implementation, time.Since(started), err)
	return err
}

// Observe records a measurement made by application code.
func (g *Guard) Observe(feature string, implementation GuardImplementation, duration time.Duration, err error) {
	if g == nil {
		return
	}
	g.observe(feature, implementation, duration, err)
}

func (g *Guard) observe(feature string, implementation GuardImplementation, duration time.Duration, err error) {
	if feature == "" {
		feature = "unnamed"
	}
	g.mu.Lock()
	feature = g.featureLocked(feature)
	key := guardKey{feature: feature, implementation: implementation}
	metric := g.metrics[key]
	if metric == nil {
		metric = &guardMetric{}
		g.metrics[key] = metric
	}
	metric.calls++
	if err != nil {
		metric.errors++
	}
	metric.total += duration
	if metric.calls == 1 || duration < metric.min {
		metric.min = duration
	}
	if duration > metric.max {
		metric.max = duration
	}
	ns := uint64(duration)
	bucket := 0
	if ns > 0 {
		bucket = bits.Len64(ns)
	}
	if bucket >= len(metric.buckets) {
		bucket = len(metric.buckets) - 1
	}
	metric.buckets[bucket]++
	g.mu.Unlock()
}

func (g *Guard) featureLocked(feature string) string {
	if _, ok := g.features[feature]; ok {
		return feature
	}
	if len(g.features) < g.maxFeatures {
		g.features[feature] = struct{}{}
		return feature
	}
	g.droppedSamples++
	return "other"
}

// GuardCompare runs and profiles two pure implementations and returns the
// manual result. It must not be used for code with side effects: both
// functions are intentionally executed.
func GuardCompare[T any](g *Guard, feature string, equal func(T, T) bool, manual, automatic func() (T, error)) (T, error) {
	started := time.Now()
	manualValue, manualErr := manual()
	if g != nil {
		g.observe(feature, GuardManual, time.Since(started), manualErr)
	}

	started = time.Now()
	automaticValue, automaticErr := automatic()
	if g != nil {
		g.observe(feature, GuardAutomatic, time.Since(started), automaticErr)
		match := (manualErr == nil) == (automaticErr == nil)
		if match && manualErr == nil {
			if equal != nil {
				match = equal(manualValue, automaticValue)
			} else {
				match = reflect.DeepEqual(manualValue, automaticValue)
			}
		}
		g.mu.Lock()
		feature = g.featureLocked(feature)
		comparison := g.comparisons[feature]
		if comparison == nil {
			comparison = &guardComparison{}
			g.comparisons[feature] = comparison
		}
		comparison.checks++
		if !match {
			comparison.mismatches++
		}
		g.mu.Unlock()
	}
	return manualValue, manualErr
}

// GuardMetric is one immutable row in a Guard report.
type GuardMetric struct {
	Feature        string
	Implementation GuardImplementation
	Calls          uint64
	Errors         uint64
	Total          time.Duration
	Average        time.Duration
	Minimum        time.Duration
	Maximum        time.Duration
	P50            time.Duration
	P95            time.Duration
}

// GuardAdvice compares the observed implementations for one feature.
type GuardAdvice struct {
	Feature        string
	Checks         uint64
	Mismatches     uint64
	Recommendation string
}

// GuardReport is a race-safe point-in-time profile snapshot.
type GuardReport struct {
	Metrics        []GuardMetric
	Advice         []GuardAdvice
	DroppedSamples uint64
}

// Report returns stable, feature-sorted metrics and recommendations.
func (g *Guard) Report() GuardReport {
	if g == nil {
		return GuardReport{}
	}
	g.mu.RLock()
	report := GuardReport{Metrics: make([]GuardMetric, 0, len(g.metrics)), DroppedSamples: g.droppedSamples}
	byFeature := make(map[string]map[GuardImplementation]GuardMetric)
	for key, metric := range g.metrics {
		row := GuardMetric{
			Feature: key.feature, Implementation: key.implementation,
			Calls: metric.calls, Errors: metric.errors, Total: metric.total,
			Minimum: metric.min, Maximum: metric.max,
			P50: histogramPercentile(metric.buckets, metric.calls, 50),
			P95: histogramPercentile(metric.buckets, metric.calls, 95),
		}
		if metric.calls != 0 {
			row.Average = metric.total / time.Duration(metric.calls)
		}
		report.Metrics = append(report.Metrics, row)
		if byFeature[key.feature] == nil {
			byFeature[key.feature] = make(map[GuardImplementation]GuardMetric)
		}
		byFeature[key.feature][key.implementation] = row
	}
	for feature, implementations := range byFeature {
		comparison := g.comparisons[feature]
		advice := GuardAdvice{Feature: feature}
		if comparison != nil {
			advice.Checks, advice.Mismatches = comparison.checks, comparison.mismatches
		}
		manual, hasManual := implementations[GuardManual]
		automatic, hasAutomatic := implementations[GuardAutomatic]
		switch {
		case advice.Mismatches > 0:
			advice.Recommendation = "outputs differ; inspect the manual implementation before choosing"
		case hasManual && hasAutomatic && manual.Errors > automatic.Errors:
			advice.Recommendation = "automatic is more stable in observed calls"
		case hasManual && hasAutomatic && manual.Average > automatic.Average+automatic.Average/10:
			advice.Recommendation = "automatic is faster in observed calls"
		case hasManual && hasAutomatic:
			advice.Recommendation = "manual is competitive; choose it only if its extra control is useful"
		case hasManual:
			advice.Recommendation = "manual path detected; add a safe automatic comparison or compare production metrics"
		default:
			advice.Recommendation = "automatic path only"
		}
		report.Advice = append(report.Advice, advice)
	}
	g.mu.RUnlock()
	sort.Slice(report.Metrics, func(i, j int) bool {
		if report.Metrics[i].Feature == report.Metrics[j].Feature {
			return report.Metrics[i].Implementation < report.Metrics[j].Implementation
		}
		return report.Metrics[i].Feature < report.Metrics[j].Feature
	})
	sort.Slice(report.Advice, func(i, j int) bool { return report.Advice[i].Feature < report.Advice[j].Feature })
	return report
}

func histogramPercentile(buckets [64]uint64, count, percentile uint64) time.Duration {
	if count == 0 {
		return 0
	}
	target := (count*percentile + 99) / 100
	var seen uint64
	for bucket, amount := range buckets {
		seen += amount
		if seen >= target {
			if bucket == 0 {
				return 0
			}
			return time.Duration(uint64(1) << (bucket - 1))
		}
	}
	return 0
}

// String formats a compact report suitable for logs and development output.
func (r GuardReport) String() string {
	if len(r.Metrics) == 0 {
		return "Starlings Guard: no samples"
	}
	var out strings.Builder
	out.WriteString("Starlings Guard\n")
	for _, metric := range r.Metrics {
		fmt.Fprintf(&out, "%s [%s]: calls=%d errors=%d avg=%s p95≈%s max=%s\n",
			metric.Feature, metric.Implementation, metric.Calls, metric.Errors, metric.Average, metric.P95, metric.Maximum)
	}
	for _, advice := range r.Advice {
		fmt.Fprintf(&out, "%s: %s\n", advice.Feature, advice.Recommendation)
	}
	return strings.TrimSuffix(out.String(), "\n")
}
