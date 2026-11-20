# phasefleet

Coordinator-free phase coordination for service fleets. Members agree on a shared rhythm for periodic work, and they can spread that work around the period so it does not land in one pile. There is no leader. A member that dies drops out of the schedule; the ones still alive keep going and, under DESYNC, close the gaps.

This is coordination of load, not a clock. Wall time stays with chrony, NTP, PTP, or a cloud time source.

The numbers below come from the in-process simulator. `cmd/node` runs the same step over UDP and POSTs a firing to `cmd/sink`. The sink times that arrival on its own clock and serves concurrency, peak, and the interval histogram at `/metrics`. An empty `-sink` only logs the firing. The node also serves `θ − ω₀·t` at its own `/metrics`.

## The problem

A fleet often runs the same periodic job on every member: a cron at `:00`, a cache sweep, a compaction, a scrape, a health check. If every member starts together, the shared database or upstream API sees all of them at once.

`N` is the member count, `d` is how long one run of the job takes, and `T` is the period on which every member must run it again. The average number of overlapping runs, if the starts are spread evenly, is `N·d/T`. The ideal peak is `⌈N·d/T⌉`.

Aligned starts reach a peak of `N`. Random jitter is the usual fix. It draws each phase once and then leaves it alone. Those draws clump: with many members and few slots, the busiest slot is well above the ideal. A hash of the member id (the same idea as a Prometheus scrape offset or a Jenkins `H` schedule) is a stable draw. It does not clump by accident of a shared seed, and it still collides the way any fixed assignment does.

## Why oscillators

Each member keeps a phase on `[0, 2π)`. One full turn is one period, `ω₀ = 2π/T`. The job fires when the phase crosses 0. The next phase is a function of the member's own phase and the phases it has heard. No member stores the roster of start times, and no member is the source of truth.

Two couplings sit on that circle:

- Kuramoto with `K > 0` pulls phases together, so the fleet shares one firing time. The step is `dθ/dt = ω₀ + (K/n) Σ sin(θⱼ − θᵢ)`.
- DESYNC spreads phases. The member looks at the nearest phase ahead and the nearest phase behind and steps toward the middle of that arc (Degesys et al., 2007). `Alpha` is the fraction of the centering move taken on one tick. The runs here use `0.5`.

A negative Kuramoto `K` drives the order parameter `r` to about 0. Two clumps on opposite sides of the circle already do that, so the load stays bunched. On the 60-member row below, `K = -1` gives `r = 0` and a peak of 5, where the ideal peak is 1. Even spacing is the DESYNC rule.

Jitter and hash are initial phases, then Kuramoto with `K = 0`: free rotation at `ω₀`. They never look at a neighbor, so a hole left by a dead member stays put.

## How members interact

One tick, for every live member:

1. Collect the neighbor phases currently held. The simulator includes a neighbor once its delay has elapsed. The UDP node keeps the newest packet from each peer: a loss leaves the previous phase in place, and a peer that has never been heard stays out. An older `seq` is discarded.
2. Compute the next phase with the shared `Stepper`. The caller applies delay before the call. The core never sees a clock or an RTT.
3. Publish the new phase. A dead member does not step and is not a neighbor.

A firing is a forward crossing of 0. Arrival at the shared resource is send time plus that member's delay to the sink. The peak is the deepest overlap of the half-open intervals `[arrival, arrival+d)`.

On the wire the announcement is `{id, phase, seq, sent_at}`. `sent_at` is written so a later geo run can shift a phase by a measured RTT. The step does not use it yet. A lost packet does not stop the tick.

The simulator applies this synchronously in one process, including a per-pair delay in ticks. The UDP node steps from the last packet it has heard from each peer.

## How well it works

Task duration `d = 1`. A slot is one task long, so the slot count is `T/d`. Jitter median and p99 are 5000 seeds. Hash is one deterministic phase per member id. DESYNC is one seeded run. Aligned fires the whole fleet together, so its peak is `N`.

| N | slots T/d | ideal | aligned | jitter median | jitter p99 | hash | DESYNC |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 20 | 3600 | 1 | 20 | 1 | 2 | 1 | 1 |
| 60 | 60 | 1 | 60 | 5 | 7 | 4 | 1 |
| 100 | 60 | 2 | 100 | 6 | 9 | 5 | 2 |
| 1000 | 60 | 17 | 1000 | 30 | 36 | 27 | 17 |
| 10000 | 60 | 167 | 10000 | 207 | 225 | 199 | 183 |

The 10000-member DESYNC cell is the peak after 30000 ticks, still above 167. Every smaller row reached the ideal column. The last extra overlap is neighbor-to-neighbor mixing and takes on the order of `N²` ticks.

After 30% of 60 members are removed, DESYNC closes the live gaps: mean absolute deviation from `2π/N` falls from 0.05983 to `9.49e-5` in 836 coupling ticks, with 42 members still alive. Jitter on the same initial phases stays at 0.1012. The holes stay where the dead members were.

Regenerate the table and the figures with:

```
powershell -File experiments/run.ps1
```

Output lands in `experiments/out/` (`table.md`, `peak.svg`, `recovery.svg`).

## Where a simpler schedule is enough

Jitter matches the ideal when `N·d` is much smaller than `T`. Twenty members and 3600 slots is that row: median peak 1, p99 peak 2. A one-second job once an hour looks like this, and DESYNC has nothing to improve. Hash lands on the same peak.

Hash is the better of the two static schedules once the fleet is near capacity, and it is still short of even spacing. At 60 members and 60 slots the hash peak is 4 and the jitter median is 5, against an ideal of 1. At 100 members the hash peak is 5 and the jitter median is 6, against an ideal of 2. Neither schedule moves when membership changes.

At large `N` the relative jitter gap shrinks (about 1.24× the ideal at 10000 members) while the extra concurrent runs remain. DESYNC reaches the ideal through 1000 members. At 10000 members the step budget ran out at 183.

The rows where the spread pays for itself are the ones near capacity: the ideal is a handful of concurrent runs, and jitter or hash still pile several times that. There DESYNC matches the ideal, and after a loss it rebuilds the gaps. Jitter and hash do not.

The compromises behind that boundary are in [docs/design.md](docs/design.md): why this is not a clock, why a negative Kuramoto coupling is not an even schedule, why the peak is taken from the sink, why one fixed false phase drags the mean, and why one Kubernetes cluster does not span regions.

## Requirements

Go 1.27.

## Commands

```
go test ./...
go build -o bin/ ./cmd/sim ./cmd/node ./cmd/sink
go run ./cmd/sim -n 100 -mode desync
go run ./cmd/sink -listen :8080 -d 1
go run ./cmd/node -id a -listen 127.0.0.1:9001 -peers b=127.0.0.1:9002 -sink http://127.0.0.1:8080 -metrics :9101
go run ./cmd/node -id b -listen 127.0.0.1:9002 -peers a=127.0.0.1:9001 -sink http://127.0.0.1:8080 -metrics :9102
docker compose up
```

## Local stand

`docker compose up` builds one image and starts six members, the sink, Prometheus, and Grafana. Open http://localhost:3000. The home dashboard is the sink peak, the gaps between arrivals, and each member's phase relative to its own process clock.

Sync and desync are the same image. The stepper comes from the environment, not from a second build. `STRATEGY` defaults to `desync`. On PowerShell, `$env:STRATEGY = "kuramoto"` before `docker compose up` switches the fleet; leave `K` unset and the omitted Kuramoto coupling stays 2. `T`, `ALPHA`, and `D` are the other knobs. A local `.env` is picked up by Compose and is gitignored.

`docker compose logs node-a` is the member's slog line: id, phase, neighbor count, and whether it fired.

```mermaid
flowchart LR
  subgraph members [members]
    a[node a]
    b[node b]
    n[node n]
  end
  a -- "UDP phase" --> b
  b -- "UDP phase" --> n
  a -- fire --> sink[sink]
  b -- fire --> sink
  n -- fire --> sink
  sink --> prom[Prometheus]
  a --> prom
  prom --> graf[Grafana]
```

Compose uses a fixed peer list. The kind fleet below discovers pods through the headless Service instead. The image, the sink, and the dashboard are the same.

## Kind

No Helm. The manifests are kustomize: a StatefulSet, a headless Service named `node` with UDP port `gossip`, the sink, Prometheus, and Grafana. `deploy/overlays/desync` is the base strategy. `deploy/overlays/sync` is the same image with `PHASEFLEET_STRATEGY=kuramoto`, and an omitted `K` stays 2.

```
docker build -t phasefleet:latest .
kind load docker-image phasefleet:latest
kubectl apply -k deploy/overlays/desync
kubectl -n phasefleet rollout status statefulset/node
kubectl -n phasefleet port-forward svc/grafana 3000:3000
```

Grafana is http://localhost:3000. The sink peak is the record of the spread. Each member's id is its pod name. Peers come from SRV `_gossip._udp.node.phasefleet.svc.cluster.local`, so a pod that disappears drops out of the next lookup.

To watch a member leave and the others close up:

```
kubectl -n phasefleet delete pod node-0
```

The peak moves when that member stops firing, then DESYNC spreads the ones still up. The StatefulSet starts `node-0` again and it rejoins the DNS name. `kubectl -n phasefleet scale statefulset/node --replicas=4` is the same kind of change with a new count. Prometheus scrapes `node-0` through `node-5`, so a scale past six needs a matching target list.

Sync, on the same image:

```
kubectl apply -k deploy/overlays/sync
kubectl -n phasefleet rollout status statefulset/node
```

The dashboard phase series should pull together. Raw phase in PromQL is not the source for the order parameter.

One locked phase, on the same image: the StatefulSet steps with the circular median (`PHASEFLEET_AGGREGATE=median`, `K = 8`) and a pod named `liar` publishes `π` without listening. The honest series stay on their own rhythm. Prometheus on this overlay also scrapes `liar`. The simulator comparison, mean against median, is in [docs/design.md](docs/design.md).

```
kubectl apply -k deploy/overlays/byzantine
kubectl -n phasefleet rollout status statefulset/node
kubectl -n phasefleet rollout status deployment/liar
```

Rebuild and `kind load` the image first, so the node binary knows `-aggregate` and `-lock`. Regenerate the curves with `go run ./experiments/byzantine`.

## Layout

- `internal/core` — phase wrap, order parameter, Kuramoto, circular median, and DESYNC `Stepper`
- `internal/sim` — in-process fleet, delay, peak, recovery
- `internal/gossip` — UDP transport, `{id, phase, seq, sent_at}`, static and DNS `PeerSource`
- `internal/sink` — arrivals, peak, interval histogram
- `experiments` — the grid above, the locked-phase curves, and the script that writes the table
- `cmd/sim` — one simulated fleet
- `cmd/node` — one member: tick, UDP gossip, fire, phase offset
- `cmd/sink` — HTTP arrivals, peak, `/metrics`
- `compose.yaml` — six members, sink, Prometheus, Grafana
- `deploy/` — kustomize base, and overlays `desync`, `sync`, and `byzantine`
- `deploy/base/config/dashboards/` — the Grafana board provisioned on startup
- `docs/design.md` — the decisions behind the table
