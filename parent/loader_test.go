package parent

import (
	"sync"
	"testing"
	"time"
)

type testStarter struct {
	name          string
	stopPriority  uint
	stopAllowAsync bool
	stopMaxWait   time.Duration
	initCalled    bool
	startCount    int
	stopCount     int
	startOrder    *[]string
	stopOrder     *[]string
}

func newTestStarter(name string, stopPriority uint) *testStarter {
	return &testStarter{
		name:         name,
		stopPriority: stopPriority,
		stopMaxWait:  time.Second,
	}
}

func (s *testStarter) Setting() *Setting {
	return NewSetting(s.name, s.stopPriority, s.stopAllowAsync, s.stopMaxWait, func(instance any) {
		s.initCalled = instance == s
	})
}

func (s *testStarter) Start() (any, error) {
	s.startCount++
	if s.startOrder != nil {
		*s.startOrder = append(*s.startOrder, s.name)
	}
	return s, nil
}

func (s *testStarter) Stop(maxWaitTime time.Duration) (gracefully, stopped bool, err error) {
	s.stopCount++
	if s.stopOrder != nil {
		*s.stopOrder = append(*s.stopOrder, s.name)
	}
	return true, true, nil
}

func resetTestLoader() {
	loader = nil
	once = sync.Once{}
}

func assertStringSliceEqual(t *testing.T, actual, expected []string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("slice length mismatch, actual=%v expected=%v", actual, expected)
	}
	for i := range actual {
		if actual[i] != expected[i] {
			t.Fatalf("slice item mismatch, actual=%v expected=%v", actual, expected)
		}
	}
}

func assertContains(t *testing.T, actual []string, expected string) {
	t.Helper()
	for _, item := range actual {
		if item == expected {
			return
		}
	}
	t.Fatalf("expected %q in %v", expected, actual)
}

func TestInitStarterLoaderWithEmptyStartersIsSafe(t *testing.T) {
	resetTestLoader()

	loader := InitStarterLoader(nil)
	if loader == nil {
		t.Fatal("loader should not be nil")
	}
	if err := loader.Start(); err == nil {
		t.Fatal("empty loader should return error on Start")
	}

	starter := newTestStarter("dynamic", 1)
	loader.AddStarter(starter)
	if err := loader.Start(); err != nil {
		t.Fatalf("start dynamic starter failed: %v", err)
	}
	if starter.startCount != 1 {
		t.Fatalf("dynamic starter start count mismatch: %d", starter.startCount)
	}
}

func TestStartUsesRegisteredOrderAndIsIdempotent(t *testing.T) {
	resetTestLoader()

	startOrder := make([]string, 0)
	first := newTestStarter("first", 1)
	second := newTestStarter("second", 2)
	first.startOrder = &startOrder
	second.startOrder = &startOrder

	loader := InitStarterLoader([]Starter{first, second})
	if err := loader.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if err := loader.Start(); err != nil {
		t.Fatalf("repeat start failed: %v", err)
	}

	assertStringSliceEqual(t, startOrder, []string{"first", "second"})
	if !first.initCalled || !second.initCalled {
		t.Fatal("init handler should be called after successful start")
	}
}

func TestStopAllByRegisteredOrder(t *testing.T) {
	resetTestLoader()

	stopOrder := make([]string, 0)
	first := newTestStarter("first", 1)
	second := newTestStarter("second", 2)
	first.stopOrder = &stopOrder
	second.stopOrder = &stopOrder

	loader := InitStarterLoader([]Starter{first, second})
	if err := loader.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	results, err := loader.StopAllByRegisteredOrder(time.Second)
	if err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	assertStringSliceEqual(t, stopOrder, []string{"first", "second"})
	if len(results) != 2 {
		t.Fatalf("stop result count mismatch: %d", len(results))
	}
}

func TestStopAllBySettingUsesStopPriority(t *testing.T) {
	resetTestLoader()

	stopOrder := make([]string, 0)
	high := newTestStarter("high", 20)
	low := newTestStarter("low", 0)
	middle := newTestStarter("middle", 10)
	high.stopOrder = &stopOrder
	low.stopOrder = &stopOrder
	middle.stopOrder = &stopOrder

	loader := InitStarterLoader([]Starter{high, low, middle})
	if err := loader.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	results, err := loader.StopAllBySetting()
	if err != nil {
		t.Fatalf("stop by setting failed: %v", err)
	}

	assertStringSliceEqual(t, stopOrder, []string{"low", "middle", "high"})
	if len(results) != 3 {
		t.Fatalf("stop result count mismatch: %d", len(results))
	}
}

func TestStopStarterAndStoppedStarters(t *testing.T) {
	resetTestLoader()

	first := newTestStarter("first", 1)
	second := newTestStarter("second", 2)

	loader := InitStarterLoader([]Starter{first, second})
	if err := loader.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	result, err := loader.StopStarter("second", time.Second)
	if err != nil {
		t.Fatalf("stop starter failed: %v", err)
	}
	if result.StarterName != "second" || !result.Stopped || !result.Gracefully {
		t.Fatalf("unexpected stop result: %+v", result)
	}
	assertContains(t, loader.StoppedStarters(), "second")
}

func TestSettingGetters(t *testing.T) {
	handler := func(instance any) {}
	setting := NewSetting("demo", 10, true, time.Second, handler)

	if setting.StarterName() != "demo" {
		t.Fatalf("starter name mismatch: %s", setting.StarterName())
	}
	if setting.StopPriority() != 10 {
		t.Fatalf("stop priority mismatch: %d", setting.StopPriority())
	}
	if !setting.StopAllowAsync() {
		t.Fatal("stop allow async should be true")
	}
	if setting.StopMaxWaitTime() != time.Second {
		t.Fatalf("stop max wait time mismatch: %s", setting.StopMaxWaitTime())
	}
	if setting.InitHandler() == nil {
		t.Fatal("init handler should not be nil")
	}
}
