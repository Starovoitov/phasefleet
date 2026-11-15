package sim

import (
	"encoding/csv"
	"io"
	"strconv"
)

// WriteCSV writes one row per alive node per tick.
func WriteCSV(w io.Writer, cfg Config, samples []Sample) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"strategy", "N", "d", "T", "tick", "id", "phase", "send", "arrival", "alive"}); err != nil {
		return err
	}
	n := strconv.Itoa(cfg.N)
	d := strconv.FormatFloat(cfg.D, 'g', -1, 64)
	period := strconv.FormatFloat(cfg.T, 'g', -1, 64)
	for _, s := range samples {
		send, arrival := "", ""
		if s.Fired {
			send = strconv.FormatFloat(s.Send, 'g', -1, 64)
			arrival = strconv.FormatFloat(s.Arrival, 'g', -1, 64)
		}
		err := cw.Write([]string{
			cfg.Strategy,
			n,
			d,
			period,
			strconv.Itoa(s.Tick),
			s.ID,
			strconv.FormatFloat(s.Phase, 'g', -1, 64),
			send,
			arrival,
			"true",
		})
		if err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
