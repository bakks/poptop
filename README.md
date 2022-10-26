# Poptop

A modern top command that charts system metrics like CPU load, network IO, etc in the terminal.

"What's going on with my local system?". I often want a quick overview of the load on my system, more visually and with a higher-level view than the classic `top` program. Poptop turns your terminal into a dynamic charting tool for system metrics. While the top and htop commands show precise point-in-time data, Poptop aims to provide metrics over a time window to give a better at-a-glance summary of your system's activity. And make it look cool.

<a href="http://www.youtube.com/watch?feature=player_embedded&v=sk_Xbdyac-g
" target="_blank"><img src="https://github.com/bakks/poptop/raw/main/assets/screenshot.png" 
alt="Video of Poptop" /></a>

```
Flags:
  -h, --help                   Show help information
  -r, --redraw-interval=500    Redraw interval in milliseconds (how often to repaint charts)
  -s, --sample-interval=500    Sample interval in milliseconds (how often to fetch a new datapoint
  -d, --chart-duration=120     Duration of the charted series in seconds (i.e. width of chart x-axis in time), 60 == 1 minute
  -z, --split-horizontal       Arrange panes horizontally rather than vertically
  -w, --tile-windows           Tile windows rather than placing them in a horizontal or vertical line
  -a, --smooth=4               How many samples will be included in running average
  -L, --cpu-load               Add CPU Load chart to layout
  -C, --cpu-percent            Add CPU % chart to layout
  -D, --disk-iops              Add Disk IOPS chart to layout
  -E, --disk-io                Add Disk IO chart to layout
  -N, --network-io             Add Network IO chart to layout
  -T, --top-cpu                Add Top Processes by CPU list to layout
  -M, --top-memory             Add Top Processes by Memory list to layout


Examples:
  poptop -CL -d 30        Show only CPU Load and % charts for 30 second duration.

  poptop -w -LCDN         Show 4 specific charts arranged in a square.
```

## Layout

Poptop displays some default charts, but also allows you to select your own. For example, 'poptop -LC' will display only CPU load and % charts. You can also add and remove charts at runtime by pressing the key corresponding to their flag (e.g. press C to toggle the CPU % chart).

By default, all charts will be stacked vertically. You can use the -z flag to stack them horizontally instead.

You can also use the -w flag to arrange charts in a square, i.e. to switch between vertical and horizontal stacking as the layout is built. 'z' and 'w' can also be pressed at runtime to change the layout dynamically.

## Metrics

### CPU Load (1min, 5min, 15min)

Charts CPU load at 1, 5, 15min averages by calling sysctl.

Load is one of the simplest metrics for understanding how busy your system is. It means roughly how many processes are executing or waiting to execute on a CPU. If load is higher than the number of CPU cores on your system then it indicates processes are having to wait for execution.

### CPU (%) (min, avg, max)

A chart to show min, average, max CPU busy % time. On MacOS this calls `host_processor_info()`. The judgement call here is that min, avg, max is a simpler way to understand CPU load rather than a single average, or charting per-CPU time.

### Network IO (KiB/s) (send, recv)

Chart to show throughput on the selected network device in kibibytes per second using data from the netstat command. We automatically choose a network device based on which device has received the most inbound data since system start, changing dynamically if this this switches to a new device.

### Disk IOPS (read, write)

Chart to show Disk IOPS (input/output operations per second) over time using data from iostat. Arguably, in an everyday scenario with many heavy processes then IOPS is a simpler metric than throughput, but if disk load is skewed to a specific process (e.g. heavy file copies, database operations), then disk throughput may be a better metric. This chart currently shows only a single disk.

### Disk IO (KiB/s) (read, write)

Chart to show disk IO throughput in kibibytes per second based on iostat output. This chart currently shows only a single disk.

### Top CPU Processes (%, pid, command)

Show a list of top CPU processes output by the ps command, i.e. which processes are consuming the most CPU. This is sampled at one-fourth of the sample interval rate since this is a point-in-time list rather than a chart. Run 'man ps' for more information on calculation methodology.

### Top Memory Processes (%, pid, command)

Show a list of top Memory processes output by the ps command, i.e. which processes are consuming the most real memory. This is sampled at one-fourth of the sample interval rate since this is a point-in-time list rather than a chart. Run 'man ps' for more information on calculation methodology.

## Acknowledgements

Poptop is written in Golang and uses the following libraries:

- [github.com/alecthomas/kong](github.com/alecthomas/kong)
- [github.com/mum4k/termdash](github.com/mum4k/termdash) [(forked)](github.com/bakks/termdash)
- [github.com/shirou/gopsutil/v3](github.com/shirou/gopsutil/v3)

MIT License - Copyright (c) 2022 Peter Bakkum
