package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/container"
	"github.com/mum4k/termdash/widgets/text"
)

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

	// Sample top less frequently than configured for other charts because it's a point-in-time measure
	interval := config.SampleInterval * 4

	go periodic(ctx, interval, func() error {
		topCpu, topMem := topProcesses(ctx, config)
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

	cpuTitle := cell.NewRichTextString(ColorWidgetTitle).
		AddOpt(cell.Bold()).
		AddText(" Top CPU Processes (%, pid, command) ")

	memTitle := cell.NewRichTextString(ColorWidgetTitle).
		AddOpt(cell.Bold()).
		AddText(" Top Memory Processes (%, pid, command) ")

	cpuOpts := makeContainer(cpuTextBox, cpuTitle)
	memOpts := makeContainer(memTextBox, memTitle)

	return cpuOpts, memOpts, nil
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

// Create CPU and Memory top lists using output from a shared ps command execution.
func topProcesses(ctx context.Context, config *PoptopConfig) ([]*PsProcess, []*PsProcess) {
	procs, err := GetPsProcesses(ctx)
	if err != nil {
		panic(err)
	}

	sort.Slice(procs, func(i, j int) bool {
		return procs[i].CpuPerc > procs[j].CpuPerc
	})

	procsByCpu := make([]*PsProcess, min(config.TopRowsShown, len(procs)))
	copy(procsByCpu, procs)

	sort.Slice(procs, func(i, j int) bool {
		return procs[i].MemPerc > procs[j].MemPerc
	})

	procsByMem := make([]*PsProcess, min(config.TopRowsShown, len(procs)))
	copy(procsByMem, procs)

	return procsByCpu, procsByMem
}
