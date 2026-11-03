#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# Runs one configuration: run.sh <dialect> <dsn> <procs> <slots-per-proc> <delay> <per-host> [window-seconds]
# Prints: dialect procs slots delay perhost jobs/s rss-MiB-total rss-peak cpu-s db-conns errors
set -u
bin=$1; dialect=$2; dsn=$3; procs=$4; slots=$5; delay=$6; perhost=$7; window=${8:-20}
secs=$(awk -v d="$delay" 'BEGIN{sub(/ms$/,"",d); if (d ~ /s$/) {sub(/s$/,"",d); print d} else print d/1000}')
total=$((procs * slots))
jobs=$(awk -v t=$total -v w=$window -v s=$secs 'BEGIN{n=int(t*(w+8)/s*1.3)+50; print n}')
err=$(mktemp)
count() {
  if [ "$dialect" = sqlite ]; then sqlite3 "${dsn#file:}" 'SELECT count(*) FROM jobs' 2>/dev/null
  else docker exec spike2004-pg psql -U my -d my -tAc 'SELECT count(*) FROM jobs'; fi
}
clear() {
  if [ "$dialect" = sqlite ]; then sqlite3 "${dsn#file:}" 'DELETE FROM jobs'
  else docker exec spike2004-pg psql -U my -d my -qc 'TRUNCATE jobs'; fi
}
conns() {
  if [ "$dialect" = sqlite ]; then echo 0
  else docker exec spike2004-pg psql -U my -d my -tAc "SELECT count(*) FROM pg_stat_activity WHERE datname='my' AND pid<>pg_backend_pid()"; fi
}
clear
"$bin" -mode load -dialect "$dialect" -dsn "$dsn" -jobs "$jobs" -delay "$delay"
pids=()
for _ in $(seq 1 "$procs"); do
  "$bin" -mode worker -dialect "$dialect" -dsn "$dsn" -slots "$slots" -per-host "$perhost" ${SPIKE_FLAGS:-} 2>>"$err" &
  pids+=($!)
done
sleep 5
c0=$(count); t0=$(date +%s.%N)
rss_peak=0; conn_peak=0
for _ in $(seq 1 $((window / 2))); do
  sleep 2
  rss=0
  for p in "${pids[@]}"; do r=$(awk '/VmRSS/{print $2}' "/proc/$p/status" 2>/dev/null); rss=$((rss + ${r:-0})); done
  [ "$rss" -gt "$rss_peak" ] && rss_peak=$rss
  n=$(conns); [ "$n" -gt "$conn_peak" ] && conn_peak=$n
done
c1=$(count); t1=$(date +%s.%N)
cpu=0
tck=$(getconf CLK_TCK)
for p in "${pids[@]}"; do s=$(awk '{print $14+$15}' "/proc/$p/stat" 2>/dev/null); cpu=$((cpu + ${s:-0})); done
kill "${pids[@]}" 2>/dev/null
wait "${pids[@]}" 2>/dev/null
races=$(grep -c 'DATA RACE' "$err"); [ "$races" -gt 0 ] && echo "DATA RACE: $races"; errs=$(grep -c 'level=ERROR' "$err"); warns=$(grep -c 'outcome=retry' "$err")
awk -v dl="$dialect" -v p="$procs" -v s="$slots" -v d="$delay" -v ph="$perhost" -v c0="$c0" -v c1="$c1" -v t0="$t0" -v t1="$t1" \
  -v rp="$rss_peak" -v cpu="$cpu" -v tck="$tck" -v cn="$conn_peak" -v e="$errs" -v w="$warns" -v tot="$total" -v sec="$secs" 'BEGIN{
  r=(c0-c1)/(t1-t0); printf "%s procs=%d slots=%d delay=%s perhost=%s jobs/s=%.1f ideal=%.1f rss_total=%.0fMiB cpu=%.1fs db_conns_peak=%d errors=%d retries=%d\n",
    dl,p,s,d,ph,r,tot/sec,rp/1024,cpu/tck,cn,e,w}'
rm -f "$err"
