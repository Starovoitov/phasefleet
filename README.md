# phasefleet

Coordinator-free phase coordination for service fleets — sync and anti-sync (thundering-herd avoidance) via coupled oscillators.

`internal/core` has no clock and no network. The simulator and a real node call the same step.

## Requirements

Go 1.27.

## Commands

```
go test ./...
go build -o bin/ ./cmd/sim ./cmd/node ./cmd/sink
go run ./cmd/sim -n 100 -mode desync
go run ./cmd/node -listen :9000
go run ./cmd/sink -listen :8080
```

## Layout

- `internal/core` — phase wrap, order parameter, `Stepper`
- `internal/gossip` — `{id, phase, seq, sent_at}`, `PeerSource`, `Transport`
- `cmd/sim` — in-process fleet
- `cmd/node` — one UDP member
- `cmd/sink` — firing log and peak concurrency
