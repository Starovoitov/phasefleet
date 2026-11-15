# Jitter, hash, and DESYNC

Task duration d = 1. Slots are T/d. Ideal is ceil(N·d/T).
Jitter median and p99 are 5000 seeds. Hash is one deterministic assignment from the node id.
DESYNC is one seeded run. Aligned fires every node together, so its peak is N.

| N | slots T/d | ideal | aligned | jitter median | jitter p99 | hash | DESYNC | where this row sits |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 20 | 3600 | 1 | 20 | 1 | 2 | 1 | 1 | jitter is enough: N·d is much smaller than T |
| 60 | 60 | 1 | 60 | 5 | 7 | 4 | 1 | near capacity: jitter peak is several times the ideal |
| 100 | 60 | 2 | 100 | 6 | 9 | 5 | 2 | near capacity: jitter peak is several times the ideal |
| 1000 | 60 | 17 | 1000 | 30 | 36 | 27 | 17 | large N: relative peak is 1.76× ideal, the absolute gap remains |
| 10000 | 60 | 167 | 10000 | 207 | 225 | 199 | 183 after 30000 ticks, still above the ideal | large N: relative peak is 1.24× ideal, the absolute gap remains |

Where DESYNC is marked as still above the ideal, that is the peak after the step budget, not the equilibrium. The same stepper reached the ideal column on every smaller row. The last extra overlap disappears by neighbor-to-neighbor mixing, which takes on the order of N² ticks.

## After killing 30% of 60 nodes

DESYNC live-gap MAD starts at 0.05983 and reaches 9.49e-05 after 836 coupling ticks (42 nodes still alive).
Jitter on the same initial phases stays at MAD 0.1012. The holes left by the dead nodes do not close.

## Negative Kuramoto coupling

N=60, slots=60, K=-1. Order parameter r=0.000, peak=5, ideal=1. r near 0 is not an even schedule.

