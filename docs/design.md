# Design decisions

phasefleet coordinates when fleet members run a periodic job. It does not replace a clock. Wall time stays with chrony, NTP, PTP, or a cloud time source. The step never reads that clock. A later measurement of `θ − ω₀·t` does, and only so a graph can be read.

## Even spacing is DESYNC

Kuramoto with `K > 0` pulls phases together. A negative `K` drives the order parameter `r` toward 0. Two clumps on opposite sides of the circle already produce that `r`, and the load stays bunched. On the 60-member row of the simulator, `K = -1` gives `r = 0` and a peak of 5 where the ideal peak is 1.

Uniform spacing is DESYNC: each member steps toward the middle of the arc between the neighbor ahead and the neighbor behind. The runs use alpha `0.5`.

## The peak comes from the sink

Prometheus scrapes members at different moments, and the phase keeps rotating. A raw phase series, and an `r` computed from those scrapes, looks disordered even when the fleet is locked. The sink records arrivals on its own clock. Current concurrency, peak, and the gap histogram do not depend on when a scrape happened. The phase series on the dashboard is `θ − ω₀·t` since the process started, wrapped onto `[0, 2π)`, and it is not a source for `r`.

## Where jitter is enough

`N` members, task duration `d`, period `T`. The ideal peak is `⌈N·d/T⌉`.

When `N·d` is much smaller than `T`, a one-shot jitter draw already matches that ideal. A one-second job once an hour is this case, and DESYNC has nothing to add. Hash, a deterministic phase from the member id, lands in the same place and does not move when members die.

DESYNC earns its keep near `N ≈ T/d`, where the ideal is a handful of overlapping runs and jitter or hash still stack several times that. After members disappear, DESYNC closes the gaps. Jitter and hash leave the holes where the dead members were.

## A fixed false phase

A silent member is absent from the neighbor list. DESYNC rearranges the members still heard. There is no observation to reject, and the median is not part of that path.

A member that stays up, keeps publishing one phase, and ignores everyone else is a false observation. The Kuramoto mean counts it as one more term. Honest members that start together stay together, and the coupling drags that clump toward the fixed phase. The fixed phase does not rotate, so a large enough `K` cancels `ω₀` and the clump stops. Replacing the mean with the circular median leaves the clump on its own rhythm: one phase cannot move the median while the others outnumber it.

Seed 1, eleven members, one of them held at `π`, `K = 200`, `T = 1`. Under the mean the honest clump finishes 0.3196 radians from `π`, order `r = 1`, and advances 0 over the last period. Under the median the same clump keeps `r = 1` and advances 6.2832, a full turn. The curves are `experiments/out/byzantine.svg`, from `go run ./experiments/byzantine`.

`PHASEFLEET_AGGREGATE=median` selects that step on the node. `deploy/overlays/byzantine` runs it at `K = 8` and `T = 10`, with an extra pod locked at `π`. Six neighbors put the mean's entrainment threshold near 3.8, so this `K` is on the side where the mean would follow the locked pod. The StatefulSet uses the median.

A member that reports a different phase to each peer is outside this run. The median is one aggregator, not a bound on every byzantine fault. Packet signatures are a separate check. The W-MSR family is not implemented.

## One cluster does not span regions

A single Kubernetes control plane across regions fails on etcd round-trip time, 50–150 ms, before this algorithm does. The geo layout is independent machines, one region each, with gossip between them and no shared control plane. Cluster mesh and a multi-cluster network are out of scope for the same reason.
