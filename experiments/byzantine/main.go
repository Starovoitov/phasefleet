// Command byzantine writes the mean-versus-median curves for one locked phase.
// The seed matches internal/sim.Byzantine.
package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Starovoitov/phasefleet/internal/core"
	"github.com/Starovoitov/phasefleet/internal/sim"
)

func main() {
	out := "experiments/out"
	if len(os.Args) == 3 && os.Args[1] == "-out" {
		out = os.Args[2]
	} else if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: byzantine [-out dir]")
		os.Exit(2)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	const seed = 1
	mean, median, err := sim.Byzantine(seed)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	path := filepath.Join(out, "byzantine.csv")
	if err := writeCSV(path, mean, median); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	svgPath := filepath.Join(out, "byzantine.svg")
	if err := writeSVG(svgPath, mean, median); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	from := len(mean) / 2
	fmt.Printf("seed %d\n", seed)
	fmt.Printf("mean   final dist %.4f  last-period advance %.4f  r %.4f\n",
		mean[len(mean)-1].Dist, advance(mean[from:]), mean[len(mean)-1].R)
	fmt.Printf("median final dist %.4f  last-period advance %.4f  r %.4f\n",
		median[len(median)-1].Dist, advance(median[from:]), median[len(median)-1].R)
	fmt.Printf("wrote %s and %s\n", path, svgPath)
}

func writeCSV(path string, mean, median []sim.ByzantineSample) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		cerr := f.Close()
		if err == nil {
			err = cerr
		}
	}()
	w := csv.NewWriter(f)
	if err := w.Write([]string{"aggregate", "tick", "dist", "r", "psi"}); err != nil {
		return err
	}
	for _, series := range []struct {
		name string
		rows []sim.ByzantineSample
	}{
		{"mean", mean},
		{"median", median},
	} {
		for _, s := range series.rows {
			if err := w.Write([]string{
				series.name,
				strconv.Itoa(s.Tick),
				strconv.FormatFloat(s.Dist, 'g', 8, 64),
				strconv.FormatFloat(s.R, 'g', 8, 64),
				strconv.FormatFloat(s.Psi, 'g', 8, 64),
			}); err != nil {
				return err
			}
		}
	}
	w.Flush()
	return w.Error()
}

func writeSVG(path string, mean, median []sim.ByzantineSample) error {
	const (
		width  = 640
		height = 320
		left   = 48
		right  = 16
		top    = 36
		bottom = 32
	)
	plotW := float64(width - left - right)
	plotH := float64(height - top - bottom)
	ticks := float64(len(mean) - 1)
	yOf := func(dist float64) float64 {
		return float64(top) + plotH*(1-dist/math.Pi)
	}
	xOf := func(tick int) float64 {
		return float64(left) + plotW*float64(tick)/ticks
	}
	body := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">
<rect width="100%%" height="100%%" fill="#fafafa"/>
<text x="%d" y="22" font-family="sans-serif" font-size="14">distance from the honest mean to the locked phase</text>
<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#ccc"/>
<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#ccc"/>
<text x="%d" y="%d" font-family="sans-serif" font-size="11" fill="#666">π</text>
<text x="%d" y="%d" font-family="sans-serif" font-size="11" fill="#666">0</text>
<polyline fill="none" stroke="#c45c26" stroke-width="1.5" points="%s"/>
<polyline fill="none" stroke="#2f6f4e" stroke-width="1.5" points="%s"/>
<text x="%d" y="%d" font-family="sans-serif" font-size="12" fill="#c45c26">mean</text>
<text x="%d" y="%d" font-family="sans-serif" font-size="12" fill="#2f6f4e">median</text>
</svg>
`,
		width, height, width, height,
		left,
		left, top, left, height-bottom,
		left, height-bottom, width-right, height-bottom,
		8, top+10,
		12, height-bottom,
		points(mean, xOf, yOf),
		points(median, xOf, yOf),
		left, height-8,
		left+48, height-8,
	)
	return os.WriteFile(path, []byte(body), 0o644)
}

func points(series []sim.ByzantineSample, xOf func(int) float64, yOf func(float64) float64) string {
	b := make([]byte, 0, len(series)*16)
	for i, s := range series {
		if i > 0 {
			b = append(b, ' ')
		}
		b = append(b, strconv.FormatFloat(xOf(s.Tick), 'f', 1, 64)...)
		b = append(b, ',')
		b = append(b, strconv.FormatFloat(yOf(s.Dist), 'f', 1, 64)...)
	}
	return string(b)
}

func advance(samples []sim.ByzantineSample) float64 {
	if len(samples) < 2 {
		return 0
	}
	var adv float64
	prev := samples[0].Psi
	for _, s := range samples[1:] {
		adv += core.Delta(core.Phase(prev), core.Phase(s.Psi))
		prev = s.Psi
	}
	return adv
}
