#!/bin/bash
# measure.sh <label> <pid> <pprof addr>: the memory of a process, into ./out/<label>/
set -eu
L=$1; PID=$2; A=$3; O=out/$L; mkdir -p $O
grep -E '^(VmRSS|RssAnon|RssFile|RssShmem|VmHWM|Threads)' /proc/$PID/status > $O/status.txt
cat /proc/$PID/smaps_rollup > $O/smaps_rollup.txt
cat /proc/$PID/smaps > $O/smaps.txt
curl -fsS "http://$A/debug/pprof/heap?debug=1" | sed -n '/^# runtime.MemStats/,$p' > $O/memstats.txt
curl -fsS "http://$A/debug/pprof/heap" -o $O/heap.pb.gz
curl -fsS "http://$A/debug/pprof/goroutine" -o $O/goroutine.pb.gz
curl -fsS "http://$A/debug/pprof/goroutine?debug=1" -o $O/goroutine.txt
echo "== $L"; cat $O/status.txt
