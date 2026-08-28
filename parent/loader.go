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

	// StarterStatusStarted 表示组件已经成功启动。
	StarterStatusStarted StarterStatus = 1
	// StarterStatusStopped 表示组件已经停止或尚未启动。
	StarterStatusStopped StarterStatus = -1
)

// StarterStatus 表示组件在Loader中的生命周期状态。
type StarterStatus int8

// StarterLoader 负责统一管理所有Starter组件的注册、启动和停止。
//
// 同一进程中Loader应保持单例，组件按注册顺序启动，可按配置顺序停止。
type StarterLoader struct {
	sync.Mutex
	starters starterWrappers
}

// Starter 定义可被StarterLoader统一管理的组件生命周期接口。
type Starter interface {

	// Setting 模块设置
	Setting() *Setting

	// Start 模块注册方法 启动顺序按照注册的starter顺序依次启动
	Start() (any, error)

	// Stop 声明模块的停止方法，具体超时控制由模块自行实现。
	// 		maxWaitTime 等待优雅停机的最大时间
	// 		gracefully 	是否以优雅停机的形式关闭
	// 		stopped 是否已经停止该模块，错误的汇报将导致loader无法准确判断模块状态
	// 		err 异常
	Stop(maxWaitTime time.Duration) (gracefully, stopped bool, err error)
}

// 包裹原始Starter做未来拓展
type starterWrapper struct {
	stateMutex sync.Mutex
	// 状态 0=未启动 1=已启动 -1=已停止，其他值表示启停过渡状态。
	status  StarterStatus
	starter Starter
}

// 获取Starter名称
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

// find 获取指定名称的Starter
func (s *starterWrappers) find(starterName string) *starterWrapper {
	wrapper, _ := coll.SliceFind(*s, func(wrapper *starterWrapper) bool {
		return wrapper != nil && wrapper.getStarterName() == starterName
	})
	return wrapper
}

// 检查是否所有Setting均已配置
func (s *starterWrappers) checkSetting() bool {
	return !coll.SliceContainsBy(*s, func(wrapper *starterWrapper) bool {
		return wrapper == nil || isNilStarter(wrapper.starter) || wrapper.starter.Setting() == nil
	})
}

// 未启动的组件名称
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

// Setting 定义模块初始化、重启和停止行为。
// 直接调用StopAllByRegisteredOrder时，停止优先级和异步配置不会生效。
type Setting struct {

	// 模块名称
	starterName string

	// 是否允许模块成功停止后再次启动
	allowRestart bool

	// 组件在初始化时执行指定的初始化方法 instance为各个组件的原始实例，由自模块控制，执行时机为执行Starter.Register成功后
	initHandler func(instance any)

	// 卸载时优先级，权重越小，优先级越高 (适用于starterLoader执行按设置卸载模块)
	// 注意，相同的优先级会导致不稳定排序出现不稳定的同优先级先后顺序
	stopPriority uint

	// 是否允许该模块并发停止 (适用于StarterLoader按设置停止模块)
	// 开启后会立即继续调度后续模块，但Loader仍会等待全部停止任务完成或达到总超时。
	stopAllowAsync bool

	// 等待优雅停机的最大时间 (秒) (适用于starterLoader执行按设置卸载模块)
	// StarterLoader 该超时不由Loader控制，因为无法感知真实Stop的状态，由具体模块实现
	stopMaxWaitTime time.Duration
}

// NewSetting 创建一个模块设置
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

// AllowRestart 返回模块成功停止后是否允许再次启动。
func (s *Setting) AllowRestart() bool {
	return s != nil && s.allowRestart
}

// StarterName 返回模块名称。
func (s *Setting) StarterName() string {
	if s == nil {
		return ""
	}
	return s.starterName
}

// InitHandler 返回组件启动成功后执行的初始化函数。
func (s *Setting) InitHandler() func(instance any) {
	if s == nil {
		return nil
	}
	return s.initHandler
}

// StopPriority 返回模块停止优先级，值越小越优先停止。
func (s *Setting) StopPriority() uint {
	if s == nil {
		return 0
	}
	return s.stopPriority
}

// StopAllowAsync 返回模块是否允许异步停止。
func (s *Setting) StopAllowAsync() bool {
	if s == nil {
		return false
	}
	return s.stopAllowAsync
}

// StopMaxWaitTime 返回模块优雅停止的最大等待时间。
func (s *Setting) StopMaxWaitTime() time.Duration {
	if s == nil {
		return 0
	}
	return s.stopMaxWaitTime
}

// StopResult 模块停止卸载结果
type StopResult struct {
	// 卸载模块
	StarterName string
	// 异常信息
	Error error
	// 模块是否已经完成停止
	Stopped bool
	// 是否优雅停机
	Gracefully bool
}

// InitStarterLoader 初始化或返回全局唯一的模块加载器。
//
// 首次调用时会注册传入的starters；后续需要动态增加组件时应使用AddStarter。
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

// AddStarter 动态添加一个或多个Starter组件。
//
// 新增组件会追加到注册列表末尾，因此启动顺序仍遵循先注册先启动。
func (s *StarterLoader) AddStarter(starters ...Starter) {
	newStarterWrappers := coll.SliceCollect(starters, func(item Starter) *starterWrapper {
		return &starterWrapper{starter: item}
	})
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	s.starters = append(s.starters, newStarterWrappers...)
}

// Start 按注册顺序启动所有未启动的Starter组件。
func (s *StarterLoader) Start() error {
	starters := s.snapshotStarters()
	if len(starters) == 0 {
		return ErrMissStarters
	}
	starterNames := make(map[string]struct{}, len(starters))
	for _, wrapper := range starters {
		// Setting可能依赖前序Starter完成初始化，必须在实际启动当前Starter前再校验。
		if err := validateStarter(wrapper, starterNames); err != nil {
			return err
		}
		if err := start(wrapper); err != nil {
			return err
		}
	}
	return nil
}

// StartStarter 启动指定名称且尚未启动的Starter组件。
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

// StopAllBySetting 按Setting中的停止配置停止所有Starter组件。
//
// 停止优先级由Setting.stopPriority决定，值越小越优先停止。
// 返回结果与排序后的Starter一一对应；总超时时尚未完成的项为nil，后台任务不会修改已返回的结果切片。
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

// StoppedStarters 返回所有未处于Started状态的Starter组件名称。
func (s *StarterLoader) StoppedStarters() []string {
	starters := s.snapshotStarters()
	if len(starters) == 0 {
		return nil
	}
	return starters.stoppedStarters()
}

// StopAllByRegisteredOrder 按注册顺序停止所有Starter组件，并忽略Setting中的停止配置。
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

// StopStarter 停止指定名称的Starter组件。
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

// snapshotStarters 返回当前注册列表的快照，生命周期回调不会占用Loader注册表锁。
func (s *StarterLoader) snapshotStarters() starterWrappers {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	return coll.SliceCollect(s.starters, func(wrapper *starterWrapper) *starterWrapper {
		return wrapper
	})
}

// validate 校验Starter注册项，避免生命周期执行期间出现nil引用或名称歧义。
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

// validateStarter 按启动顺序校验单个Starter，避免提前触发后续Starter的Setting逻辑。
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

// isNilStarter 同时识别nil接口和包含nil指针的Starter接口。
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

// 启动指定的模块 如果已启动则忽略
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

// 停止指定的模块
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
