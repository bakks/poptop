# Poptop

A modern top command that charts system metrics like CPU load, network IO, etc in the terminal.

"What's going on with my local system?". Poptop turns your terminal into a dynamic charting tool for system metrics. While the top and htop commands show precise point-in-time data, Poptop aims to provide metrics over a time window to give a better at-a-glance summary of your system's activity. And make it look cool.

<a href="http://www.youtube.com/watch?feature=player_embedded&v=sk_Xbdyac-g
" target="_blank"><img src="https://github.com/bakks/poptop/raw/master/assets/screenshot.png" 
alt="Video of Poptop" /></a>

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

## Layout

By default, all charts will be stacked vertically. You can use the `-z` flag to stack them horizontally instead.

You can also use the `-w` flag to arrange charts in a square, i.e. to switch between vertical and horizontal stacking as the layout is built.
