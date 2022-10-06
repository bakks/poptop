package main

import (
	"context"
	"fmt"
	"time"

	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/container"
	"github.com/mum4k/termdash/linestyle"
	"github.com/mum4k/termdash/terminal/terminalapi"
	"github.com/mum4k/termdash/widgetapi"
	"github.com/mum4k/termdash/widgets/linechart"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/net"
)

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

// Create a widget that shows CPU load measured at 1min, 5min, 15min averages.
// This uses a sysctl call to find CPU load.
//
// Load is one of the simplest metrics for understanding how busy your system is.
// It means roughly how many processes are executing or waiting to execute on a CPU.
// If load is higher than the number of CPU cores on your system then it indicates
// processes are having to wait for execution.
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

// Create a chart to show min, average, max CPU busy % time.
// On MacOS this calls host_processor_info().
// The judgement call here is that min, avg, max is a simpler way to understand CPU load
// rather than a single average, or charting per-CPU time.
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

// Chart to show throughput on the selected network device in kibibytes per second
// using data from the netstat command.
// We automatically choose a network device based on which device has received the most
// inbound data since system start, changing dynamically if this this switches to a new device.
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

// Chart to show Disk IOPS (input/output operations per second) over time using data from iostat.
// Arguably, in an everyday scenario with many heavy processes then IOPS is a simpler metric than
// throughput, but if disk load is skewed to a specific process (e.g. heavy file copies, database
// operations), then disk throughput may be a better metric.
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

		// TODO logic for merging disk iostats
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

// Chart to show disk IO throughput in kibibytes per second based on iostat output.
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
