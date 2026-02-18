#!/bin/bash
# loadbars-remote.sh - Emits loadbars protocol (M LOADAVG, M MEMSTATS, M NETSTATS, M CPUSTATS)
# for local or remote execution. No Perl required.
# Usage: bash loadbars-remote.sh
# Interval for CPU sampling (seconds)
INTERVAL=0.14

while true; do
  # Load average: first 3 fields of /proc/loadavg joined by ;
  echo "M LOADAVG"
  read -r l1 l5 l15 _ < /proc/loadavg 2>/dev/null || true
  echo "${l1:-0};${l5:-0};${l15:-0}"

  # Memory: full /proc/meminfo
  echo "M MEMSTATS"
  cat /proc/meminfo 2>/dev/null || true

  # Network: /proc/net/dev, skip 2 header lines, then "iface: rx... tx..."
  echo "M NETSTATS"
  while IFS= read -r line; do
    line="${line/:/ }"
    set -- $line
    # $1=iface, $2=rx_bytes $3=rx_packets $4=rx_errs $5=rx_drop ... $10=tx_bytes $11=tx_packets $12=tx_errs $13=tx_drop
    if [ -n "$2" ] || [ -n "${10:-}" ]; then
      echo "$1:b=${2:-0};tb=${10:-0};p=${3:-0};tp=${11:-0} e=${4:-0};te=${12:-0};d=${5:-0};td=${13:-0}"
    fi
  done < <(tail -n +3 /proc/net/dev 2>/dev/null)

  # Disk: /proc/diskstats, one line per block device with cumulative counters
  echo "M DISKSTATS"
  while IFS= read -r line; do
    set -- $line
    # $1=major $2=minor $3=device $4=reads_completed $5=reads_merged
    # $6=sectors_read $7=ms_reading $8=writes_completed $9=writes_merged
    # $10=sectors_written $11=ms_writing $12=ios_in_progress $13=ms_io
    [ -n "$3" ] && echo "$3:rs=${6:-0};ws=${10:-0};rt=${7:-0};wt=${11:-0};io=${13:-0}"
  done < /proc/diskstats 2>/dev/null

  # CPU: /proc/stat, 20 times with INTERVAL sleep
  echo "M CPUSTATS"
  for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
    cat /proc/stat 2>/dev/null || true
    sleep "$INTERVAL" 2>/dev/null || true
  done
done
