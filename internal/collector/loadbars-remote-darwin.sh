#!/bin/bash
# loadbars-remote-darwin.sh - macOS version using sysctl, vm_stat, netstat, and iostat
# Emits loadbars protocol (M LOADAVG, M MEMSTATS, M NETSTATS, M CPUSTATS)
# Interval for CPU sampling (seconds)
INTERVAL=0.14

# Get number of CPUs
NCPU=$(sysctl -n hw.ncpu)

while true; do
  # Load average: from sysctl
  echo "M LOADAVG"
  sysctl -n vm.loadavg 2>/dev/null | awk '{print $2";"$3";"$4}' || echo "0;0;0"

  # Memory: convert vm_stat output to /proc/meminfo-like format
  echo "M MEMSTATS"
  vm_stat 2>/dev/null | awk '
    BEGIN { pagesize = 4096 }
    /page size of ([0-9]+)/ { pagesize = $8 }
    /Pages free:/ { free = $3 * pagesize / 1024 }
    /Pages active:/ { active = $3 * pagesize / 1024 }
    /Pages inactive:/ { inactive = $3 * pagesize / 1024 }
    /Pages speculative:/ { speculative = $3 * pagesize / 1024 }
    /Pages wired down:/ { wired = $4 * pagesize / 1024 }
    /Pages occupied by compressor:/ { compressed = $5 * pagesize / 1024 }
    END {
      total = free + active + inactive + speculative + wired + compressed
      printf "MemTotal: %d kB\n", total
      printf "MemFree: %d kB\n", free
      printf "MemAvailable: %d kB\n", free + inactive + speculative
      printf "SwapTotal: 0 kB\n"
      printf "SwapFree: 0 kB\n"
    }
  '

  # Network: use netstat -ibn for interface stats
  echo "M NETSTATS"
  netstat -ibn 2>/dev/null | awk '
    NR > 1 && $1 !~ /^Name/ && $3 ~ /^<Link/ {
      # netstat -ibn output on macOS:
      # Name Mtu Network Address Ipkts Ierrs Ibytes Opkts Oerrs Obytes Coll
      iface = $1
      ipkts = $5
      ierrs = $6
      ibytes = $7
      opkts = $8
      oerrs = $9
      obytes = $10
      
      if (ibytes ~ /^[0-9]+$/ && obytes ~ /^[0-9]+$/) {
        printf "%s:b=%s;tb=%s;p=%s;tp=%s e=%s;te=%s;d=0;td=0\n", 
          iface, ibytes, obytes, ipkts, opkts, ierrs, oerrs
      }
    }
  '

  # CPU: macOS doesn't have /proc/stat, use iostat for CPU percentages
  echo "M CPUSTATS"
  for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
    # iostat output on macOS: user sys idle
    # Convert to /proc/stat format: cpu user nice system idle iowait irq softirq steal guest guest_nice
    # We'll simulate values since macOS doesn't provide all fields
    iostat -c 1 2>/dev/null | tail -1 | awk -v ncpu="$NCPU" '
      {
        # iostat columns: %user %nice %sys %idle (approximately)
        # Multiply by ncpu to get total ticks (simulated)
        user = int($3 * ncpu * 10)
        sys = int($4 * ncpu * 10)
        idle = int($5 * ncpu * 10)
        
        # Output in /proc/stat format
        printf "cpu %d 0 %d %d 0 0 0 0 0 0\n", user, sys, idle
      }
    '
    sleep "$INTERVAL" 2>/dev/null || true
  done
done
