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
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/net"
)

// redrawInterval is how often termdash redraws the screen.
const redrawInterval = 2000 * time.Millisecond
const sampleInterval = 500 * time.Millisecond
const sampleWindow = 240

// widgets holds the widgets used by this demo.
type widgets struct {
	loadChart []container.Option
	cpuChart  []container.Option
	netChart  []container.Option
	diskChart []container.Option
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

	cpuChart, err := newCpuChart(ctx)
	if err != nil {
		return nil, err
	}

	netChart, err := newNetChart(ctx)
	if err != nil {
		return nil, err
	}

	diskChart, err := newDiskChart(ctx)
	if err != nil {
		return nil, err
	}

	return &widgets{
		loadChart: loadChart,
		cpuChart:  cpuChart,
		netChart:  netChart,
		diskChart: diskChart,
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
		loadAvg, err := load.AvgWithContext(ctx)
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
		container.BorderTitle(" CPU Load (1min, 5min, 15min) "),
		container.PlaceWidget(lc)}

	return opts, nil
}

type minMax struct {
	min float64
	max float64
}

func getMinMax(list []float64) *minMax {
	min := math.MaxFloat64
	max := float64(-1)

	for _, n := range list {
		if n < min {
			min = n
		}
		if n > max {
			max = n
		}
	}

	return &minMax{
		min: min,
		max: max,
	}
}

func getAvg(list []float64) float64 {
	a := float64(0)
	for _, n := range list {
		a += n
	}

	return a / float64(len(list))
}

func color(i int) string {
	return fmt.Sprintf("\u001B[%dm", i)
}

func newCpuChart(ctx context.Context) ([]container.Option, error) {

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
	avgCpu := NewBoundedSeries(sampleWindow)
	minCpu := NewBoundedSeries(sampleWindow)
	maxCpu := NewBoundedSeries(sampleWindow)

	go periodic(ctx, redrawInterval/2, func() error {
		cpuAllPerc, err := cpu.PercentWithContext(ctx, sampleInterval, true)
		if err != nil {
			return err
		}

		minMax := getMinMax(cpuAllPerc)

		avgCpu.AddValue(getAvg(cpuAllPerc))
		minCpu.AddValue(minMax.min)
		maxCpu.AddValue(minMax.max)

		err = lc.Series("c_cpuAvg", avgCpu.Values(),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(202))),
			linechart.SeriesXLabels(xLabels),
		)
		if err != nil {
			return err
		}
		err = lc.Series("b_cpuMax", maxCpu.Values(),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(196))),
			linechart.SeriesXLabels(xLabels),
		)
		if err != nil {
			return err
		}
		err = lc.Series("a_cpuMin", minCpu.Values(),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(33))),
			linechart.SeriesXLabels(xLabels),
		)
		return err
	})

	//title := fmt.Sprintf("%sCPU (%smin%s, %savg%s, %smax%s)", color(37), color(89), color(37), color(87), color(37), color(88), color(37))

	opts := []container.Option{container.Border(linestyle.Light),
		container.BorderTitle(" CPU (min, avg, max) "),
		container.PlaceWidget(lc)}

	return opts, nil
}

func findNetworkDevice(iostat []net.IOCountersStat, device string) net.IOCountersStat {
	for _, st := range iostat {
		if st.Name == device {
			return st
		}
	}
	panic(fmt.Sprintf("Could not find device %s", device))
}

func newNetChart(ctx context.Context) ([]container.Option, error) {

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
	sent := NewBoundedSeries(sampleWindow)
	recv := NewBoundedSeries(sampleWindow)
	var lastSent uint64
	var lastRecv uint64

	go periodic(ctx, redrawInterval/2, func() error {
		iostats, err := net.IOCountersWithContext(ctx, true)
		if err != nil {
			return err
		}
		iostat := findNetworkDevice(iostats, "en0")

		newSent := iostat.BytesSent / 1024
		newRecv := iostat.BytesRecv / 1024

		if lastSent != 0 {
			sent.AddValue(float64(newSent - lastSent))
		}
		lastSent = newSent

		if lastRecv != 0 {
			recv.AddValue(float64(newRecv - lastRecv))
		}
		lastRecv = newRecv

		err = lc.Series("c_sent", sent.Values(),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(33))),
			linechart.SeriesXLabels(xLabels),
		)
		if err != nil {
			return err
		}
		err = lc.Series("b_recv", recv.Values(),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(196))),
			linechart.SeriesXLabels(xLabels),
		)
		return err
	})

	opts := []container.Option{container.Border(linestyle.Light),
		container.BorderTitle(" Network IO (send, recv) "),
		container.PlaceWidget(lc)}

	return opts, nil
}

func newDiskChart(ctx context.Context) ([]container.Option, error) {

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
	write := NewBoundedSeries(sampleWindow)
	read := NewBoundedSeries(sampleWindow)
	var lastWrite uint64
	var lastRead uint64

	go periodic(ctx, redrawInterval/2, func() error {
		iostats, err := disk.IOCountersWithContext(ctx)
		if err != nil {
			return err
		}

		var iostat disk.IOCountersStat
		for _, v := range iostats {
			iostat = v
		}

		newWrite := iostat.ReadBytes / 1024
		newRead := iostat.WriteBytes / 1024

		if lastWrite != 0 {
			write.AddValue(float64(newWrite - lastWrite))
		}
		lastWrite = newWrite

		if lastRead != 0 {
			read.AddValue(float64(newRead - lastRead))
		}
		lastRead = newRead

		err = lc.Series("c_write", write.Values(),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(33))),
			linechart.SeriesXLabels(xLabels),
		)
		if err != nil {
			return err
		}
		err = lc.Series("b_read", read.Values(),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(196))),
			linechart.SeriesXLabels(xLabels),
		)
		return err
	})

	opts := []container.Option{container.Border(linestyle.Light),
		container.BorderTitle(" Disk IO (read, write) "),
		container.PlaceWidget(lc)}

	return opts, nil
}

// layout prepares container options that represent the desired screen layout.
// This function demonstrates the use of the grid builder.
// layout() and contLayout() demonstrate the two available layout APIs and
// both produce equivalent layouts for layoutType layoutAll.
func layout(w *widgets) ([]container.Option, error) {
	opts := []container.Option{
		container.SplitHorizontal(
			container.Top(
				container.SplitHorizontal(
					container.Top(w.loadChart...),
					container.Bottom(w.cpuChart...),
				),
			),
			container.Bottom(
				container.SplitHorizontal(
					container.Top(w.netChart...),
					container.Bottom(w.diskChart...),
				),
			),
		),
	}
	return opts, nil
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
