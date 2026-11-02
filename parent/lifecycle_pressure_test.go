package parent

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"testing"
	"testing/synctest"
	"time"
	"weak"
)

// newPressureLoader isolates measurements from the process-wide singleton.
func newPressureLoader(count int, mode string, payloadBytes int) *StarterLoader {
	current := &StarterLoader{}
	for i := 0; i < count; i++ {
		async := mode == "async" || mode == "mixed" && i%2 == 0
		payload := make([]byte, payloadBytes)
		current.AddStarter(&callbackStarter{
			setting: NewSetting(fmt.Sprintf("component-%d", i), true, uint(i), async, time.Second, nil),
			start:   func() { runtime.KeepAlive(payload) },
		})
	}
	return current
}

func checkPressureResults(tb testing.TB, results []*StopResult, err error, count int) {
	tb.Helper()
	if err != nil || len(results) != count {
		tb.Fatalf("shutdown: count=%d err=%v", len(results), err)
	}
	for _, result := range results {
		if result == nil || result.Error != nil || !result.Stopped || !result.Gracefully {
			tb.Fatalf("incomplete shutdown: %+v", result)
		}
	}
}

func TestLifecycleTimeoutWorkersDrain(t *testing.T) {
	for _, async := range []bool{false, true} {
		t.Run(fmt.Sprintf("async=%t", async), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// Virtual time makes repeated timeout/release cycles deterministic.
				for cycle := 0; cycle < 200; cycle++ {
					release := make(chan struct{})
					current := &StarterLoader{}
					current.AddStarter(&callbackStarter{
						setting: NewSetting("blocked", false, 0, async, time.Second, nil),
						stop:    func() { <-release },
					})
					if err := current.Start(); err != nil {
						t.Fatal(err)
					}
					results, err := current.StopAllBySetting(time.Millisecond)
					if !errors.Is(err, ErrStopAllTimeout) || len(results) != 1 || results[0] != nil {
						close(release)
						t.Fatalf("unexpected timeout result: %v %v", results, err)
					}
					close(release)
					synctest.Wait()
					wrapper := current.starters[0]
					wrapper.stateMutex.Lock()
					status := wrapper.status
					wrapper.stateMutex.Unlock()
					if status != StarterStatusStopped || results[0] != nil {
						t.Fatalf("background completion: status=%v snapshot=%v", status, results)
					}
				}
				// The bubble fails if any shutdown goroutine remains blocked at exit.
			})
		})
	}
}

func BenchmarkLoaderLifecycle(b *testing.B) {
	for _, count := range []int{8, 64, 256} {
		for _, mode := range []string{"registered", "sync", "async", "mixed", "mixed-deadline"} {
			b.Run(fmt.Sprintf("%s/%d", mode, count), func(b *testing.B) {
				actualMode := mode
				if mode == "mixed-deadline" {
					actualMode = "mixed"
				}
				current := newPressureLoader(count, actualMode, 0)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if err := current.Start(); err != nil {
						b.Fatal(err)
					}
					var results []*StopResult
					var err error
					switch mode {
					case "registered":
						results, err = current.StopAllByRegisteredOrder(time.Second)
					case "mixed-deadline":
						results, err = current.StopAllBySetting(time.Second)
					default:
						results, err = current.StopAllBySetting()
					}
					checkPressureResults(b, results, err, count)
				}
			})
		}
	}
}

// runPressureBatch returns only weak references, so the measurement does not own component graphs.
func runPressureBatch(t *testing.T, mode string, count int) []weak.Pointer[starterWrapper] {
	t.Helper()
	refs := make([]weak.Pointer[starterWrapper], 0, count*8)
	for i := 0; i < count; i++ {
		current := newPressureLoader(8, mode, 4096)
		if err := current.Start(); err != nil {
			t.Fatal(err)
		}
		results, err := current.StopAllBySetting(time.Second)
		checkPressureResults(t, results, err, 8)
		for _, wrapper := range current.starters {
			refs = append(refs, weak.Make(wrapper))
		}
	}
	return refs
}

func TestLifecycleMemoryPressure(t *testing.T) {
	if os.Getenv("PARENT_MEMORY_STRESS") != "1" {
		t.Skip("set PARENT_MEMORY_STRESS=1 to run repeated heap/ownership measurements")
	}
	for _, mode := range []string{"sync", "async", "mixed"} {
		t.Run(mode, func(t *testing.T) {
			// Warm up runtime and logging pools before taking the baseline.
			runPressureBatch(t, mode, 100)
			runtime.GC()
			var baseline runtime.MemStats
			runtime.ReadMemStats(&baseline)
			baseGoroutines := runtime.NumGoroutine()
			for batch := 1; batch <= 5; batch++ {
				refs := runPressureBatch(t, mode, 1000)
				live := len(refs)
				deadline := time.Now().Add(5 * time.Second)
				for live > 0 && time.Now().Before(deadline) {
					runtime.GC()
					live = 0
					for _, ref := range refs {
						if ref.Value() != nil {
							live++
						}
					}
					if live > 0 {
						time.Sleep(time.Millisecond)
					}
				}
				var stats runtime.MemStats
				runtime.ReadMemStats(&stats)
				t.Logf("batch=%d cycles=%d live_wrappers=%d heap=%d heap_delta=%d objects=%d goroutines=%d base_goroutines=%d total_alloc_delta=%d",
					batch, batch*1000, live, stats.HeapAlloc, int64(stats.HeapAlloc)-int64(baseline.HeapAlloc),
					stats.HeapObjects, runtime.NumGoroutine(), baseGoroutines, stats.TotalAlloc-baseline.TotalAlloc)
				if live != 0 {
					t.Fatalf("%d completed wrappers remain strongly reachable", live)
				}
			}
		})
	}
}
