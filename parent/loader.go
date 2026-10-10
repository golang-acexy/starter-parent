package parent

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/acexy/golang-toolkit/logger"
	"github.com/acexy/golang-toolkit/util/coll"
)

var loader *StarterLoader
var once sync.Once

const (
	starterStatusStarting StarterStatus = 2
	starterStatusStopping StarterStatus = -2

	// StarterStatusStarted means startup and its initialization callback completed.
	StarterStatusStarted StarterStatus = 1
	// StarterStatusStopped means the component reported a completed stop; the initial state is zero.
	StarterStatusStopped StarterStatus = -1
)

// StarterStatus represents the lifecycle state tracked by the loader.
type StarterStatus int8

// StarterLoader coordinates component registration, startup, and shutdown.
// Registry access and transition decisions are synchronized; overlapping lifecycle
// calls are not readiness barriers. Prefer one startup followed by one shutdown.
// Do not copy a loader after first use. Callbacks are not panic-recovered.
type StarterLoader struct {
	// Mutex protects registry membership; do not hold it when calling loader methods.
	sync.Mutex
	starters starterWrappers
}

// Starter defines component-owned initialization and shutdown managed by a loader.
type Starter interface {

	// Setting returns non-nil settings with a stable, non-empty, unique name.
	// It may be called repeatedly, including during asynchronous shutdown. Implementations
	// must support those calls safely and keep settings stable during an operation.
	Setting() *Setting

	// Start initializes resources and returns the instance passed to InitHandler.
	// It must clean up its own partial initialization on error; the loader does not
	// roll back earlier components or recover panics. Dependencies must be available.
	Start() (any, error)

	// Stop closes resources and enforces its own maxWaitTime policy, including zero
	// or negative durations. The loader does not force cancellation. Report stopped
	// accurately even on error; gracefully and stopped are independent results.
	Stop(maxWaitTime time.Duration) (gracefully, stopped bool, err error)
}

// starterWrapper tracks one registration independently of the registry lock.
type starterWrapper struct {
	stateMutex sync.Mutex
	status     StarterStatus
	starter    Starter
}

// getStarterName provides a fallback for missing or invalid registrations.
func (s *starterWrapper) getStarterName() string {
	if s == nil || isNilStarter(s.starter) {
		return "unnamed"
	}
	setting := s.starter.Setting()
	if setting != nil && setting.starterName != "" {
		return setting.starterName
	}
	return "unnamed"
}

type starterWrappers []*starterWrapper

// find returns the first registration matching the requested name.
func (s *starterWrappers) find(starterName string) *starterWrapper {
	wrapper, _ := coll.SliceFind(*s, func(wrapper *starterWrapper) bool {
		return wrapper != nil && wrapper.getStarterName() == starterName
	})
	return wrapper
}

// checkSetting checks that every registration has a starter and settings.
func (s *starterWrappers) checkSetting() bool {
	return !coll.SliceContainsBy(*s, func(wrapper *starterWrapper) bool {
		return wrapper == nil || isNilStarter(wrapper.starter) || wrapper.starter.Setting() == nil
	})
}

// stoppedStarters includes initial, stopped, and transitional states.
func (s *starterWrappers) stoppedStarters() []string {
	return coll.SliceFilterCollect(*s, func(wrapper *starterWrapper) (string, bool) {
		if wrapper == nil {
			return "unnamed", true
		}
		starterName := wrapper.getStarterName()
		wrapper.stateMutex.Lock()
		defer wrapper.stateMutex.Unlock()
		return starterName, wrapper.status != StarterStatusStarted
	})
}

// Setting defines naming, restart, initialization, and shutdown policies.
// Instances created by NewSetting support concurrent reads. Shutdown policies
// apply only to StopAllBySetting; other stop methods use their explicit duration.
type Setting struct {
	starterName string

	allowRestart bool

	initHandler func(instance any)

	stopPriority uint

	stopAllowAsync bool

	stopMaxWaitTime time.Duration
}

// NewSetting stores policies without validating names or durations.
// allowRestart permits startup after a completed stop. Lower stopPriority runs
// first, with no stable order for ties; stopAllowAsync permits concurrent stopping.
// stopMaxWaitTime is forwarded unchanged. A nil initHandler disables the callback;
// otherwise it runs after every successful start, before marking the component started.
func NewSetting(starterName string, allowRestart bool, stopPriority uint, stopAllowAsync bool, stopMaxWaitTime time.Duration, initHandler func(instance any)) *Setting {
	return &Setting{
		starterName:     starterName,
		allowRestart:    allowRestart,
		stopPriority:    stopPriority,
		stopAllowAsync:  stopAllowAsync,
		stopMaxWaitTime: stopMaxWaitTime,
		initHandler:     initHandler,
	}
}

// AllowRestart reports restart permission; a nil receiver returns false.
func (s *Setting) AllowRestart() bool {
	return s != nil && s.allowRestart
}

// StarterName returns the registration name; a nil receiver returns an empty string.
func (s *Setting) StarterName() string {
	if s == nil {
		return ""
	}
	return s.starterName
}

// InitHandler returns the post-start callback; a nil receiver returns nil.
func (s *Setting) InitHandler() func(instance any) {
	if s == nil {
		return nil
	}
	return s.initHandler
}

// StopPriority returns shutdown priority, lowest first; a nil receiver returns zero.
func (s *Setting) StopPriority() uint {
	if s == nil {
		return 0
	}
	return s.stopPriority
}

// StopAllowAsync reports asynchronous shutdown permission; a nil receiver returns false.
func (s *Setting) StopAllowAsync() bool {
	if s == nil {
		return false
	}
	return s.stopAllowAsync
}

// StopMaxWaitTime returns the duration forwarded to component shutdown.
// A nil receiver returns zero; the component defines zero and negative duration behavior.
func (s *Setting) StopMaxWaitTime() time.Duration {
	if s == nil {
		return 0
	}
	return s.stopMaxWaitTime
}

// StopResult records one shutdown attempt; inspect Error and both completion flags.
type StopResult struct {
	// StarterName identifies the component targeted by this attempt.
	StarterName string
	// Error contains component or transition errors, independent of completion flags.
	Error error
	// Stopped reports resource shutdown completion and determines the next loader state.
	Stopped bool
	// Gracefully reports a graceful shutdown; it does not imply Stopped.
	Gracefully bool
}

// InitStarterLoader initializes the process-wide singleton once and returns it.
// Concurrent initialization is safe; only the first call registers its slice.
// Nil or empty input creates an empty loader. Registration is not validated here;
// later input is ignored, so use AddStarter for additional components.
func InitStarterLoader(starters []Starter) *StarterLoader {
	once.Do(func() {
		wrappers := coll.SliceCollect(starters, func(starter Starter) *starterWrapper {
			return &starterWrapper{starter: starter}
		})
		loader = &StarterLoader{
			starters: wrappers,
		}
	})
	return loader
}

// AddStarter appends registrations without validation or starting them.
// Empty input is a no-op. Registry mutation is synchronized; an in-progress
// lifecycle snapshot excludes later additions. Invalid entries can prevent later
// lifecycle operations, including shutdown; there is no removal API.
func (s *StarterLoader) AddStarter(starters ...Starter) {
	newStarterWrappers := coll.SliceCollect(starters, func(item Starter) *starterWrapper {
		return &starterWrapper{starter: item}
	})
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	s.starters = append(s.starters, newStarterWrappers...)
}

// Start starts the current snapshot in registration order, validating each item
// immediately before use to support settings that depend on earlier startup.
// Empty registration returns ErrNoStarter. The first error ends traversal without
// rollback. Started or starting components are skipped; starting is not a readiness
// barrier, so use one startup flow when dependency order matters.
func (s *StarterLoader) Start() error {
	starters := s.snapshotStarters()
	if len(starters) == 0 {
		return ErrMissStarters
	}
	starterNames := make(map[string]struct{}, len(starters))
	for _, wrapper := range starters {
		// Evaluate lazy settings only after earlier starters have initialized.
		if err := validateStarter(wrapper, starterNames); err != nil {
			return err
		}
		if err := start(wrapper); err != nil {
			return err
		}
	}
	return nil
}

// StartStarter starts the named component without starting its dependencies.
// The entire registration snapshot must be valid, even for unrelated components.
// Empty registration returns ErrNoStarter; an absent name returns ErrUnknownStarterName.
// Already started or starting components return nil without waiting for readiness.
func (s *StarterLoader) StartStarter(starterName string) error {
	starters := s.snapshotStarters()
	if len(starters) == 0 {
		return ErrNoStarter
	}
	if err := starters.validate(); err != nil {
		return err
	}
	wrapper := starters.find(starterName)
	if wrapper == nil {
		return fmt.Errorf("%w: %s", ErrUnknownStarterName, starterName)
	}
	return start(wrapper)
}

// StopAllBySetting validates the entire snapshot, then stops by ascending priority
// using component async and wait policies. Equal priorities have no stable order.
// Only the first allMaxWaitTime is used; omission waits for all tasks, while zero
// or negative values permit immediate timeout. The timer follows validation and
// sorting, and timeout does not cancel background work. Results follow sorted order;
// unfinished slots are nil and background work never changes the returned snapshot.
// Component errors are stored in results; the method error reports registration
// failures or ErrStopAllTimeout. Empty registration returns ErrNoStarter.
func (s *StarterLoader) StopAllBySetting(allMaxWaitTime ...time.Duration) ([]*StopResult, error) {
	starters := s.snapshotStarters()
	if len(starters) == 0 {
		return nil, ErrNoStarter
	}
	if err := starters.validate(); err != nil {
		return nil, err
	}
	copied := coll.SliceCollect(starters, func(item *starterWrapper) *starterWrapper {
		return item
	})
	coll.SliceSort(copied, func(e *starterWrapper) int {
		return int(e.starter.Setting().stopPriority)
	})
	stopResults := make([]*StopResult, len(copied))
	var wg sync.WaitGroup
	wg.Add(len(copied))
	var resultMutex sync.Mutex
	go func() {
		for index, wrapper := range copied {
			setting := wrapper.starter.Setting()
			if !setting.stopAllowAsync {
				result := stop(wrapper, setting.stopMaxWaitTime)
				resultMutex.Lock()
				stopResults[index] = result
				resultMutex.Unlock()
				wg.Done()
			} else {
				go func(resultIndex int, starterWrapper *starterWrapper, maxWaitTime time.Duration) {
					defer wg.Done()
					result := stop(starterWrapper, maxWaitTime)
					resultMutex.Lock()
					stopResults[resultIndex] = result
					resultMutex.Unlock()
				}(index, wrapper, setting.stopMaxWaitTime)
			}
		}
	}()
	snapshotResults := func() []*StopResult {
		resultMutex.Lock()
		defer resultMutex.Unlock()
		return coll.SliceCollect(stopResults, func(result *StopResult) *StopResult {
			return result
		})
	}
	if len(allMaxWaitTime) > 0 {
		allStopDone := make(chan struct{})
		go func() {
			wg.Wait()
			close(allStopDone)
		}()
		timer := time.NewTimer(allMaxWaitTime[0])
		defer timer.Stop()
		select {
		case <-allStopDone:
			return snapshotResults(), nil
		case <-timer.C:
			return snapshotResults(), ErrStopAllTimeout
		}
	} else {
		wg.Wait()
	}
	return snapshotResults(), nil
}

// StoppedStarters returns names not currently marked started, including components
// that are starting or stopping. This is a per-component observation, not an atomic
// batch status. Empty registration returns nil; invalid entries may be named unnamed.
func (s *StarterLoader) StoppedStarters() []string {
	starters := s.snapshotStarters()
	if len(starters) == 0 {
		return nil
	}
	return starters.stoppedStarters()
}

// StopAllByRegisteredOrder validates the entire snapshot and stops synchronously
// in forward registration order, ignoring individual shutdown policies.
// maxWaitTime is forwarded unchanged, not enforced by the loader.
// Empty registration returns ErrNoStarter. Component errors remain in StopResults
// and do not stop traversal or become the aggregate method error.
func (s *StarterLoader) StopAllByRegisteredOrder(maxWaitTime time.Duration) ([]*StopResult, error) {
	starters := s.snapshotStarters()
	if len(starters) == 0 {
		return nil, ErrNoStarter
	}
	if err := starters.validate(); err != nil {
		return nil, err
	}
	stopResult := make([]*StopResult, 0, len(starters))
	for _, wrapper := range starters {
		stopResult = append(stopResult, stop(wrapper, maxWaitTime))
	}
	return stopResult, nil
}

// StopStarter validates the entire snapshot and stops the named component.
// maxWaitTime is forwarded unchanged, overriding its configured wait duration.
// Empty registration returns ErrNoStarter; an absent name returns ErrUnknownStarterName.
// Component and transition errors are in StopResult.Error, with a nil method error.
func (s *StarterLoader) StopStarter(starterName string, maxWaitTime time.Duration) (*StopResult, error) {
	starters := s.snapshotStarters()
	if len(starters) == 0 {
		return nil, ErrNoStarterSet
	}
	if err := starters.validate(); err != nil {
		return nil, err
	}
	wrapper := starters.find(starterName)
	if wrapper == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnknownStarterName, starterName)
	}
	return stop(wrapper, maxWaitTime), nil
}

// snapshotStarters copies membership so callbacks run without the registry lock.
func (s *StarterLoader) snapshotStarters() starterWrappers {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	return coll.SliceCollect(s.starters, func(wrapper *starterWrapper) *starterWrapper {
		return wrapper
	})
}

// validate rejects nil registrations, missing settings, and ambiguous names.
func (s starterWrappers) validate() error {
	if !(&s).checkSetting() {
		for _, wrapper := range s {
			if wrapper == nil || isNilStarter(wrapper.starter) {
				return ErrNilStarter
			}
		}
		return ErrSomeStarterNoSetting
	}
	names := make(map[string]struct{}, len(s))
	for _, wrapper := range s {
		starterName := wrapper.starter.Setting().starterName
		if starterName == "" {
			return ErrEmptyStarterName
		}
		if _, exists := names[starterName]; exists {
			return fmt.Errorf("%w: %s", ErrDuplicateStarterName, starterName)
		}
		names[starterName] = struct{}{}
	}
	return nil
}

// validateStarter defers settings evaluation until the component is reached during startup.
func validateStarter(wrapper *starterWrapper, names map[string]struct{}) error {
	if wrapper == nil || isNilStarter(wrapper.starter) {
		return ErrNilStarter
	}
	setting := wrapper.starter.Setting()
	if setting == nil {
		return ErrSomeStarterNoSetting
	}
	starterName := setting.starterName
	if starterName == "" {
		return ErrEmptyStarterName
	}
	if _, exists := names[starterName]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateStarterName, starterName)
	}
	names[starterName] = struct{}{}
	return nil
}

// isNilStarter recognizes nil interfaces and typed nil interface values.
func isNilStarter(starter Starter) bool {
	if starter == nil {
		return true
	}
	value := reflect.ValueOf(starter)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// start guards transitions without waiting for an already-starting component.
func start(wrapper *starterWrapper) error {
	starter := wrapper.starter
	setting := starter.Setting()
	starterName := wrapper.getStarterName()
	wrapper.stateMutex.Lock()
	if wrapper.status == StarterStatusStarted || wrapper.status == starterStatusStarting {
		wrapper.stateMutex.Unlock()
		return nil
	}
	if wrapper.status == starterStatusStopping {
		wrapper.stateMutex.Unlock()
		return fmt.Errorf("%w: %s", ErrStarterStopping, starterName)
	}
	previousStatus := wrapper.status
	if previousStatus == StarterStatusStopped && (setting == nil || !setting.allowRestart) {
		wrapper.stateMutex.Unlock()
		return fmt.Errorf("%w: %s", ErrStarterRestartDisabled, starterName)
	}
	wrapper.status = starterStatusStarting
	wrapper.stateMutex.Unlock()

	current := time.Now()
	logger.Logrus().Traceln(starterName, "starting now...")
	instance, err := starter.Start()
	if err != nil {
		wrapper.stateMutex.Lock()
		wrapper.status = previousStatus
		wrapper.stateMutex.Unlock()
		logger.Logrus().WithError(err).Errorln(starterName, "start failed with error:", err)
		return err
	}
	if setting != nil && setting.initHandler != nil {
		setting.initHandler(instance)
	}
	wrapper.stateMutex.Lock()
	wrapper.status = StarterStatusStarted
	wrapper.stateMutex.Unlock()
	logger.Logrus().Traceln(starterName, "started successful cost:", time.Since(current))
	return nil
}

// stop trusts the component completion flag to update the tracked state.
func stop(wrapper *starterWrapper, maxWaitTime time.Duration) *StopResult {
	starterName := wrapper.getStarterName()
	wrapper.stateMutex.Lock()
	if wrapper.status != StarterStatusStarted {
		err := ErrStarterNotStarted
		if wrapper.status == starterStatusStopping {
			err = ErrStarterStopping
		}
		wrapper.stateMutex.Unlock()
		return &StopResult{StarterName: starterName, Error: err}
	}
	wrapper.status = starterStatusStopping
	wrapper.stateMutex.Unlock()
	starter := wrapper.starter
	current := time.Now()
	logger.Logrus().Traceln(starterName, "stopping now...")
	gracefully, stopped, err := starter.Stop(maxWaitTime)
	if err != nil {
		logger.Logrus().WithError(err).Errorln(starterName, "stop failed with error", err)
	} else {
		logger.Logrus().Traceln(starterName, "stopped successful cost:", time.Since(current))
	}
	wrapper.stateMutex.Lock()
	if stopped {
		wrapper.status = StarterStatusStopped
	} else {
		wrapper.status = StarterStatusStarted
	}
	wrapper.stateMutex.Unlock()
	return &StopResult{
		StarterName: starterName,
		Error:       err,
		Gracefully:  gracefully,
		Stopped:     stopped,
	}
}
