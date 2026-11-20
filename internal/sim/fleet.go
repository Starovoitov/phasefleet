// Package sim runs a fleet in one process.
// The step is always core.Stepper. Delay is applied here, before the call:
// a neighbor whose phase has not arrived yet is left out of the slice.
package sim

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"sort"
	"strconv"

	"github.com/Starovoitov/phasefleet/internal/core"
)

const twoPi = 2 * math.Pi

const (
	StrategyAligned  = "aligned"
	StrategyJitter   = "jitter"
	StrategyHash     = "hash"
	StrategyKuramoto = "kuramoto"
	StrategyDesync   = "desync"
)

// Config is one run. Dt is the tick length in the same unit as T and D.
// KillAt < 0 disables the kill. Delay, when set, is a tick count per ordered
// pair: Delay[i][j] is how many ticks a phase from j takes to reach i.
// SinkDelay is the extra time from a firing to its arrival at the sink.
type Config struct {
	N            int
	D            float64
	T            float64
	Dt           float64
	Ticks        int
	Seed         int64
	Strategy     string
	K            float64
	Alpha        float64
	Delay        [][]int
	SinkDelay    []float64
	KillAt       int
	KillFraction float64
	Trace        bool
	// Aggregate is "mean" (the default) or "median". Median replaces the
	// Kuramoto average with the circular median. DESYNC has no average,
	// so median is refused there.
	Aggregate string
	// Lock holds LockIndex at LockedPhase. That member stays a neighbor
	// and does not step. LockIndex is ignored when Lock is false.
	Lock        bool
	LockIndex   int
	LockedPhase float64
}

// Node is one member after a run.
type Node struct {
	ID    string
	Phase core.Phase
	Alive bool
}

// Fire is one crossing of phase 0. Send is when the node fired.
// Arrival is Send plus that node's delay to the sink.
type Fire struct {
	Tick    int
	ID      string
	Send    float64
	Arrival float64
}

// Sample is one alive node on one tick, for the CSV trace.
type Sample struct {
	Tick    int
	ID      string
	Phase   float64
	Fired   bool
	Send    float64
	Arrival float64
}

// Result is the aggregate of a straight run.
type Result struct {
	Fires          []Fire
	Final          []Node
	Samples        []Sample
	Peak           int
	MedianInterval float64
}

// Fleet is the mutable population. A tick reads the phases already published,
// steps every live node, then publishes the new phases.
type Fleet struct {
	cfg      Config
	stepper  core.Stepper
	omega    float64
	dt       float64
	id       []string
	phase    []core.Phase
	next     []core.Phase
	alive    []bool
	order    []int
	nAlive   int
	ordered  bool
	time     float64
	tick     int
	shifts   int
	maxDelay int
	hist     [][]core.Phase
	delay    [][]int
	sink     []float64
	desync   bool
	free     bool
	trace    bool
	samples  []Sample
	minGap   float64
	maxGap   float64
	pair     [2]core.Observation
}

// New builds a fleet and publishes the initial phases.
func New(cfg Config) (*Fleet, error) {
	if cfg.N < 1 {
		return nil, fmt.Errorf("n must be positive")
	}
	if cfg.T <= 0 {
		return nil, fmt.Errorf("t must be positive")
	}
	if cfg.D < 0 {
		return nil, fmt.Errorf("d must be non-negative")
	}
	if cfg.KillAt < -1 {
		return nil, fmt.Errorf("kill-at must be >= -1")
	}
	if cfg.Delay != nil && (len(cfg.Delay) != cfg.N) {
		return nil, fmt.Errorf("delay matrix must be %d×%d", cfg.N, cfg.N)
	}
	for i, row := range cfg.Delay {
		if len(row) != cfg.N {
			return nil, fmt.Errorf("delay row %d must have %d entries", i, cfg.N)
		}
		for _, d := range row {
			if d < 0 {
				return nil, fmt.Errorf("delay must be >= 0")
			}
		}
	}
	if cfg.SinkDelay != nil && len(cfg.SinkDelay) != cfg.N {
		return nil, fmt.Errorf("sink delay must have %d entries", cfg.N)
	}
	if cfg.Lock && (cfg.LockIndex < 0 || cfg.LockIndex >= cfg.N) {
		return nil, fmt.Errorf("lock index %d out of range", cfg.LockIndex)
	}
	stepper, err := newStepper(cfg)
	if err != nil {
		return nil, err
	}
	f := &Fleet{
		cfg:      cfg,
		stepper:  stepper,
		omega:    twoPi / cfg.T,
		dt:       cfg.Dt,
		id:       make([]string, cfg.N),
		phase:    initialPhases(cfg.Strategy, cfg.N, cfg.Seed),
		next:     make([]core.Phase, cfg.N),
		alive:    make([]bool, cfg.N),
		order:    make([]int, cfg.N),
		nAlive:   cfg.N,
		delay:    cfg.Delay,
		sink:     cfg.SinkDelay,
		desync:   cfg.Strategy == StrategyDesync,
		trace:    cfg.Trace,
		maxDelay: maxDelay(cfg.Delay),
	}
	switch cfg.Strategy {
	case StrategyAligned, StrategyJitter, StrategyHash:
		f.free = true
	case StrategyKuramoto:
		f.free = cfg.K == 0
	}
	for i := range f.alive {
		f.alive[i] = true
		f.id[i] = strconv.Itoa(i)
		f.order[i] = i
	}
	f.applyLock()
	if f.maxDelay > 0 {
		f.hist = make([][]core.Phase, f.maxDelay+1)
		for i := range f.hist {
			f.hist[i] = make([]core.Phase, cfg.N)
		}
		copy(f.hist[0], f.phase)
	}
	return f, nil
}

// Run steps the fleet Ticks times and aggregates firings.
func Run(cfg Config) (Result, error) {
	if cfg.Ticks < 0 {
		return Result{}, fmt.Errorf("ticks must be >= 0")
	}
	if cfg.Ticks > 0 && cfg.Dt <= 0 {
		return Result{}, fmt.Errorf("dt must be positive")
	}
	f, err := New(cfg)
	if err != nil {
		return Result{}, err
	}
	var fires []Fire
	for t := 0; t < cfg.Ticks; t++ {
		fires = append(fires, f.Tick()...)
	}
	return f.result(fires), nil
}

// SetDt changes the tick length. Relaxation uses 0 so only the coupling moves.
func (f *Fleet) SetDt(dt float64) { f.dt = dt }

// Reseed replaces phases and revives every node. The strategy stays.
func (f *Fleet) Reseed(seed int64) {
	copy(f.phase, initialPhases(f.cfg.Strategy, len(f.phase), seed))
	f.applyLock()
	for i := range f.alive {
		f.alive[i] = true
	}
	f.time = 0
	f.tick = 0
	f.shifts = 0
	f.ordered = false
	f.nAlive = len(f.phase)
	f.samples = nil
	if f.hist != nil {
		for i := range f.hist {
			for j := range f.hist[i] {
				f.hist[i][j] = 0
			}
		}
		copy(f.hist[0], f.phase)
	}
}

// Tick advances every live node by one step. Phases are swapped only after
// every node has read the phases published on earlier ticks.
func (f *Fleet) Tick() []Fire {
	if f.tick == f.cfg.KillAt && f.cfg.KillFraction > 0 {
		f.kill(f.cfg.KillFraction)
	}
	var fires []Fire
	switch {
	case f.desync && f.maxDelay == 0:
		fires = f.advanceDesync()
	case f.free:
		fires = f.advanceFree()
	default:
		fires = f.advanceFull()
	}
	if f.trace {
		f.record(fires)
	}
	f.publish()
	f.time += f.dt
	f.tick++
	return fires
}

// Relax runs coupling-only ticks (dt = 0) until gaps sit within relTol of
// 2π/N, or max steps elapse. relTol is a fraction of the ideal gap.
func (f *Fleet) Relax(max int, relTol float64) (steps int, converged bool) {
	saved := f.dt
	f.dt = 0
	defer func() { f.dt = saved }()
	for s := 1; s <= max; s++ {
		f.Tick()
		if f.Uniform(relTol) {
			return s, true
		}
	}
	return max, false
}

// Uniform reports whether live gaps are within relTol of 2π/N.
func (f *Fleet) Uniform(relTol float64) bool {
	if f.nAlive < 2 {
		return true
	}
	ideal := twoPi / float64(f.nAlive)
	return f.minGap >= ideal*(1-relTol) && f.maxGap <= ideal*(1+relTol)
}

// GapMAD is the mean absolute deviation of live gaps from 2π/N.
func (f *Fleet) GapMAD() float64 {
	return gapMAD(f.livePhases())
}

// Order is the Kuramoto order parameter of the live phases.
func (f *Fleet) Order() (r, psi float64) {
	return core.Order(f.livePhases())
}

// KillFraction marks the highest-index live fraction dead.
// Dead nodes stop stepping and stop being neighbors.
func (f *Fleet) KillFraction(frac float64) { f.kill(frac) }

func (f *Fleet) kill(frac float64) {
	k := int(math.Floor(frac*float64(len(f.alive)) + 1e-9))
	if k > len(f.alive) {
		k = len(f.alive)
	}
	for i := len(f.alive) - k; i < len(f.alive); i++ {
		f.alive[i] = false
	}
	f.ordered = false
}

func (f *Fleet) result(fires []Fire) Result {
	arrivals := make([]float64, len(fires))
	for i, fire := range fires {
		arrivals[i] = fire.Arrival
	}
	final := make([]Node, len(f.phase))
	for i := range final {
		final[i] = Node{ID: f.id[i], Phase: f.phase[i], Alive: f.alive[i]}
	}
	return Result{
		Fires:          fires,
		Final:          final,
		Samples:        f.samples,
		Peak:           PeakOverlap(arrivals, f.cfg.D),
		MedianInterval: MedianInterval(arrivals),
	}
}

func (f *Fleet) advanceFree() []Fire {
	fires := make([]Fire, 0, f.nAlive)
	for i, alive := range f.alive {
		if !alive || f.locked(i) {
			continue
		}
		prev := f.phase[i]
		next := f.stepper.Step(prev, nil, f.dt)
		fires = append(fires, f.crossings(i, prev, next)...)
		f.phase[i] = next
	}
	return fires
}

func (f *Fleet) advanceDesync() []Fire {
	f.sortAlive()
	fires := make([]Fire, 0, f.nAlive)
	for k := 0; k < f.nAlive; k++ {
		i := f.order[k]
		if f.locked(i) {
			f.next[i] = f.phase[i]
			continue
		}
		obs := f.flanks(k)
		prev := f.phase[i]
		next := f.stepper.Step(prev, obs, f.dt)
		f.next[i] = next
	}
	for k := 0; k < f.nAlive; k++ {
		i := f.order[k]
		fires = append(fires, f.crossings(i, f.phase[i], f.next[i])...)
		f.phase[i] = f.next[i]
	}
	f.sortAlive()
	f.measureGaps()
	return fires
}

func (f *Fleet) advanceFull() []Fire {
	fires := make([]Fire, 0, f.nAlive)
	for i, alive := range f.alive {
		if !alive {
			continue
		}
		prev := f.phase[i]
		if f.locked(i) {
			f.next[i] = prev
			continue
		}
		obs := f.neighbors(i)
		next := f.stepper.Step(prev, obs, f.dt)
		f.next[i] = next
		fires = append(fires, f.crossings(i, prev, next)...)
	}
	for i, alive := range f.alive {
		if alive {
			f.phase[i] = f.next[i]
		}
	}
	f.ordered = false
	f.sortAlive()
	f.measureGaps()
	return fires
}

// flanks passes only the nearest ahead and behind. Desync ignores every
// other observation, so the step matches a full neighborhood.
func (f *Fleet) flanks(k int) []core.Observation {
	n := f.nAlive
	if n < 2 {
		return nil
	}
	i := f.order[k]
	succ, pred := -1, -1
	for s := 1; s < n; s++ {
		j := f.order[(k+s)%n]
		if f.phase[j] != f.phase[i] {
			succ = j
			break
		}
	}
	for s := 1; s < n; s++ {
		j := f.order[(k-s+n)%n]
		if f.phase[j] != f.phase[i] {
			pred = j
			break
		}
	}
	if succ < 0 {
		return nil
	}
	f.pair[0].Phase = f.phase[succ]
	if pred < 0 || pred == succ {
		return f.pair[:1]
	}
	f.pair[1].Phase = f.phase[pred]
	return f.pair[:2]
}

func (f *Fleet) neighbors(i int) []core.Observation {
	obs := make([]core.Observation, 0, len(f.phase)-1)
	for j := range f.phase {
		if j == i || !f.alive[j] {
			continue
		}
		p, ok := f.seen(j, f.delayOf(i, j))
		if !ok {
			continue
		}
		obs = append(obs, core.Observation{ID: f.id[j], Phase: p})
	}
	return obs
}

func (f *Fleet) delayOf(i, j int) int {
	if f.delay == nil {
		return 0
	}
	return f.delay[i][j]
}

func (f *Fleet) seen(j, ago int) (core.Phase, bool) {
	if ago == 0 {
		return f.phase[j], true
	}
	if ago > f.shifts || f.hist == nil {
		return 0, false
	}
	return f.hist[ago][j], true
}

func (f *Fleet) crossings(i int, prev, next core.Phase) []Fire {
	if f.dt <= 0 {
		return nil
	}
	base := core.Wrap(float64(prev) + f.omega*f.dt)
	add := f.omega*f.dt + core.Delta(base, next)
	if add <= 1e-12 {
		return nil
	}
	start := float64(prev)
	var out []Fire
	for k := math.Floor(start/twoPi) + 1; ; k++ {
		boundary := k * twoPi
		if boundary > start+add+1e-9 {
			break
		}
		u := f.dt * (boundary - start) / add
		if u < 0 || u > f.dt+1e-9 {
			continue
		}
		if u > f.dt {
			u = f.dt
		}
		send := f.time + u
		out = append(out, Fire{
			Tick:    f.tick,
			ID:      f.id[i],
			Send:    send,
			Arrival: send + f.sinkDelay(i),
		})
		if k > start/twoPi+4 {
			break
		}
	}
	return out
}

func (f *Fleet) sinkDelay(i int) float64 {
	if f.sink == nil {
		return 0
	}
	return f.sink[i]
}

func (f *Fleet) publish() {
	if f.maxDelay == 0 {
		return
	}
	newest := f.hist[f.maxDelay]
	copy(newest, f.phase)
	for k := f.maxDelay; k > 0; k-- {
		f.hist[k] = f.hist[k-1]
	}
	f.hist[0] = newest
	if f.shifts < f.maxDelay {
		f.shifts++
	}
}

func (f *Fleet) record(fires []Fire) {
	byID := make(map[string]Fire, len(fires))
	for _, fire := range fires {
		if _, ok := byID[fire.ID]; !ok {
			byID[fire.ID] = fire
		}
	}
	for i, alive := range f.alive {
		if !alive {
			continue
		}
		s := Sample{Tick: f.tick, ID: f.id[i], Phase: float64(f.phase[i])}
		if fire, ok := byID[f.id[i]]; ok {
			s.Fired = true
			s.Send = fire.Send
			s.Arrival = fire.Arrival
		}
		f.samples = append(f.samples, s)
	}
}

func (f *Fleet) sortAlive() {
	f.nAlive = 0
	for i, alive := range f.alive {
		if alive {
			f.order[f.nAlive] = i
			f.nAlive++
		}
	}
	sort.Slice(f.order[:f.nAlive], func(a, b int) bool {
		return f.phase[f.order[a]] < f.phase[f.order[b]]
	})
	f.ordered = true
}

func (f *Fleet) measureGaps() {
	n := f.nAlive
	if n < 2 {
		f.minGap, f.maxGap = 0, 0
		return
	}
	f.minGap = math.Inf(1)
	f.maxGap = 0
	for k := 0; k < n; k++ {
		a := float64(f.phase[f.order[k]])
		b := float64(f.phase[f.order[(k+1)%n]])
		gap := b - a
		if k == n-1 {
			gap = b + twoPi - a
		}
		if gap < f.minGap {
			f.minGap = gap
		}
		if gap > f.maxGap {
			f.maxGap = gap
		}
	}
}

func (f *Fleet) livePhases() []core.Phase {
	out := make([]core.Phase, 0, f.nAlive)
	for i, alive := range f.alive {
		if alive {
			out = append(out, f.phase[i])
		}
	}
	return out
}

func (f *Fleet) locked(i int) bool {
	return f.cfg.Lock && i == f.cfg.LockIndex
}

func (f *Fleet) applyLock() {
	if !f.cfg.Lock {
		return
	}
	f.phase[f.cfg.LockIndex] = core.Wrap(f.cfg.LockedPhase)
}

func newStepper(cfg Config) (core.Stepper, error) {
	aggregate := cfg.Aggregate
	if aggregate == "" {
		aggregate = "mean"
	}
	switch aggregate {
	case "mean", "median":
	default:
		return nil, fmt.Errorf("unknown aggregate %q", cfg.Aggregate)
	}
	if aggregate == "median" && cfg.Strategy != StrategyKuramoto {
		return nil, fmt.Errorf("median aggregate is only defined for kuramoto")
	}
	switch cfg.Strategy {
	case StrategyAligned, StrategyJitter, StrategyHash:
		return core.NewKuramoto(cfg.T, 0), nil
	case StrategyKuramoto:
		if aggregate == "median" {
			return core.NewMedianKuramoto(cfg.T, cfg.K), nil
		}
		return core.NewKuramoto(cfg.T, cfg.K), nil
	case StrategyDesync:
		return core.NewDesync(cfg.T, cfg.Alpha), nil
	default:
		return nil, fmt.Errorf("unknown strategy %q", cfg.Strategy)
	}
}

func initialPhases(strategy string, n int, seed int64) []core.Phase {
	switch strategy {
	case StrategyAligned:
		return make([]core.Phase, n)
	case StrategyHash:
		out := make([]core.Phase, n)
		for i := range out {
			out[i] = hashPhase(strconv.Itoa(i))
		}
		return out
	default:
		return jitterPhases(n, seed)
	}
}

func jitterPhases(n int, seed int64) []core.Phase {
	r := rand.New(rand.NewPCG(uint64(seed), 1))
	out := make([]core.Phase, n)
	for i := range out {
		out[i] = core.Phase(r.Float64() * twoPi)
	}
	return out
}

func hashPhase(id string) core.Phase {
	h := fnv.New64a()
	_, _ = h.Write([]byte(id))
	// FNV of a short decimal id leaves clumps in the high bits, so a raw
	// phase piles hundreds of nodes into one slot. The final mix is what
	// makes this the Prometheus-style deterministic offset.
	mixed := splitmix64(h.Sum64())
	return core.Wrap((float64(mixed) / math.MaxUint64) * twoPi)
}

func splitmix64(z uint64) uint64 {
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func maxDelay(delay [][]int) int {
	m := 0
	for _, row := range delay {
		for _, d := range row {
			if d > m {
				m = d
			}
		}
	}
	return m
}
