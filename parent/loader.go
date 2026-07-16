package parent

import (
	"fmt"
	"sync"
	"time"

	"github.com/acexy/golang-toolkit/logger"
	"github.com/acexy/golang-toolkit/util/coll"
)

var loader *StarterLoader
var once sync.Once

const (
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
	starters *starterWrappers
}

// Starter 定义可被StarterLoader统一管理的组件生命周期接口。
type Starter interface {

	// Setting 模块设置
	Setting() *Setting

	// Start 模块注册方法 启动顺序按照注册的starter顺序依次启动
	Start() (any, error)

	// Stop 声明模块的卸载关闭方法 模块应当已阻塞的形式实现
	// 		maxWaitSeconds 等待优雅停机的最大时间 (秒)
	// 		gracefully 	是否以优雅停机的形式关闭
	// 		stopped 是否已经停止该模块，错误的汇报将导致loader无法准确判断模块状态
	// 		err 异常
	Stop(maxWaitTime time.Duration) (gracefully, stopped bool, err error)
}

// 包裹原始Starter做未来拓展
type starterWrapper struct {
	// 状态 0=未启动 1=已启动 -1=已停止
	status  StarterStatus
	starter Starter
}

// 获取Starter名称
func (s *starterWrapper) getStarterName() string {
	setting := s.starter.Setting()
	if setting != nil && setting.starterName != "" {
		return setting.starterName
	}
	return "unnamed"
}

type starterWrappers []*starterWrapper

// find 获取指定名称的Starter
func (s *starterWrappers) find(starterName string) *starterWrapper {
	for _, wrapper := range *s {
		if wrapper.starter.Setting() != nil && wrapper.starter.Setting().starterName == starterName {
			return wrapper
		}
	}
	return nil
}

// 检查是否所有Setting均已配置
func (s *starterWrappers) checkSetting() bool {
	for _, v := range *s {
		if v.starter.Setting() == nil {
			return false
		}
	}
	return true
}

// 未启动的组件名称
func (s *starterWrappers) stoppedStarters() []string {
	starterNames := make([]string, 0)
	for _, v := range *s {
		if v.status != StarterStatusStarted {
			starterNames = append(starterNames, v.getStarterName())
		}
	}
	return starterNames
}

// Setting 卸载模块时对应的配置
// 注意	直接执行Unload函数，卸载配置将忽略，执行按照加载顺序卸载
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

	// 是否允许该模块异步卸载 (适用于starterLoader执行按设置卸载模块)
	// 如果使用异步卸载，starterLoader将不等待该模块的卸载反馈直接执行后续操作
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
		wrappers := make(starterWrappers, 0, len(starters))
		for _, v := range starters {
			wrappers = append(wrappers, &starterWrapper{
				starter: v,
			})
		}
		loader = &StarterLoader{
			starters: &wrappers,
		}
	})
	return loader
}

// AddStarter 动态添加一个或多个Starter组件。
//
// 新增组件会追加到注册列表末尾，因此启动顺序仍遵循先注册先启动。
func (s *StarterLoader) AddStarter(starters ...Starter) {
	defer s.Mutex.Unlock()
	s.Mutex.Lock()
	s.ensureStarters()
	if len(*s.starters) == 0 {
		*s.starters = make([]*starterWrapper, 0)
	}
	newStarterWrappers := coll.SliceCollect(starters, func(item Starter) *starterWrapper {
		return &starterWrapper{
			starter: item,
		}
	})
	v := append(*s.starters, newStarterWrappers...)
	s.starters = &v
}

// Start 按注册顺序启动所有未启动的Starter组件。
func (s *StarterLoader) Start() error {
	defer s.Mutex.Unlock()
	s.Mutex.Lock()
	s.ensureStarters()
	if len(*s.starters) == 0 {
		return ErrMissStarters
	}
	for _, wrapper := range *s.starters {
		if err := start(wrapper); err != nil {
			return err
		}
	}
	return nil
}

// StartStarter 启动指定名称且尚未启动的Starter组件。
func (s *StarterLoader) StartStarter(starterName string) error {
	defer s.Mutex.Unlock()
	s.Mutex.Lock()
	s.ensureStarters()
	if len(*s.starters) == 0 {
		return ErrNoStarter
	}
	wrapper := s.starters.find(starterName)
	if wrapper == nil {
		return fmt.Errorf("%w: %s", ErrUnknownStarterName, starterName)
	}
	return start(wrapper)
}

// StopAllBySetting 按Setting中的停止配置停止所有Starter组件。
//
// 停止优先级由Setting.stopPriority决定，值越小越优先停止。
func (s *StarterLoader) StopAllBySetting(allMaxWaitTime ...time.Duration) ([]*StopResult, error) {
	defer s.Mutex.Unlock()
	s.Mutex.Lock()
	s.ensureStarters()
	if len(*s.starters) == 0 {
		return nil, ErrNoStarter
	}
	if !s.starters.checkSetting() {
		return nil, ErrSomeStarterNoSetting
	}
	copied := coll.SliceCollect(*s.starters, func(item *starterWrapper) *starterWrapper {
		return item
	})
	coll.SliceSort(copied, func(e *starterWrapper) int {
		return int(e.starter.Setting().stopPriority)
	})
	stopResult := make([]*StopResult, 0)
	var wg sync.WaitGroup
	wg.Add(len(*s.starters))
	var mu sync.Mutex
	go func() {
		coll.SliceForEachAll(copied, func(wrapper *starterWrapper) {
			setting := wrapper.starter.Setting()
			if !setting.stopAllowAsync {
				result := stop(wrapper, setting.stopMaxWaitTime)
				mu.Lock()
				stopResult = append(stopResult, result)
				wg.Done()
				mu.Unlock()
			} else {
				go func(starterWrapper *starterWrapper) {
					defer wg.Done()
					result := stop(starterWrapper, starterWrapper.starter.Setting().stopMaxWaitTime)
					mu.Lock()
					stopResult = append(stopResult, result)
					mu.Unlock()
				}(wrapper)
			}
		})
	}()
	if len(allMaxWaitTime) > 0 {
		allStopDone := make(chan struct{})
		go func() {
			wg.Wait()
			close(allStopDone)
		}()
		select {
		case <-allStopDone:
			return stopResult, nil
		case <-time.After(allMaxWaitTime[0]):
			return stopResult, ErrStopAllTimeout
		}
	} else {
		wg.Wait()
	}
	return stopResult, nil
}

// StoppedStarters 返回所有未处于Started状态的Starter组件名称。
func (s *StarterLoader) StoppedStarters() []string {
	defer s.Mutex.Unlock()
	s.Mutex.Lock()
	s.ensureStarters()
	if len(*s.starters) == 0 {
		return nil
	}
	return s.starters.stoppedStarters()
}

// StopAllByRegisteredOrder 按注册顺序停止所有Starter组件，并忽略Setting中的停止配置。
func (s *StarterLoader) StopAllByRegisteredOrder(maxWaitTime time.Duration) ([]*StopResult, error) {
	defer s.Mutex.Unlock()
	s.Mutex.Lock()
	s.ensureStarters()
	if len(*s.starters) == 0 {
		return nil, ErrNoStarter
	}
	stopResult := make([]*StopResult, 0)
	for _, wrapper := range *s.starters {
		stopResult = append(stopResult, stop(wrapper, maxWaitTime))
	}
	return stopResult, nil
}

// StopStarter 停止指定名称的Starter组件。
func (s *StarterLoader) StopStarter(starterName string, maxWaitTime time.Duration) (*StopResult, error) {
	defer s.Mutex.Unlock()
	s.Mutex.Lock()
	s.ensureStarters()
	if len(*s.starters) == 0 {
		return nil, ErrNoStarterSet
	}
	wrapper := s.starters.find(starterName)
	if wrapper == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnknownStarterName, starterName)
	}
	return stop(wrapper, maxWaitTime), nil
}

// ensureStarters 确保Loader内部列表已初始化，避免空Loader使用时发生panic。
func (s *StarterLoader) ensureStarters() {
	if s.starters == nil {
		wrappers := make(starterWrappers, 0)
		s.starters = &wrappers
	}
}

// 启动指定的模块 如果已启动则忽略
func start(wrapper *starterWrapper) error {
	if wrapper.status != StarterStatusStarted {
		starter := wrapper.starter
		setting := starter.Setting()
		starterName := wrapper.getStarterName()
		if wrapper.status == StarterStatusStopped && (setting == nil || !setting.allowRestart) {
			return fmt.Errorf("%w: %s", ErrStarterRestartDisabled, starterName)
		}
		current := time.Now()
		logger.Logrus().Traceln(starterName, "starting now...")
		instance, err := starter.Start()
		if err != nil {
			logger.Logrus().WithError(err).Errorln(starterName, "start failed with error:", err)
			return err
		}
		if setting != nil && setting.initHandler != nil {
			// 执行初始化方法
			setting.initHandler(instance)
		}
		logger.Logrus().Traceln(starterName, "started successful cost:", time.Since(current))
		wrapper.status = StarterStatusStarted
	}
	return nil
}

// 停止指定的模块
func stop(wrapper *starterWrapper, maxWaitTime time.Duration) *StopResult {
	starterName := wrapper.getStarterName()
	if wrapper.status != StarterStatusStarted {
		return &StopResult{StarterName: starterName, Error: ErrStarterNotStarted}
	}
	starter := wrapper.starter
	current := time.Now()
	logger.Logrus().Traceln(starterName, "stopping now...")
	gracefully, stopped, err := starter.Stop(maxWaitTime)
	if err != nil {
		logger.Logrus().WithError(err).Errorln(starterName, "stop failed with error", err)
	} else {
		logger.Logrus().Traceln(starterName, "stopped successful cost:", time.Since(current))
	}
	if stopped {
		wrapper.status = StarterStatusStopped
	}
	return &StopResult{
		StarterName: starterName,
		Error:       err,
		Gracefully:  gracefully,
		Stopped:     stopped,
	}
}
