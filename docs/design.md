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

## One cluster does not span regions

A single Kubernetes control plane across regions fails on etcd round-trip time, 50–150 ms, before this algorithm does. The geo layout is independent machines, one region each, with gossip between them and no shared control plane. Cluster mesh and a multi-cluster network are out of scope for the same reason.
