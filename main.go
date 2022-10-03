package main

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/mum4k/termdash"
	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/container"
	"github.com/mum4k/termdash/keyboard"
	"github.com/mum4k/termdash/linestyle"
	"github.com/mum4k/termdash/terminal/termbox"
	"github.com/mum4k/termdash/terminal/terminalapi"
	"github.com/mum4k/termdash/widgetapi"
	"github.com/mum4k/termdash/widgets/linechart"
	"github.com/mum4k/termdash/widgets/text"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/net"
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

type Widgets [][]container.Option

// newWidgets initializes widgets in the configured order and passes them back as []container.Option`s
func newWidgets(ctx context.Context, t terminalapi.Terminal, c *container.Container, config *PoptopConfig) (Widgets, error) {
	var topCpu []container.Option
	var topMem []container.Option
	var err error
	widgets := [][]container.Option{}

	for _, widgetRef := range config.Widgets {
		var newWidget []container.Option

		switch widgetRef {
		case WidgetCPULoad:
			newWidget, err = newLoadChart(ctx, config)

		case WidgetCPUPerc:
			newWidget, err = newCpuChart(ctx, config)

		case WidgetNetworkIO:
			newWidget, err = newNetChart(ctx, config)

		case WidgetDiskIOPS:
			newWidget, err = newDiskIOPSChart(ctx, config)

		case WidgetDiskIO:
			newWidget, err = newDiskIOChart(ctx, config)

		case WidgetTopCPU:
			if topCpu == nil {
				topCpu, topMem, err = newTopBoxes(ctx, config)
			}
			newWidget = topCpu

		case WidgetTopMem:
			if topCpu == nil {
				topCpu, topMem, err = newTopBoxes(ctx, config)
			}
			newWidget = topMem
		}

		if err != nil {
			return nil, err
		}
		if newWidget == nil {
			panic(fmt.Sprintf("Failed to initialize widget %d", widgetRef))
		}
		widgets = append(widgets, newWidget)
	}

	return widgets, nil
}

func formatLabels(config *PoptopConfig, xIndexToLabel func(n int) string) map[int]string {
	labels := map[int]string{}

	for i := 0; i < config.NumSamples; i++ {
		labels[i] = xIndexToLabel(i)
	}

	return labels
}

// Initializes both a top CPU and top memory box
// We do these together because they depend on the same call to `ps`
func newTopBoxes(ctx context.Context, config *PoptopConfig) ([]container.Option, []container.Option, error) {
	cpuTextBox, err := text.New()
	if err != nil {
		return nil, nil, err
	}
	memTextBox, err := text.New()
	if err != nil {
		return nil, nil, err
	}

	go periodic(ctx, config.SampleInterval, func() error {
		topCpu, topMem := topProcesses(ctx)
		if err != nil {
			return err
		}

		lines := []string{}
		for _, proc := range topCpu {
			lineItem := fmt.Sprintf("%3.0f%%  %-5d  %s\n", proc.CpuPerc, proc.Pid, proc.Command)
			lines = append(lines, lineItem)
		}

		fullText := strings.Join(lines, "")
		cpuTextBox.Write(fullText, text.WriteReplace())

		lines = []string{}
		for _, proc := range topMem {
			lineItem := fmt.Sprintf("%3.0f%%  %-5d  %s\n", proc.MemPerc, proc.Pid, proc.Command)
			lines = append(lines, lineItem)
		}

		fullText = strings.Join(lines, "")
		memTextBox.Write(fullText, text.WriteReplace())

		return nil
	})

	cpuTitle := cell.NewRichTextString(cell.ColorWhite).
		AddText(" Top CPU Processes (%, pid, command) ")

	memTitle := cell.NewRichTextString(cell.ColorWhite).
		AddText(" Top Memory Processes (%, pid, command) ")

	cpuOpts := makeContainer(cpuTextBox, cpuTitle)
	memOpts := makeContainer(memTextBox, memTitle)

	return cpuOpts, memOpts, nil
}

func formatOnePoint(n float64) string {
	return fmt.Sprintf("%.1f", n)
}

func formatNoPoint(n float64) string {
	return fmt.Sprintf("%.0f", n)
}

func formatPercent(n float64) string {
	return fmt.Sprintf("%.0f%%", n)
}

func newLinechart(opts ...linechart.Option) (*linechart.LineChart, error) {
	defaultOpts := []linechart.Option{
		linechart.AxesCellOpts(cell.FgColor(cell.ColorGray)),
		linechart.YLabelCellOpts(cell.FgColor(cell.ColorSilver)),
		linechart.XLabelCellOpts(cell.FgColor(cell.ColorSilver)),
	}
	mergedOpts := append(defaultOpts, opts...)

	return linechart.New(mergedOpts...)
}

func makeContainer(widget widgetapi.Widget, title *cell.RichTextString) []container.Option {
	return []container.Option{container.Border(linestyle.Round),
		container.BorderColor(cell.ColorGray),
		container.FocusedColor(cell.ColorGray),
		container.TitleColor(cell.ColorWhite),
		container.TitleFocusedColor(cell.ColorWhite),
		container.RichBorderTitle(title),
		container.PlaceWidget(widget)}
}

func newLoadChart(ctx context.Context, config *PoptopConfig) ([]container.Option, error) {
	xLabels := formatLabels(config, func(n int) string {
		x := float64(n) * float64(config.SampleInterval) / float64(time.Second)
		return fmt.Sprintf("%.0fs", x)
	})

	lc, err := newLinechart(linechart.YAxisFormattedValues(formatOnePoint))
	if err != nil {
		return nil, err
	}

	nSamples := config.NumSamples
	load1 := NewBoundedSeries(nSamples)
	load5 := NewBoundedSeries(nSamples)
	load15 := NewBoundedSeries(nSamples)

	go periodic(ctx, config.SampleInterval, func() error {
		loadAvg, err := load.AvgWithContext(ctx)
		if err != nil {
			return err
		}

		load1.AddValue(loadAvg.Load1)
		load5.AddValue(loadAvg.Load5)
		load15.AddValue(loadAvg.Load15)

		err = lc.Series("c_load1", load1.SmoothedValues(config.SmoothingSamples),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(87))),
		)
		if err != nil {
			return err
		}
		err = lc.Series("b_load5", load5.SmoothedValues(config.SmoothingSamples),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(93))),
		)
		if err != nil {
			return err
		}
		err = lc.Series("a_load15", load15.SmoothedValues(config.SmoothingSamples),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(124))),
			linechart.SeriesXLabels(xLabels),
		)
		return err
	})

	title := cell.NewRichTextString(cell.ColorWhite).
		AddText(" CPU Load (").
		SetFgColor(cell.ColorNumber(87)).
		AddText("1min").
		ResetColor().
		AddText(", ").
		SetFgColor(cell.ColorNumber(93)).
		AddText("5min").
		ResetColor().
		AddText(", ").
		SetFgColor(cell.ColorNumber(124)).
		AddText("15min").
		ResetColor().
		AddText(") ")

	opts := makeContainer(lc, title)

	return opts, nil
}

func newCpuChart(ctx context.Context, config *PoptopConfig) ([]container.Option, error) {

	xLabels := formatLabels(config, func(n int) string {
		x := float64(n) * float64(config.SampleInterval) / float64(time.Second)
		return fmt.Sprintf("%.0fs", x)
	})

	lc, err := newLinechart(linechart.YAxisFormattedValues(formatPercent))
	if err != nil {
		return nil, err
	}

	nSamples := config.NumSamples
	avgCpu := NewBoundedSeries(nSamples)
	minCpu := NewBoundedSeries(nSamples)
	maxCpu := NewBoundedSeries(nSamples)

	go periodic(ctx, config.SampleInterval, func() error {
		cpuAllPerc, err := cpu.PercentWithContext(ctx, config.SampleInterval, true)
		if err != nil {
			return err
		}

		minMax := getMinMax(cpuAllPerc)

		avgCpu.AddValue(getAvg(cpuAllPerc))
		minCpu.AddValue(minMax.min)
		maxCpu.AddValue(minMax.max)

		err = lc.Series("c_cpuAvg", avgCpu.SmoothedValues(config.SmoothingSamples),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(202))),
			linechart.SeriesXLabels(xLabels),
		)
		if err != nil {
			return err
		}
		err = lc.Series("b_cpuMax", maxCpu.SmoothedValues(config.SmoothingSamples),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(196))),
			linechart.SeriesXLabels(xLabels),
		)
		if err != nil {
			return err
		}
		err = lc.Series("a_cpuMin", minCpu.SmoothedValues(config.SmoothingSamples),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(33))),
			linechart.SeriesXLabels(xLabels),
		)
		return err
	})

	title := cell.NewRichTextString(cell.ColorWhite).
		AddText(" CPU (%) (").
		SetFgColor(cell.ColorNumber(33)).
		AddText("min").
		ResetColor().
		AddText(", ").
		SetFgColor(cell.ColorNumber(202)).
		AddText("avg").
		ResetColor().
		AddText(", ").
		SetFgColor(cell.ColorNumber(196)).
		AddText("max").
		ResetColor().
		AddText(") ")

	opts := makeContainer(lc, title)

	return opts, nil
}

// Identify the network device with the most inbound data (since startup) based on iostat output
func findNetworkDevice(iostat []net.IOCountersStat) net.IOCountersStat {
	var maxBytesReceived uint64 = 0
	var stat net.IOCountersStat

	for _, st := range iostat {
		if st.BytesRecv > maxBytesReceived {
			maxBytesReceived = st.BytesRecv
			stat = st
		}
	}

	if maxBytesReceived == 0 {
		panic(fmt.Sprintf("Could not find network device"))
	}

	return stat
}

func newNetChart(ctx context.Context, config *PoptopConfig) ([]container.Option, error) {

	xLabels := formatLabels(config, func(n int) string {
		x := float64(n) * float64(config.SampleInterval) / float64(time.Second)
		return fmt.Sprintf("%.0fs", x)
	})

	lc, err := newLinechart(linechart.YAxisFormattedValues(formatNoPoint))
	if err != nil {
		return nil, err
	}

	device := ""
	var sent *BoundedSeries
	var recv *BoundedSeries
	var lastSent uint64
	var lastRecv uint64

	go periodic(ctx, config.SampleInterval, func() error {
		iostats, err := net.IOCountersWithContext(ctx, true)
		if err != nil {
			return err
		}
		iostat := findNetworkDevice(iostats)

		if iostat.Name != device {
			// This happens at startup OR if a different network device has become the most active
			nSamples := config.NumSamples
			sent = NewBoundedSeries(nSamples)
			recv = NewBoundedSeries(nSamples)
			lastSent = 0
			lastRecv = 0
			device = iostat.Name
		}

		newSent := iostat.BytesSent * uint64(time.Second/config.SampleInterval) / 1024
		newRecv := iostat.BytesRecv * uint64(time.Second/config.SampleInterval) / 1024

		if lastSent != 0 {
			sent.AddValue(float64(newSent - lastSent))
		}
		lastSent = newSent

		if lastRecv != 0 {
			recv.AddValue(float64(newRecv - lastRecv))
		}
		lastRecv = newRecv

		err = lc.Series("c_sent", sent.SmoothedValues(config.SmoothingSamples),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(196))),
			linechart.SeriesXLabels(xLabels),
		)
		if err != nil {
			return err
		}
		err = lc.Series("b_recv", recv.SmoothedValues(config.SmoothingSamples),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(33))),
			linechart.SeriesXLabels(xLabels),
		)
		return err
	})

	title := cell.NewRichTextString(cell.ColorWhite).
		AddText(" Network IO (KiB/s) (").
		SetFgColor(cell.ColorNumber(196)).
		AddText("send").
		ResetColor().
		AddText(", ").
		SetFgColor(cell.ColorNumber(33)).
		AddText("recv").
		ResetColor().
		AddText(") ")

	opts := makeContainer(lc, title)

	return opts, nil
}

func newDiskIOPSChart(ctx context.Context, config *PoptopConfig) ([]container.Option, error) {
	xLabels := formatLabels(config, func(n int) string {
		x := float64(n) * float64(config.SampleInterval) / float64(time.Second)
		return fmt.Sprintf("%.0fs", x)
	})

	lc, err := newLinechart(linechart.YAxisFormattedValues(formatNoPoint))
	if err != nil {
		return nil, err
	}
	write := NewBoundedSeries(config.NumSamples)
	read := NewBoundedSeries(config.NumSamples)
	var lastWrite uint64
	var lastRead uint64

	go periodic(ctx, config.SampleInterval, func() error {
		iostats, err := disk.IOCountersWithContext(ctx)
		if err != nil {
			return err
		}

		var iostat disk.IOCountersStat
		for _, v := range iostats {
			iostat = v
		}

		newRead := iostat.ReadCount
		newWrite := iostat.WriteCount

		if lastWrite != 0 {
			write.AddValue(float64(newWrite-lastWrite) * float64(time.Second/config.SampleInterval))
		}
		lastWrite = newWrite

		if lastRead != 0 {
			read.AddValue(float64(newRead-lastRead) * float64(time.Second/config.SampleInterval))
		}
		lastRead = newRead

		err = lc.Series("c_read", read.SmoothedValues(config.SmoothingSamples),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(196))),
			linechart.SeriesXLabels(xLabels),
		)
		if err != nil {
			return err
		}
		err = lc.Series("b_write", write.SmoothedValues(config.SmoothingSamples),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(33))),
			linechart.SeriesXLabels(xLabels),
		)
		return err
	})

	title := cell.NewRichTextString(cell.ColorWhite).
		AddText(" Disk IOPS (").
		SetFgColor(cell.ColorNumber(33)).
		AddText("read").
		ResetColor().
		AddText(", ").
		SetFgColor(cell.ColorNumber(196)).
		AddText("write").
		ResetColor().
		AddText(") ")

	opts := makeContainer(lc, title)

	return opts, nil
}

func newDiskIOChart(ctx context.Context, config *PoptopConfig) ([]container.Option, error) {
	xLabels := formatLabels(config, func(n int) string {
		x := float64(n) * float64(config.SampleInterval) / float64(time.Second)
		return fmt.Sprintf("%.0fs", x)
	})

	lc, err := newLinechart(linechart.YAxisFormattedValues(formatNoPoint))
	if err != nil {
		return nil, err
	}
	write := NewBoundedSeries(config.NumSamples)
	read := NewBoundedSeries(config.NumSamples)
	var lastWrite uint64
	var lastRead uint64

	go periodic(ctx, config.SampleInterval, func() error {
		iostats, err := disk.IOCountersWithContext(ctx)
		if err != nil {
			return err
		}

		var iostat disk.IOCountersStat
		for _, v := range iostats {
			iostat = v
		}

		newRead := iostat.ReadBytes * uint64(time.Second/config.SampleInterval) / 1024
		newWrite := iostat.WriteBytes * uint64(time.Second/config.SampleInterval) / 1024

		if lastWrite != 0 {
			write.AddValue(float64(newWrite - lastWrite))
		}
		lastWrite = newWrite

		if lastRead != 0 {
			read.AddValue(float64(newRead - lastRead))
		}
		lastRead = newRead

		err = lc.Series("c_write", write.SmoothedValues(config.SmoothingSamples),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(196))),
			linechart.SeriesXLabels(xLabels),
		)
		if err != nil {
			return err
		}
		err = lc.Series("b_read", read.SmoothedValues(config.SmoothingSamples),
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(33))),
			linechart.SeriesXLabels(xLabels),
		)
		return err
	})

	title := cell.NewRichTextString(cell.ColorWhite).
		AddText(" Disk IO (KiB/s) (").
		SetFgColor(cell.ColorNumber(33)).
		AddText("read").
		ResetColor().
		AddText(", ").
		SetFgColor(cell.ColorNumber(196)).
		AddText("write").
		ResetColor().
		AddText(") ")

	opts := makeContainer(lc, title)

	return opts, nil
}

// returns the first power of 2 greater than the input
func nextPower2(i int) int {
	n := 1
	for n < i {
		n = n << 1
	}
	return n
}

// Recursively creates a layout by nesting widgets into SplitHorizontals
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
	if rangeA+1 == rangeB {
		if rangeB >= len(widgets) {
			return widgets[rangeA]
		}

		return []container.Option{
			container.SplitHorizontal(
				container.Top(widgets[rangeA]...),
				container.Bottom(widgets[rangeB]...))}
	}

	rangeAa := rangeA
	rangeAb := (rangeB-rangeA+1)/2 + rangeA - 1
	rangeBa := rangeAb + 1
	rangeBb := rangeB

	widgetA := layoutR(widgets, rangeAa, rangeAb)
	if rangeBa >= len(widgets) {
		return widgetA
	}

	widgetB := layoutR(widgets, rangeBa, rangeBb)
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
	return layoutR(widgets, 0, nextPower2(len(widgets))-1), nil
}

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

type PsProcess struct {
	User    string
	Pid     int
	CpuPerc float64
	MemPerc float64
	Command string
}

func GetPsProcesses(ctx context.Context) ([]*PsProcess, error) {
	args := []string{"auxc"}
	out, err := commandWithContext(ctx, "ps", args...)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(out), "\n")
	processes := []*PsProcess{}

	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			break
		}

		cmd := strings.Join(fields[10:], " ")
		pid, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, err
		}

		cpuPerc, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			return nil, err
		}

		memPerc, err := strconv.ParseFloat(fields[3], 64)
		if err != nil {
			return nil, err
		}

		process := &PsProcess{
			User:    fields[0],
			Pid:     pid,
			CpuPerc: cpuPerc,
			MemPerc: memPerc,
			Command: cmd,
		}

		processes = append(processes, process)
	}

	return processes, nil
}

func (this *PsProcess) String() string {
	return fmt.Sprintf("%s,%d,%f,%f,%s\n", this.User, this.Pid, this.CpuPerc, this.MemPerc, this.Command)
}

func topProcesses(ctx context.Context) ([]*PsProcess, []*PsProcess) {
	const numRowsShown int = 20
	procs, err := GetPsProcesses(ctx)
	if err != nil {
		panic(err)
	}

	sort.Slice(procs, func(i, j int) bool {
		return procs[i].CpuPerc > procs[j].CpuPerc
	})

	procsByCpu := make([]*PsProcess, min(numRowsShown, len(procs)))
	copy(procsByCpu, procs)

	sort.Slice(procs, func(i, j int) bool {
		return procs[i].MemPerc > procs[j].MemPerc
	})

	procsByMem := make([]*PsProcess, min(numRowsShown, len(procs)))
	copy(procsByMem, procs)

	return procsByCpu, procsByMem
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
}

var cli struct {
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
		Widgets:           []int{WidgetCPULoad, WidgetCPUPerc, WidgetNetworkIO, WidgetDiskIOPS, WidgetTopCPU, WidgetTopMem},
		SelectWidgetsMode: false,
	}
}

func (this *PoptopConfig) Finalize() {
	// Calculate the number of samples we'll retain by dividing the chart duration by the sampling interval
	this.NumSamples = int(math.Ceil(float64(this.ChartDuration) / float64(this.SampleInterval)))
}

// rootID is the ID assigned to the root container.
const rootID = "root"

func main() {
	var err error
	config := DefaultConfig()
	kong.Parse(&cli)
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

	ctx, cancel := context.WithCancel(context.Background())
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
