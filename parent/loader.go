package parent

import (
	"errors"
	"github.com/acexy/golang-toolkit/logger"
	"github.com/acexy/golang-toolkit/util/coll"
	"sort"
	"sync"
	"time"
)

var loader *StarterLoader
var loaderOnce sync.Once

type StarterLoader struct {
	sync.Mutex
	starters *starterWrappers
}

type Starter interface {

	// Setting 模块设置
	Setting() *Setting

	// Start 模块注册方法 启动顺序按照注册的starter顺序依次启动
	Start() (interface{}, error)

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
	status  int8
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
		if v.status != 1 {
			starterNames = append(starterNames, v.getStarterName())
		}
	}
	return starterNames
}

// 实现Sort接口

func (s *starterWrappers) Len() int {
	return len(*s)
}

func (s *starterWrappers) Swap(i, j int) {
	(*s)[i], (*s)[j] = (*s)[j], (*s)[i]
}

func (s *starterWrappers) Less(i, j int) bool {
	setting1 := (*s)[i].starter.Setting()
	setting2 := (*s)[j].starter.Setting()
	return setting1.stopPriority < setting2.stopPriority
}

// Setting 卸载模块时对应的配置
// 注意	直接执行Unload函数，卸载配置将忽略，执行按照加载顺序卸载
type Setting struct {

	// 模块名称
	starterName string

	// 组件在初始化时执行指定的初始化方法 instance为各个组件的原始实例，由自模块控制，执行时机为执行Starter.Register成功后
	initHandler func(instance interface{})

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
func NewSetting(starterName string, stopPriority uint, stopAllowAsync bool, stopMaxWaitTime time.Duration, initHandler func(instance interface{})) *Setting {
	return &Setting{
		starterName:     starterName,
		stopPriority:    stopPriority,
		stopAllowAsync:  stopAllowAsync,
		stopMaxWaitTime: stopMaxWaitTime,
		initHandler:     initHandler,
	}
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

// NewStarterLoader 创建一个模块加载器
func NewStarterLoader(starters []Starter) *StarterLoader {
	loaderOnce.Do(func() {
		if len(starters) == 0 {
			loader = &StarterLoader{}
		} else {
			if loader == nil {
				wrappers := make([]*starterWrapper, len(starters))
				for i, v := range starters {
					wrappers[i] = &starterWrapper{
						starter: v,
					}
				}
				loader = &StarterLoader{
					starters: (*starterWrappers)(&wrappers),
				}
			}
		}
	})
	return loader
}

// AddStarter 添加一个模块
func (s *StarterLoader) AddStarter(starter Starter) {
	defer s.Mutex.Unlock()
	s.Mutex.Lock()
	if len(*s.starters) == 0 {
		*s.starters = make([]*starterWrapper, 0)
	}
	v := append(*s.starters, &starterWrapper{
		starter: starter,
	})
	s.starters = &v
}

// Start 启动所有未启动的模块 按starter加载顺序
func (s *StarterLoader) Start() error {
	defer s.Mutex.Unlock()
	s.Mutex.Lock()
	if len(*s.starters) == 0 {
		return errors.New("no starter")
	}
	for _, wrapper := range *s.starters {
		if err := start(wrapper); err != nil {
			return err
		}
	}
	return nil
}

// StartStarter 启动指定未启动的模块
func (s *StarterLoader) StartStarter(starterName string) error {
	defer s.Mutex.Unlock()
	s.Mutex.Lock()
	if len(*s.starters) == 0 {
		return errors.New("no starter")
	}
	wrapper := s.starters.find(starterName)
	if wrapper == nil {
		return errors.New("unknown starterName: " + starterName)
	}
	return start(wrapper)
}

// StopBySetting 按照卸载配置停止所有模块
func (s *StarterLoader) StopBySetting(maxWaitTime ...time.Duration) ([]*StopResult, error) {
	defer s.Mutex.Unlock()
	s.Mutex.Lock()
	if len(*s.starters) == 0 {
		return nil, errors.New("no starter")
	}
	if !s.starters.checkSetting() {
		return nil, errors.New("some starter has no setting")
	}
	var wg sync.WaitGroup
	wg.Add(len(*s.starters))

	var copied starterWrappers = make([]*starterWrapper, len(*s.starters))
	for i, v := range *s.starters {
		copied[i] = v
	}

	sort.Sort(&copied)
	asyncStop := coll.SliceFilter(copied, func(item **starterWrapper) bool {
		return (*item).starter.Setting().stopAllowAsync
	})
	syncStop := coll.SliceComplement(copied, asyncStop, func(item1, item2 **starterWrapper) bool {
		return (*item1).getStarterName() == (*item2).getStarterName()
	})

	stopResult := make([]*StopResult, 0)

	var mu sync.Mutex
	// 启动同步卸载
	go func() {
		for _, wrapper := range syncStop {
			setting := wrapper.starter.Setting()
			result := stop(wrapper, setting.stopMaxWaitTime)
			mu.Lock()
			stopResult = append(stopResult, result)
			mu.Unlock()
			wg.Done()
		}
	}()

	for _, wrapper := range asyncStop {
		setting := wrapper.starter.Setting()
		go func(starterWrapper *starterWrapper) {
			defer wg.Done()
			result := stop(starterWrapper, setting.stopMaxWaitTime)
			mu.Lock()
			stopResult = append(stopResult, result)
			mu.Unlock()
		}(wrapper)
	}

	if len(maxWaitTime) > 0 {
		allStopDone := make(chan struct{})
		go func() {
			wg.Wait()
			close(allStopDone)
		}()
		select {
		case <-allStopDone:
			return stopResult, nil
		case <-time.After(maxWaitTime[0]):
			return nil, errors.New("stop timeout")
		}
	} else {
		wg.Wait()
	}
	return stopResult, nil
}

// StoppedStarters 未启动的模块名
func (s *StarterLoader) StoppedStarters() []string {
	defer s.Mutex.Unlock()
	s.Mutex.Lock()
	if len(*s.starters) == 0 {
		return nil
	}
	return s.starters.stoppedStarters()
}

// Stop 按starter加载顺序停止所有模块 忽略卸载配置
func (s *StarterLoader) Stop(maxWaitTime time.Duration) ([]*StopResult, error) {
	defer s.Mutex.Unlock()
	s.Mutex.Lock()
	if len(*s.starters) == 0 {
		return nil, errors.New("no starter")
	}
	stopResult := make([]*StopResult, 0)
	for _, wrapper := range *s.starters {
		stopResult = append(stopResult, stop(wrapper, maxWaitTime))
	}
	return stopResult, nil
}

// StopStarter 停止指定的模块
func (s *StarterLoader) StopStarter(starterName string, maxWaitTime time.Duration) (*StopResult, error) {
	defer s.Mutex.Unlock()
	s.Mutex.Lock()
	if len(*s.starters) == 0 {
		return nil, errors.New("no starter")
	}
	wrapper := s.starters.find(starterName)
	if wrapper == nil {
		return nil, errors.New("unknown starterName: " + starterName)
	}
	return stop(wrapper, maxWaitTime), nil
}

// 启动指定的模块 如果已启动则忽略
func start(wrapper *starterWrapper) error {
	if wrapper.status != 1 {
		starter := wrapper.starter
		setting := starter.Setting()
		starterName := wrapper.getStarterName()
		now := time.Now().UnixMilli()
		instance, err := starter.Start()
		if err != nil {
			logger.Logrus().WithError(err).Errorln("starterName:", starterName, "start failed with error:", err)
			return err
		}
		if setting != nil && setting.initHandler != nil {
			// 执行初始化方法
			setting.initHandler(instance)
		}
		logger.Logrus().Traceln("starterName:", starterName, "start successful cost:", time.Now().UnixMilli()-now, "ms")
		wrapper.status = 1
	}
	return nil
}

// 停止指定的模块
func stop(wrapper *starterWrapper, maxWaitTime time.Duration) *StopResult {
	starterName := wrapper.getStarterName()
	if wrapper.status != 1 {
		return &StopResult{StarterName: starterName, Error: errors.New("not started"), Gracefully: false}
	}
	starter := wrapper.starter
	now := time.Now().UnixMilli()
	logger.Logrus().Traceln("starterName:", starterName, "stopping now")
	gracefully, stopped, err := starter.Stop(maxWaitTime)
	if err != nil {
		logger.Logrus().WithError(err).Errorln("starterName:", starterName, "stop failed with error:", err)
	} else {
		logger.Logrus().Traceln("starterName:", starterName, "stop successful cost:", time.Now().UnixMilli()-now, "ms")
	}
	if stopped {
		wrapper.status = -1
	}
	return &StopResult{
		StarterName: starterName,
		Error:       err,
		Gracefully:  gracefully,
		Stopped:     stopped,
	}
}
