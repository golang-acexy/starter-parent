package parent

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/acexy/golang-toolkit/util/coll"
)

type testStarter struct {
	name           string
	allowRestart   bool
	stopPriority   uint
	stopAllowAsync bool
	stopMaxWait    time.Duration
	initCalled     bool
	startCount     int
	stopCount      int
	startOrder     *[]string
	stopOrder      *[]string
}

type callbackStarter struct {
	setting *Setting
	start   func()
	stop    func()
}

func (s *callbackStarter) Setting() *Setting {
	return s.setting
}

func (s *callbackStarter) Start() (any, error) {
	if s.start != nil {
		s.start()
	}
	return s, nil
}

func (s *callbackStarter) Stop(maxWaitTime time.Duration) (gracefully, stopped bool, err error) {
	if s.stop != nil {
		s.stop()
	}
	return true, true, nil
}

func newTestStarter(name string, stopPriority uint) *testStarter {
	return &testStarter{
		name:         name,
		stopPriority: stopPriority,
		stopMaxWait:  time.Second,
	}
}

func (s *testStarter) Setting() *Setting {
	return NewSetting(s.name, s.allowRestart, s.stopPriority, s.stopAllowAsync, s.stopMaxWait, func(instance any) {
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
	if coll.SliceContains(actual, expected) {
		return
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

func TestStartStarterRejectsRestartWhenDisabled(t *testing.T) {
	resetTestLoader()

	starter := newTestStarter("disabled", 1)
	loader := InitStarterLoader([]Starter{starter})
	if err := loader.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if _, err := loader.StopStarter("disabled", time.Second); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
	if err := loader.StartStarter("disabled"); !errors.Is(err, ErrStarterRestartDisabled) {
		t.Fatalf("expected restart disabled error, got: %v", err)
	}
	if starter.startCount != 1 {
		t.Fatalf("disabled starter should not restart, start count: %d", starter.startCount)
	}
}

func TestStartStarterAllowsRestartWhenEnabled(t *testing.T) {
	resetTestLoader()

	starter := newTestStarter("enabled", 1)
	starter.allowRestart = true
	loader := InitStarterLoader([]Starter{starter})
	if err := loader.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if _, err := loader.StopStarter("enabled", time.Second); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
	if err := loader.StartStarter("enabled"); err != nil {
		t.Fatalf("restart failed: %v", err)
	}
	if starter.startCount != 2 {
		t.Fatalf("enabled starter start count mismatch: %d", starter.startCount)
	}
}

func TestSettingGetters(t *testing.T) {
	handler := func(instance any) {}
	setting := NewSetting("demo", false, 10, true, time.Second, handler)

	if setting.StarterName() != "demo" {
		t.Fatalf("starter name mismatch: %s", setting.StarterName())
	}
	if setting.AllowRestart() {
		t.Fatal("allow restart should be false")
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

func TestLifecycleCallbacksCanCallLoader(t *testing.T) {
	resetTestLoader()

	var currentLoader *StarterLoader
	starter := &callbackStarter{
		setting: NewSetting("reentrant", false, 1, false, time.Second, nil),
	}
	starter.start = func() {
		currentLoader.AddStarter(newTestStarter("dynamic", 2))
	}
	starter.stop = func() {
		currentLoader.StoppedStarters()
	}
	currentLoader = InitStarterLoader([]Starter{starter})

	startDone := make(chan error, 1)
	go func() {
		startDone <- currentLoader.Start()
	}()
	select {
	case err := <-startDone:
		if err != nil {
			t.Fatalf("reentrant start failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("reentrant start deadlocked")
	}

	stopDone := make(chan error, 1)
	go func() {
		_, err := currentLoader.StopStarter("reentrant", time.Second)
		stopDone <- err
	}()
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("reentrant stop failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("reentrant stop deadlocked")
	}
}

func TestStopAllBySettingTimeoutReturnsStableSnapshot(t *testing.T) {
	resetTestLoader()

	releaseStop := make(chan struct{})
	stopFinished := make(chan struct{})
	starter := &callbackStarter{
		setting: NewSetting("async", false, 1, true, time.Second, nil),
		stop: func() {
			<-releaseStop
			close(stopFinished)
		},
	}
	currentLoader := InitStarterLoader([]Starter{starter})
	if err := currentLoader.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	results, err := currentLoader.StopAllBySetting(10 * time.Millisecond)
	if !errors.Is(err, ErrStopAllTimeout) {
		t.Fatalf("expected stop timeout, got: %v", err)
	}
	if len(results) != 1 || results[0] != nil {
		t.Fatalf("unexpected timeout snapshot: %+v", results)
	}

	close(releaseStop)
	select {
	case <-stopFinished:
	case <-time.After(time.Second):
		t.Fatal("background stop did not finish")
	}
	if results[0] != nil {
		t.Fatalf("returned snapshot changed after timeout: %+v", results)
	}
}

func TestStopAllBySettingKeepsPriorityResultOrderForAsyncStops(t *testing.T) {
	resetTestLoader()

	releaseFirst := make(chan struct{})
	secondFinished := make(chan struct{})
	first := &callbackStarter{
		setting: NewSetting("first", false, 1, true, time.Second, nil),
		stop: func() {
			<-releaseFirst
		},
	}
	second := &callbackStarter{
		setting: NewSetting("second", false, 2, true, time.Second, nil),
		stop: func() {
			close(secondFinished)
		},
	}
	currentLoader := InitStarterLoader([]Starter{second, first})
	if err := currentLoader.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	type stopOutcome struct {
		results []*StopResult
		err     error
	}
	stopDone := make(chan stopOutcome, 1)
	go func() {
		results, err := currentLoader.StopAllBySetting()
		stopDone <- stopOutcome{results: results, err: err}
	}()
	select {
	case <-secondFinished:
	case <-time.After(time.Second):
		t.Fatal("second asynchronous stop did not finish")
	}
	close(releaseFirst)
	outcome := <-stopDone
	if outcome.err != nil {
		t.Fatalf("stop by setting failed: %v", outcome.err)
	}
	if len(outcome.results) != 2 || outcome.results[0].StarterName != "first" || outcome.results[1].StarterName != "second" {
		t.Fatalf("unexpected result order: %+v", outcome.results)
	}
}

func TestStarterInputValidation(t *testing.T) {
	tests := []struct {
		name     string
		starters []Starter
		expected error
	}{
		{name: "nil starter", starters: []Starter{nil}, expected: ErrNilStarter},
		{name: "typed nil starter", starters: []Starter{(*testStarter)(nil)}, expected: ErrNilStarter},
		{name: "missing setting", starters: []Starter{&callbackStarter{}}, expected: ErrSomeStarterNoSetting},
		{name: "empty name", starters: []Starter{&callbackStarter{setting: NewSetting("", false, 1, false, time.Second, nil)}}, expected: ErrEmptyStarterName},
		{
			name: "duplicate name",
			starters: []Starter{
				newTestStarter("duplicate", 1),
				newTestStarter("duplicate", 2),
			},
			expected: ErrDuplicateStarterName,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resetTestLoader()
			currentLoader := InitStarterLoader(test.starters)
			if err := currentLoader.Start(); !errors.Is(err, test.expected) {
				t.Fatalf("expected %v, got: %v", test.expected, err)
			}
		})
	}
}
