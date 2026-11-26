// SPDX-License-Identifier: AGPL-3.0-only

//go:build unix

package node_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/voice/sfu"
	"github.com/mycroblog/my/internal/adapter/voice/sfu/node/nodetest"
	"github.com/mycroblog/my/internal/app/stream"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/pkg/testkit"
)

// The measurement of what a media node costs (ADR-0101 §16), run by hand with
// `scripts/task.sh go:voice-sfu-measure`: TEST_VOICE_SFU_MEASURE=1 runs TestMeasureCost, which
// runs the node in this process and its simulated clients in a child process
// (TestMeasureClients), so that the memory and the CPU it reads are the node's alone.
const (
	measureEnv = "TEST_VOICE_SFU_MEASURE"
	clientsEnv = "TEST_VOICE_SFU_CLIENTS"
	// readyLine is what the child process writes once its clients connected and speak.
	readyLine = "voice-sfu-clients-ready"
	// measureWarmup is how long the clients speak before a window, and measureWindow how long
	// the CPU is read for.
	measureWarmup = 3 * time.Second
	measureWindow = 10 * time.Second
)

// measuredClient is a client that the child process connects.
type measuredClient struct {
	URL     string `json:"url"`
	Ticket  string `json:"ticket"`
	Session string `json:"session"`
	Speaks  bool   `json:"speaks"`
}

// TestMeasureClients is the child process of TestMeasureCost: it connects the clients of the
// file that TEST_VOICE_SFU_CLIENTS names, makes the speakers speak, writes readyLine, and leaves
// once its standard input closes.
func TestMeasureClients(t *testing.T) {
	path := os.Getenv(clientsEnv)
	if path == "" {
		t.Skip("the child process of TestMeasureCost")
	}
	b, err := os.ReadFile(path) //nolint:gosec // G304: the parent process names the file it wrote
	if err != nil {
		t.Fatal(err)
	}
	var specs []measuredClient
	if err := json.Unmarshal(b, &specs); err != nil {
		t.Fatal(err)
	}
	// Every client joins at the same moment, as a burst of joins does.
	clients := make([]*nodetest.Client, len(specs))
	var wg sync.WaitGroup
	for i, s := range specs {
		wg.Go(func() {
			c, err := nodetest.Dial(t, s.URL, s.Ticket, s.Session)
			if err != nil {
				t.Errorf("%s: %v", s.Session, err)
				return
			}
			clients[i] = c
		})
	}
	wg.Wait()
	if t.Failed() {
		t.FailNow()
	}
	nodetest.WaitConnected(t, clients...)
	ctx, stop := context.WithCancel(t.Context())
	var speaking sync.WaitGroup
	for i, s := range specs {
		if s.Speaks {
			speaking.Go(func() { clients[i].Speak(ctx, 30) })
		}
	}
	fmt.Println(readyLine)
	_, _ = io.Copy(io.Discard, os.Stdin)
	stop()
	speaking.Wait()
	for _, c := range clients {
		c.Close()
	}
}

// footprint is the memory that the process holds once collected: the live heap and the stacks of
// its goroutines.
func footprint() (heap, stacks int64) {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return int64(m.HeapAlloc), int64(m.StackInuse) //nolint:gosec // far below 2^63
}

// cpuTime returns the CPU time of the process, user and system.
func cpuTime(t *testing.T) time.Duration {
	var u syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &u); err != nil {
		t.Fatal(err)
	}
	return time.Duration(u.Utime.Nano() + u.Stime.Nano())
}

// scenario is a load of a node: rooms of participants each, speakers of whom make sound.
type scenario struct {
	rooms, participants, speakers int
}

func (s scenario) String() string {
	return fmt.Sprintf("%d room(s) x %d participants, %d speaker(s) each", s.rooms, s.participants, s.speakers)
}

// measured is what a scenario cost the node.
type measured struct {
	heap, stacks int64
	cpu          float64 // CPU-seconds per second
	forwarded    float64 // packets per second
}

// TestMeasureCost measures a media node on loopback: the memory of an idle node, of a room and of
// a participant, and the CPU of rooms of N participants with K speakers making sound (Opus at 50
// packets a second), per second and per forwarded packet. It logs a table; it asserts nothing.
func TestMeasureCost(t *testing.T) {
	if os.Getenv(measureEnv) == "" {
		t.Skip("set " + measureEnv + "=1 to measure the cost of a media node (scripts/task.sh go:voice-sfu-measure)")
	}
	clock := testkit.NewClock(time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC))
	reg := sfu.NewRegistry(testkit.NewKeyValueStore(clock), clock)
	rs := &rooms{state: map[string]appvoice.MediaRoom{}}
	heap0, stacks0 := footprint()
	e := newNodeEnvWith(t, "node-1", reg, reg, clock, rs, stream.NewHub(stream.HubOptions{}), 200)
	heap1, stacks1 := footprint()
	idle := runtime.NumGoroutine()
	t.Logf("machine: %s, %d CPUs (GOMAXPROCS %d), %s %s/%s", cpuModel(), runtime.NumCPU(), runtime.GOMAXPROCS(0), runtime.Version(), runtime.GOOS, runtime.GOARCH)
	t.Logf("idle node: heap %s, stacks %s", kib(heap1-heap0), kib(stacks1-stacks0))

	// The memory of a participant, of a room, and of a speaker that a participant hears: 100
	// listeners in one room (P = 100 p), in 50 rooms of 2 (R = 50 r + 100 p), and in one room with
	// a speaker whom the 99 others hear (S = P + 99 s).
	alone := scenario{rooms: 1, participants: 100}
	paired := scenario{rooms: 50, participants: 2}
	heard := scenario{rooms: 1, participants: 100, speakers: 1}
	results := map[scenario]measured{}
	order := []scenario{alone, paired, heard}
	for _, s := range order {
		results[s] = measureScenario(t, e, rs, s, idle)
	}
	total := func(s scenario) float64 { return float64(results[s].heap + results[s].stacks) }
	participant := total(alone) / 100
	room := (total(paired) - total(alone)) / 49
	slot := (total(heard) - total(alone)) / 99
	t.Logf("a participant adds %s, a room %s, and a speaker %s to each participant who hears them (heap and stacks)",
		kib(int64(participant)), kib(int64(room)), kib(int64(slot)))

	for _, s := range []scenario{
		{rooms: 1, participants: 10, speakers: 1}, {rooms: 1, participants: 10, speakers: 3},
		{rooms: 1, participants: 50, speakers: 1}, {rooms: 1, participants: 50, speakers: 5},
		{rooms: 1, participants: 100, speakers: 3},
	} {
		results[s] = measureScenario(t, e, rs, s, idle)
		order = append(order, s)
	}
	var out strings.Builder
	fmt.Fprintf(&out, "\n| load | heap | stacks | forwarded packets/s | CPU-seconds/s | µs of CPU per forwarded packet |\n|---|---|---|---|---|---|\n")
	for _, s := range order {
		m := results[s]
		per := "-"
		if m.forwarded > 0 {
			per = fmt.Sprintf("%.1f", m.cpu/m.forwarded*1e6)
		}
		fmt.Fprintf(&out, "| %s | %s | %s | %.0f | %.3f | %s |\n", s, kib(m.heap), kib(m.stacks), m.forwarded, m.cpu, per)
	}
	t.Log(out.String())
}

// settle waits for the goroutines of the clients that left to end, down to the idle node's, so
// that what they held can be collected.
func settle(t *testing.T, idle int, what string) {
	t.Helper()
	if !testkit.Eventually(t, time.Minute, func() bool { return runtime.NumGoroutine() <= idle }) {
		t.Logf("%s: %d goroutines, more than the %d of the idle node", what, runtime.NumGoroutine(), idle)
	}
}

// measureScenario connects the clients of s from a child process, and measures the node while
// the speakers speak: the CPU over measureWindow, and the memory that the node holds while they
// are connected more than once they left.
func measureScenario(t *testing.T, e *env, rs *rooms, s scenario, idle int) measured {
	t.Helper()
	settle(t, idle, s.String()+", before")
	heap0, stacks0 := footprint()
	var specs []measuredClient
	var opened []string
	for r := range s.rooms {
		id := fmt.Sprintf("measure-%d-%d-%d-%d", s.rooms, s.participants, s.speakers, r)
		st := appvoice.MediaRoom{Open: true, MaxParticipants: s.participants}
		for i := range s.participants {
			st.Sessions = append(st.Sessions, appvoice.MediaSession{ID: fmt.Sprintf("%s-s%d", id, i), AccountID: fmt.Sprintf("%s-a%d", id, i), CanSpeak: i < s.speakers})
		}
		rs.set(id, st)
		ref := appvoice.RoomRef{ID: id, MaxParticipants: s.participants}
		if err := e.transport.Open(t.Context(), ref); err != nil {
			t.Fatalf("Open %s: %v", id, err)
		}
		opened = append(opened, id)
		for _, ms := range st.Sessions {
			g, err := e.transport.Grant(t.Context(), ref, appvoice.Session{ID: ms.ID, AccountID: ms.AccountID}, ms.CanSpeak)
			if err != nil {
				t.Fatalf("Grant: %v", err)
			}
			specs = append(specs, measuredClient{URL: g.URL, Ticket: g.Token, Session: ms.ID, Speaks: ms.CanSpeak})
		}
	}
	path := t.TempDir() + "/clients.json"
	b, err := json.Marshal(specs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestMeasureClients$", "-test.count=1") //nolint:gosec // G204: the test binary itself
	cmd.Env = append(os.Environ(), clientsEnv+"="+path, measureEnv+"=")
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ready := make(chan bool, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		ok := false
		for sc.Scan() {
			switch {
			case sc.Text() == readyLine && !ok:
				ok = true
				ready <- true
			case sc.Text() != readyLine:
				fmt.Fprintln(os.Stderr, "clients:", sc.Text())
			}
		}
		if !ok {
			ready <- false
		}
	}()
	select {
	case ok := <-ready:
		if !ok {
			_ = cmd.Wait()
			t.Fatalf("%s: the clients did not connect", s)
		}
	case <-time.After(60 * time.Second):
		t.Fatalf("%s: the clients did not connect within a minute", s)
	}
	n := s.rooms * s.participants
	if !testkit.Eventually(t, 10*time.Second, func() bool { return e.node.Sessions() == n }) {
		t.Fatalf("%s: Sessions = %d, want %d", s, e.node.Sessions(), n)
	}
	<-time.After(measureWarmup)

	cpu0, at0, fwd0 := cpuTime(t), time.Now(), e.node.Totals().Forwarded
	<-time.After(measureWindow)
	cpu1, at1, fwd1 := cpuTime(t), time.Now(), e.node.Totals().Forwarded
	heap1, stacks1 := footprint()
	wall := at1.Sub(at0).Seconds()
	m := measured{cpu: (cpu1 - cpu0).Seconds() / wall, forwarded: float64(fwd1-fwd0) / wall}
	if tot := e.node.Totals(); tot.QueueFull > 0 {
		t.Logf("%s: %d packets dropped for a full queue", s, tot.QueueFull)
	}

	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("%s: the clients' process: %v", s, err)
	}
	if !testkit.Eventually(t, 30*time.Second, func() bool { return e.node.Sessions() == 0 && e.node.Rooms() == 0 }) {
		t.Fatalf("%s: once the clients left, Sessions = %d, Rooms = %d", s, e.node.Sessions(), e.node.Rooms())
	}
	for _, id := range opened {
		if err := e.transport.Close(t.Context(), appvoice.RoomRef{ID: id}); err != nil {
			t.Fatalf("Close %s: %v", id, err)
		}
	}
	settle(t, idle, s.String()+", after")
	heap2, stacks2 := footprint()
	m.heap, m.stacks = heap1-heap2, stacks1-stacks2
	t.Logf("%s: heap %s, stacks %s (%s and %s more than before the clients joined), %.0f forwarded packets/s, %.3f CPU-seconds/s",
		s, kib(m.heap), kib(m.stacks), kib(heap1-heap0), kib(stacks1-stacks0), m.forwarded, m.cpu)
	return m
}

func kib(n int64) string { return fmt.Sprintf("%.0f KiB", float64(n)/1024) }

// cpuModel returns the model of the processor from /proc/cpuinfo, or GOARCH elsewhere.
func cpuModel() string {
	b, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return runtime.GOARCH
	}
	for line := range strings.SplitSeq(string(b), "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) == "model name" {
			return strings.TrimSpace(v)
		}
	}
	return runtime.GOARCH
}
