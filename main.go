package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"math"

	"github.com/mum4k/termdash"
	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/container"
	"github.com/mum4k/termdash/keyboard"
	"github.com/mum4k/termdash/linestyle"
	"github.com/mum4k/termdash/terminal/tcell"
	"github.com/mum4k/termdash/terminal/termbox"
	"github.com/mum4k/termdash/terminal/terminalapi"
	"github.com/mum4k/termdash/widgets/linechart"
	"github.com/shirou/gopsutil/load"
)

// redrawInterval is how often termdash redraws the screen.
const redrawInterval = 2000 * time.Millisecond
const sampleInterval = 500 * time.Millisecond
const sampleWindow = 240

// widgets holds the widgets used by this demo.
type widgets struct {
	loadChart []container.Option
}

type BoundedSeries struct {
	values    []float64
	maxValues int
	highWater int
}

func NewBoundedSeries(maxValues int) *BoundedSeries {
	values := make([]float64, maxValues)
	for i := 0; i < maxValues; i++ {
		values[i] = math.NaN()
	}
	return &BoundedSeries{
		values:    values,
		maxValues: maxValues,
		highWater: 0,
	}
}

func (this *BoundedSeries) AddValue(v float64) {
	if this.highWater < this.maxValues {
		this.values[this.highWater] = v
		this.highWater++
	} else {
		newValues := append(this.values, v)
		if len(newValues) > this.maxValues {
			newValues = newValues[len(newValues)-this.maxValues:]
		}
		this.values = newValues
	}
}

func (this *BoundedSeries) Values() []float64 {
	return this.values
}

// newWidgets creates all widgets used by this demo.
func newWidgets(ctx context.Context, t terminalapi.Terminal, c *container.Container) (*widgets, error) {
	loadChart, err := newLoadChart(ctx)
	if err != nil {
		return nil, err
	}

	return &widgets{
		loadChart: loadChart,
	}, nil
}

func formatLabels(xIndexToLabel func(n int) string) map[int]string {
	labels := map[int]string{}

	for i := 0; i < sampleWindow; i++ {
		labels[i] = xIndexToLabel(i)
	}

	return labels
}

func newLoadChart(ctx context.Context) ([]container.Option, error) {

	xLabels := formatLabels(func(n int) string {
		x := float64(n) * float64(sampleInterval) / float64(time.Second)
		return fmt.Sprintf("%.0fs", x)
	})

	lc, err := linechart.New(
		linechart.AxesCellOpts(cell.FgColor(cell.ColorRed)),
		linechart.YLabelCellOpts(cell.FgColor(cell.ColorGreen)),
	)
	if err != nil {
		return nil, err
	}
	load1 := NewBoundedSeries(sampleWindow)
	load5 := NewBoundedSeries(sampleWindow)
	load15 := NewBoundedSeries(sampleWindow)

	go periodic(ctx, redrawInterval/2, func() error {
		loadAvg, err := load.Avg()
		if err != nil {
			return err
		}

		load1.AddValue(loadAvg.Load1)
		load5.AddValue(loadAvg.Load5)
		load15.AddValue(loadAvg.Load15)

		err = lc.Series("c_load1", load1.Values(),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(87))),
		)
		if err != nil {
			return err
		}
		err = lc.Series("b_load5", load5.Values(),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(93))),
		)
		if err != nil {
			return err
		}
		err = lc.Series("a_load15", load15.Values(),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(124))),
			linechart.SeriesXLabels(xLabels),
		)
		return err
	})

	opts := []container.Option{container.Border(linestyle.Light),
		container.BorderTitle("CPU Load (1min, 5min, 15min)"),
		container.PlaceWidget(lc)}

	return opts, nil
}

// layout prepares container options that represent the desired screen layout.
// This function demonstrates the use of the grid builder.
// layout() and contLayout() demonstrate the two available layout APIs and
// both produce equivalent layouts for layoutType layoutAll.
func layout(w *widgets) ([]container.Option, error) {
	return w.loadChart, nil
}

// rootID is the ID assigned to the root container.
const rootID = "root"

// Terminal implementations
const (
	termboxTerminal = "termbox"
	tcellTerminal   = "tcell"
)

func main() {

	terminalPtr := flag.String("terminal",
		"tcell",
		"The terminal implementation to use. Available implementations are 'termbox' and 'tcell' (default = tcell).")
	flag.Parse()

	var t terminalapi.Terminal
	var err error

	switch terminal := *terminalPtr; terminal {
	case termboxTerminal:
		t, err = termbox.New(termbox.ColorMode(terminalapi.ColorMode256))
	case tcellTerminal:
		t, err = tcell.New(tcell.ColorMode(terminalapi.ColorMode256))
	default:
		log.Fatalf("Unknown terminal implementation '%s' specified. Please choose between 'termbox' and 'tcell'.", terminal)
		return
	}

	if err != nil {
		panic(err)
	}
	defer t.Close()

	cont, err := container.New(t, container.ID(rootID))
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	w, err := newWidgets(ctx, t, cont)
	if err != nil {
		panic(err)
	}

	gridOpts, err := layout(w) // equivalent to contLayout(w)
	if err != nil {
		panic(err)
	}

	if err := cont.Update(rootID, gridOpts...); err != nil {
		panic(err)
	}

	quitter := func(k *terminalapi.Keyboard) {
		if k.Key == keyboard.KeyEsc || k.Key == keyboard.KeyCtrlC || k.String() == "q" {
			cancel()
		}
	}

	err = termdash.Run(ctx, t, cont, termdash.KeyboardSubscriber(quitter), termdash.RedrawInterval(redrawInterval))
	if err != nil {
		panic(err)
	}
}

// periodic executes the provided closure periodically every interval.
// Exits when the context expires.
func periodic(ctx context.Context, interval time.Duration, fn func() error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := fn(); err != nil {
				panic(err)
			}
		case <-ctx.Done():
			return
		}
	}
}
