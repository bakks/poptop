package main

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"time"

	"github.com/alecthomas/kong"
	"github.com/mum4k/termdash"
	"github.com/mum4k/termdash/container"
	"github.com/mum4k/termdash/keyboard"
	"github.com/mum4k/termdash/terminal/termbox"
	"github.com/mum4k/termdash/terminal/terminalapi"
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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

// returns the first power of 2 greater than the input
func nextPower2(i int) int {
	n := 1
	for n < i {
		n = n << 1
	}
	return n
}

// Recursively creates a layout by nesting widgets into SplitHorizontals.
// We operate on a slice of Widgets (sized to be a power of two) and recursively
// call on a specific range of this slice.
// The rangeA and rangeB arguments represent the start and end of the
// area of the widget slice in which we're operating. These are split
// into chunks based on dividing powers of two.
//
// For example, if len(widgets) == 5, the following calls will be made:
//   layoutR(widgets, 0, 7)
//   layoutR(widgets, 0, 3)
//   layoutR(widgets, 0, 1)
//   layoutR(widgets, 2, 3)
//   layoutR(widgets, 4, 7)
//   layoutR(widgets, 4, 5)
func layoutR(widgets Widgets, rangeA, rangeB int) []container.Option {
	if rangeA+1 == rangeB { // if the current range is two adjacent widgets
		if rangeB >= len(widgets) { // if there's only a single widget in this range
			// then wrap that widget and return
			return widgets[rangeA]
		}

		// we have two widgets, so wrap in a SplitHorizontal and return
		return []container.Option{
			container.SplitHorizontal(
				container.Top(widgets[rangeA]...),
				container.Bottom(widgets[rangeB]...))}
	}

	// split the current range into two subranges on powers of two
	// e.g. if A = 4, B = 7, then Aa = 4, Ab = 5, Ba = 6, Bb = 7
	rangeAa := rangeA
	rangeAb := (rangeB-rangeA+1)/2 + rangeA - 1
	rangeBa := rangeAb + 1
	rangeBb := rangeB

	// call recursively on the left subrange
	widgetA := layoutR(widgets, rangeAa, rangeAb)

	// if the right subrange doesn't have any widgets, then just wrap the left and return
	if rangeBa >= len(widgets) {
		return widgetA
	}

	// call recursively on the right subrange
	widgetB := layoutR(widgets, rangeBa, rangeBb)

	// put both subranges in a SplitHorizontal and return
	return []container.Option{
		container.SplitHorizontal(
			container.Top(widgetA...),
			container.Bottom(widgetB...))}
}

// Takes an array of widgets as [][]container.Option and returns a termdash
// layout based on configuration.
func layout(widgets Widgets) ([]container.Option, error) {
	if len(widgets) == 0 {
		return nil, fmt.Errorf("No widgets initialized, failing.")
	} else if len(widgets) == 1 {
		return widgets[0], nil
	}

	// define a range starting at 0 and ending with the length of the widget
	// slice rounded up to a power of two
	rangeA := 0
	rangeB := nextPower2(len(widgets)) - 1

	// kick off the recursive engine using defined widget slice and range
	return layoutR(widgets, rangeA, rangeB), nil
}

// Execute a system command, returning a byte array of the output (both stdout and stderr)
func commandWithContext(ctx context.Context, name string, arg ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, arg...)

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Start(); err != nil {
		return buf.Bytes(), err
	}

	if err := cmd.Wait(); err != nil {
		return buf.Bytes(), err
	}

	return buf.Bytes(), nil
}

const (
	WidgetCPULoad = iota
	WidgetCPUPerc
	WidgetNetworkIO
	WidgetDiskIOPS
	WidgetDiskIO
	WidgetTopCPU
	WidgetTopMem
)

type PoptopConfig struct {
	// Which widgets (boxes of info) we want to display
	// The order here signifies the order in which widgets will be placed
	Widgets []int

	// How frequently we want to redraw the entire terminal
	RedrawInterval time.Duration

	// How frequently we want to sample (e.g. get current CPU load)
	SampleInterval time.Duration

	// How long to collect data before rolling over (i.e. width of chart x axis in time)
	ChartDuration time.Duration

	// How many samples will we retain (not set, but calculated using SampleInterval and ChartDuration
	NumSamples int

	// How many samples will be averaged into a single datapoint
	SmoothingSamples int

	// If we receive any flags for specific widgets we switch into a mode where we only show the specificed widgets
	SelectWidgetsMode bool

	// Number of lines to print in the top processes / memory lists
	TopRowsShown int
}

var cli struct {
	Help           bool `short:"h" help:"Show help information"`
	RedrawInterval int  `short:"r" help:"Redraw interval in milliseconds (how often to repaint charts)" default:"500"`
	SampleInterval int  `short:"s" help:"Sample interval in milliseconds (how often to fetch a new datapoint" default:"500"`
	ChartDuration  int  `short:"d" help:"Duration of the charted series in seconds (i.e. width of chart x-axis in time), 60 == 1 minute" default:"120"`
	Smooth         int  `short:"a" help:"How many samples will be included in running average" default:"4"`
	CpuLoad        bool `short:"L" help:"Add CPU Load chart to layout" default:"false"`
	CpuPercent     bool `short:"C" help:"Add CPU % chart to layout" default:"false"`
	DiskIops       bool `short:"D" help:"Add Disk IOPS chart to layout" default:"false"`
	DiskIo         bool `short:"E" help:"Add Disk IO chart to layout" default:"false"`
	NetworkIo      bool `short:"N" help:"Add Network IO chart to layout" default:"false"`
	TopCpu         bool `short:"T" help:"Add Top Processes by CPU list to layout" default:"false"`
	TopMemory      bool `short:"M" help:"Add Top Processes by Memory list to layout" default:"false"`
}

const description string = "A modern top command that charts system metrics like CPU load, network IO, etc in the terminal."

const helpContent string = `"What's going on with my local system?". Poptop turns your terminal into a dynamic charting tool for system metrics. While the top and htop commands show precise point-in-time data, Poptop aims to provide metrics over a time window to give a better at-a-glance summary of your system's activity. And make it look cool.

# Metrics

## CPU Load (1min, 5min, 15min)

 Charts CPU load at 1, 5, 15min averages by calling sysctl.

 Load is one of the simplest metrics for understanding how busy your system is. It means roughly how many processes are executing or waiting to execute on a CPU. If load is higher than the number of CPU cores on your system then it indicates processes are having to wait for execution.

## CPU (%) (min, avg, max)

 A chart to show min, average, max CPU busy % time. On MacOS this calls host_processor_info(). The judgement call here is that min, avg, max is a simpler way to understand CPU load rather than a single average, or charting per-CPU time.

## Network IO (KiB/s) (send, recv)

 Chart to show throughput on the selected network device in kibibytes per second using data from the netstat command. We automatically choose a network device based on which device has received the most inbound data since system start, changing dynamically if this this switches to a new device.

## Disk IOPS (read, write)

 Chart to show Disk IOPS (input/output operations per second) over time using data from iostat. Arguably, in an everyday scenario with many heavy processes then IOPS is a simpler metric than throughput, but if disk load is skewed to a specific process (e.g. heavy file copies, database operations), then disk throughput may be a better metric. This chart currently shows only a single disk.

## Disk IO (KiB/s) (read, write)

 Chart to show disk IO throughput in kibibytes per second based on iostat output. This chart currently shows only a single disk.

## Top CPU Processes (%, pid, command)

 Show a list of top CPU processes output by the ps command, i.e. which processes are consuming the most CPU. This is sampled at one-fourth of the sample interval rate since this is a point-in-time list rather than a chart. Run 'man ps' for more information on calculation methodology.

## Top Memory Processes (%, pid, command)

 Show a list of top Memory processes output by the ps command, i.e. which processes are consuming the most real memory. This is sampled at one-fourth of the sample interval rate since this is a point-in-time list rather than a chart. Run 'man ps' for more information on calculation methodology.
`

func (this *PoptopConfig) selectWidget(widget int) {
	if !this.SelectWidgetsMode {
		this.SelectWidgetsMode = true
		this.Widgets = []int{}
	}

	this.Widgets = append(this.Widgets, widget)
}

func (this *PoptopConfig) ApplyFlags() error {
	if cli.RedrawInterval < 50 {
		return fmt.Errorf("You've set the redraw interval to %dms, this is likely to stress the system so we error out for values less than 50. The redraw-interval flag is in milliseconds.\n", cli.RedrawInterval)
	}
	this.RedrawInterval = time.Duration(cli.RedrawInterval) * time.Millisecond

	if cli.SampleInterval < 20 {
		return fmt.Errorf("You've set the sample interval to %dms, this is likely to stress the system so we error out for values less than 20. The sample-interval flag is in milliseconds.\n", cli.SampleInterval)
	}
	this.SampleInterval = time.Duration(cli.SampleInterval) * time.Millisecond

	this.ChartDuration = time.Duration(cli.ChartDuration) * time.Second
	this.SmoothingSamples = cli.Smooth

	if cli.CpuLoad {
		this.selectWidget(WidgetCPULoad)
	}

	if cli.CpuPercent {
		this.selectWidget(WidgetCPUPerc)
	}

	if cli.DiskIops {
		this.selectWidget(WidgetDiskIOPS)
	}

	if cli.DiskIo {
		this.selectWidget(WidgetDiskIO)
	}

	if cli.NetworkIo {
		this.selectWidget(WidgetNetworkIO)
	}

	if cli.TopCpu {
		this.selectWidget(WidgetTopCPU)
	}

	if cli.TopMemory {
		this.selectWidget(WidgetTopMem)
	}
	return nil
}

func DefaultConfig() *PoptopConfig {
	return &PoptopConfig{
		Widgets:           []int{WidgetCPULoad, WidgetCPUPerc, WidgetNetworkIO, WidgetDiskIOPS},
		SelectWidgetsMode: false,
		TopRowsShown:      25,
	}
}

func (this *PoptopConfig) Finalize() {
	// Calculate the number of samples we'll retain by dividing the chart duration by the sampling interval
	this.NumSamples = int(math.Ceil(float64(this.ChartDuration) / float64(this.SampleInterval)))
}

func main() {
	const rootID = "root"
	var err error
	ctx, cancel := context.WithCancel(context.Background())

	config := DefaultConfig()
	kongCtx := kong.Parse(&cli, kong.Name("poptop"), kong.Description(description), kong.UsageOnError(), kong.NoDefaultHelp())

	if cli.Help {
		kong.DefaultHelpPrinter(kong.HelpOptions{}, kongCtx)
		fmt.Printf("\n\n")
		fmt.Println(helpContent)
		os.Exit(0)
	}

	err = config.ApplyFlags()

	if err != nil {
		fmt.Print(err.Error())
		os.Exit(1)
	}

	config.Finalize()

	var terminal terminalapi.Terminal

	terminal, err = termbox.New(termbox.ColorMode(terminalapi.ColorMode256))
	//t, err = tcell.New(tcell.ColorMode(terminalapi.ColorMode256))

	if err != nil {
		panic(err)
	}

	cont, err := container.New(terminal, container.ID(rootID))
	if err != nil {
		panic(err)
	}

	w, err := newWidgets(ctx, terminal, cont, config)
	if err != nil {
		panic(err)
	}

	gridOpts, err := layout(w)
	if err != nil {
		panic(err)
	}

	if err := cont.Update(rootID, gridOpts...); err != nil {
		panic(err)
	}

	quitter := func(k *terminalapi.Keyboard) {
		if k.Key == keyboard.KeyEsc || k.Key == keyboard.KeyCtrlC || k.Key == 'q' {
			cancel()
			terminal.Close()
		}
	}

	err = termdash.Run(ctx, terminal, cont, termdash.KeyboardSubscriber(quitter), termdash.RedrawInterval(config.RedrawInterval))
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
