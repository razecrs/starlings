package starlings

import (
	"fmt"
	"math/bits"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// GuardImplementation identifies the implementation a profile sample used.
type GuardImplementation string

const (
	GuardAutomatic   GuardImplementation = "automatic"
	GuardManual      GuardImplementation = "manual"
	GuardApplication GuardImplementation = "application"
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
	cache          map[string]*guardCacheMetric
	state          *State
	asyncEvents    bool
	intents        Intent
	started        time.Time
	warmup         time.Duration
}

type guardCacheMetric struct {
	calls, hits, items uint64
	uses               uint64 // dependencies read by composed/derived accessors
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
		features: make(map[string]struct{}), cache: make(map[string]*guardCacheMetric),
		maxFeatures: 256, started: time.Now(), warmup: 30 * time.Second,
	}
	for _, option := range options {
		option(g)
	}
	return g
}

// WithGuardWarmup delays cache-rightsizing advice until Guard has observed a
// representative workload. Timing and correctness metrics are still recorded
// immediately. The default is 30 seconds; zero is useful in tests.
func WithGuardWarmup(warmup time.Duration) GuardOption {
	return func(g *Guard) {
		if warmup >= 0 {
			g.warmup = warmup
		}
	}
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

func (g *Guard) attach(state *State, asyncEvents bool, intents Intent) {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.state = state
	g.asyncEvents = asyncEvents
	g.intents = intents
	g.mu.Unlock()
}

func (g *Guard) cacheAccess(name string, hit bool, items int, duration time.Duration) {
	if g == nil {
		return
	}
	g.observe("cache."+name, GuardAutomatic, duration, nil)
	g.mu.Lock()
	metric := g.cache[name]
	if metric == nil {
		metric = &guardCacheMetric{}
		g.cache[name] = metric
	}
	metric.calls++
	if hit {
		metric.hits++
	}
	if items > 0 {
		metric.items += uint64(items)
	}
	g.mu.Unlock()
}

func (g *Guard) cacheUse(names ...string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	for _, name := range names {
		metric := g.cache[name]
		if metric == nil {
			metric = &guardCacheMetric{}
			g.cache[name] = metric
		}
		metric.uses++
	}
	g.mu.Unlock()
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
	Findings       []GuardFinding
	Runtime        GuardRuntime
	DroppedSamples uint64
}

// GuardFinding is an evidence-backed performance or memory recommendation.
type GuardFinding struct {
	Severity       string
	Area           string
	Evidence       string
	Recommendation string
}

// GuardRuntime is a cheap process snapshot taken when Report is called.
type GuardRuntime struct {
	HeapAlloc  uint64
	HeapInUse  uint64
	Goroutines int
	NumGC      uint32
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
		_, hasApplication := implementations[GuardApplication]
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
		case hasApplication:
			advice.Recommendation = "application handler timing captured; see findings for blocking work"
		default:
			advice.Recommendation = "automatic path only"
		}
		report.Advice = append(report.Advice, advice)
	}
	state := g.state
	asyncEvents := g.asyncEvents
	intents := g.intents
	warmedUp := time.Since(g.started) >= g.warmup
	cache := make(map[string]guardCacheMetric, len(g.cache))
	for name, metric := range g.cache {
		cache[name] = *metric
	}
	g.mu.RUnlock()

	for _, metric := range report.Metrics {
		if metric.Implementation == GuardApplication && strings.HasPrefix(metric.Feature, "handler.") && metric.P95 >= 50*time.Millisecond && !asyncEvents {
			report.Findings = append(report.Findings, GuardFinding{
				Severity: "warning", Area: metric.Feature,
				Evidence:       fmt.Sprintf("p95=%s max=%s across %d calls", metric.P95, metric.Maximum, metric.Calls),
				Recommendation: "this synchronous handler delays every later handler; move blocking I/O to a goroutine or enable WithAsyncEvents(true)",
			})
		}
	}
	if warmedUp && state != nil {
		report.Findings = append(report.Findings, guardCacheFindings(state.Config(), state.Stats(), cache, intents)...)
	}
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	report.Runtime = GuardRuntime{HeapAlloc: memory.HeapAlloc, HeapInUse: memory.HeapInuse, Goroutines: runtime.NumGoroutine(), NumGC: memory.NumGC}
	sort.Slice(report.Metrics, func(i, j int) bool {
		if report.Metrics[i].Feature == report.Metrics[j].Feature {
			return report.Metrics[i].Implementation < report.Metrics[j].Implementation
		}
		return report.Metrics[i].Feature < report.Metrics[j].Feature
	})
	sort.Slice(report.Advice, func(i, j int) bool { return report.Advice[i].Feature < report.Advice[j].Feature })
	sort.Slice(report.Findings, func(i, j int) bool {
		if report.Findings[i].Severity == report.Findings[j].Severity {
			return report.Findings[i].Area < report.Findings[j].Area
		}
		return report.Findings[i].Severity < report.Findings[j].Severity
	})
	return report
}

func guardCacheFindings(config StateConfig, stats StateStats, cache map[string]guardCacheMetric, intents Intent) []GuardFinding {
	type category struct {
		name    string
		enabled bool
		items   int
	}
	categories := []category{
		{"guilds", config.Guilds, stats.Guilds}, {"channels", config.Channels, stats.Channels},
		{"members", config.Members, stats.Members}, {"users", config.Users, stats.Users},
		{"roles", config.Roles, stats.Roles}, {"emojis", config.Emojis, stats.Emojis},
		{"stickers", config.Stickers, stats.Stickers}, {"thread_members", config.ThreadMembers, stats.ThreadMembers},
		{"voice_states", config.VoiceStates, stats.VoiceStates}, {"presences", config.Presences, stats.Presences},
		{"messages", config.MaxMessagesPerChannel > 0, stats.Messages},
	}
	var findings []GuardFinding
	intentChecks := []struct {
		name    string
		enabled bool
		need    Intent
	}{
		{"members", config.Members, IntentGuildMembers},
		{"presences", config.Presences, IntentGuildPresences},
		{"voice_states", config.VoiceStates, IntentGuildVoiceStates},
		{"emojis and stickers", config.Emojis || config.Stickers, IntentGuildExpressions},
		{"messages", config.MaxMessagesPerChannel > 0, IntentGuildMessages | IntentDirectMessages},
	}
	for _, check := range intentChecks {
		if check.enabled && intents&check.need == 0 {
			findings = append(findings, GuardFinding{
				Severity: "info", Area: "cache." + strings.ReplaceAll(check.name, " ", "_"),
				Evidence:       "cache is enabled but none of its gateway intents are requested",
				Recommendation: "enable the matching intent if this data is needed, otherwise disable this cache category",
			})
		}
	}
	for _, category := range categories {
		metric := cache[category.name]
		for name, candidate := range cache {
			if strings.HasPrefix(name, category.name+".") {
				metric.calls += candidate.calls
				metric.hits += candidate.hits
				metric.items += candidate.items
				metric.uses += candidate.uses
			}
		}
		if category.enabled && category.items > 0 && metric.calls == 0 && metric.uses == 0 {
			findings = append(findings, GuardFinding{
				Severity: "info", Area: "cache." + category.name,
				Evidence:       fmt.Sprintf("%d cached objects and no observed reads", category.items),
				Recommendation: "disable this StateConfig category if the bot does not consume it",
			})
		}
		if metric.calls >= 20 && metric.hits*4 < metric.calls*3 {
			findings = append(findings, GuardFinding{
				Severity: "warning", Area: "cache." + category.name,
				Evidence:       fmt.Sprintf("%d/%d lookups hit", metric.hits, metric.calls),
				Recommendation: "the cache is often incomplete; request the missing gateway data or use an explicit REST fallback",
			})
		}
	}
	if config.MaxMessagesPerChannel > 0 && stats.MessageChannels >= 4 {
		capacity := config.MaxMessagesPerChannel * stats.MessageChannels
		if capacity >= 100 && stats.Messages*4 < capacity {
			findings = append(findings, GuardFinding{
				Severity: "info", Area: "cache.messages.capacity",
				Evidence:       fmt.Sprintf("%d messages retained across %d channels; configured capacity is %d", stats.Messages, stats.MessageChannels, capacity),
				Recommendation: "consider lowering MaxMessagesPerChannel if this occupancy is representative",
			})
		}
	}
	for name, metric := range cache {
		if strings.HasSuffix(name, ".all") && metric.calls >= 5 && metric.items/metric.calls >= 100 {
			recommendation := "prefer an ID-based accessor in hot code; full snapshots clone and sort every returned object"
			if name == "guilds.all" {
				recommendation = "use State.GuildInfo for guild metadata or State.MemberCount for cached counts; keep State.Guild for full snapshots"
			} else if name == "members.all" {
				recommendation = "use State.Member for an ID lookup or State.MemberCount for a cached count; keep State.Members when you need every member"
			}
			findings = append(findings, GuardFinding{
				Severity: "warning", Area: "cache." + name,
				Evidence:       fmt.Sprintf("average snapshot contains %d objects", metric.items/metric.calls),
				Recommendation: recommendation,
			})
		}
	}
	return findings
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
	if len(r.Metrics) == 0 && len(r.Findings) == 0 {
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
	for _, finding := range r.Findings {
		fmt.Fprintf(&out, "%s [%s]: %s (%s)\n", finding.Area, finding.Severity, finding.Recommendation, finding.Evidence)
	}
	fmt.Fprintf(&out, "runtime: heap=%d MiB in-use=%d MiB goroutines=%d gc=%d\n",
		r.Runtime.HeapAlloc/(1024*1024), r.Runtime.HeapInUse/(1024*1024), r.Runtime.Goroutines, r.Runtime.NumGC)
	return strings.TrimSuffix(out.String(), "\n")
}
