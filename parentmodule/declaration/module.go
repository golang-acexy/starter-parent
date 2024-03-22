package declaration

import (
	"errors"
	"github.com/acexy/golang-toolkit/logger"
	"sort"
	"sync"
	"time"
)

type Module struct {
	ModuleLoaders []ModuleLoader
}

const (
	defaultConfigUnregisterPriority       = uint(9999)
	defaultConfigUnregisterMaxWaitSeconds = 20
)

// ShutdownResult 模块停止卸载结果
type ShutdownResult struct {
	ModuleName string
	Err        error
	Gracefully bool
}

// ModuleConfig 卸载模块时对应的配置
// 注意	直接执行Unload函数，卸载配置将忽略，执行按照加载顺序卸载
type ModuleConfig struct {

	// 模块名称
	ModuleName string

	// 注册模块的拦截器，用于获取原始模块的实例，进行拓展配置
	LoadInterceptor func(instance interface{})

	// 卸载时优先级，权重越小，优先级越高
	// 注意，相同的优先级会导致不稳定排序出现不稳定的同优先级先后顺序
	UnregisterPriority uint

	// 是否允许该模块异步卸载
	// true	执行异步卸载，触发卸载后不立即等待卸载结果
	UnregisterAllowAsync bool

	// 等待优雅停机的最大时间 (秒)
	UnregisterMaxWaitSeconds uint
}

type sortedModuleByUnregisterPriority []ModuleLoader

func (c sortedModuleByUnregisterPriority) Len() int {
	return len(c)
}

func (c sortedModuleByUnregisterPriority) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func (c sortedModuleByUnregisterPriority) Less(i, j int) bool {
	configI := c[i].ModuleConfig()
	configI = checkModuleConfig(configI)
	configJ := c[j].ModuleConfig()
	configJ = checkModuleConfig(configJ)
	return configI.UnregisterPriority < configJ.UnregisterPriority
}

// ModuleLoader 声明starter加载的通用接口
type ModuleLoader interface {

	// ModuleConfig 设置卸载模块时配置
	ModuleConfig() *ModuleConfig

	// Register  声明模块的注册启动方法
	Register() error

	// RawInstance 原始模块的实例
	RawInstance() interface{}

	// Unregister 声明模块的卸载关闭方法 函数会阻塞直到停机完成
	// param	maxWaitSeconds 等待优雅停机的最大时间 (秒)
	// return	gracefully 	是否以优雅停机的形式关闭
	Unregister(maxWaitSeconds uint) (gracefully bool, err error)
}

// Load 依次加载启动每个模块
// 采用同步的模式，仅当上一个模块启动正常后执行后续启动，任何模块的错误将中断并返回异常
func (m *Module) Load() error {
	if len(m.ModuleLoaders) != 0 {
		var err error
		for _, loader := range m.ModuleLoaders {
			var moduleName string
			if loader.ModuleConfig() == nil || loader.ModuleConfig().ModuleName == "" {
				moduleName = "unnamed"
			} else {
				moduleName = loader.ModuleConfig().ModuleName
			}
			t := time.Now().UnixMilli()
			instance := loader.RawInstance()
			if instance != nil {
				loadInterceptor := loader.ModuleConfig().LoadInterceptor
				if loadInterceptor != nil {
					loadInterceptor(instance)
				}
			}
			err = loader.Register()
			if err != nil {
				logger.Logrus().WithField("moduleName", moduleName).Errorln("load module error")
				return err
			}
			logger.Logrus().WithField("moduleName", moduleName).WithField("cost", time.Now().UnixMilli()-t).Traceln("load module success")
		}
		return nil
	} else {
		return errors.New("nil module loader")
	}
}

// Unload 依次卸载每个模块 仅在上一个模块成功卸载后处理下一个 忽略UnregisterConfig配置
// param	maxWaitSeconds 等待优雅停机的最大时间(秒) 该时间将分别作用于每个模块
// return	map[string]ShutdownResult
func (m *Module) Unload(maxWaitSeconds uint) []ShutdownResult {
	logger.Logrus().Traceln("uninstall modules one by one")
	shutdownResult := make([]ShutdownResult, len(m.ModuleLoaders))
	for index, loader := range m.ModuleLoaders {
		var moduleName string
		if loader.ModuleConfig() == nil || loader.ModuleConfig().ModuleName == "" {
			moduleName = "unnamed"
		} else {
			moduleName = loader.ModuleConfig().ModuleName
		}
		t := time.Now().UnixMilli()
		gracefully, err := loader.Unregister(maxWaitSeconds)

		if err == nil {
			shutdownResult[index] = ShutdownResult{
				ModuleName: moduleName,
				Gracefully: gracefully,
			}
			if gracefully {
				logger.Logrus().WithField("moduleName", moduleName).WithField("cost", time.Now().UnixMilli()-t).Traceln("doUnload module success")
			} else {
				logger.Logrus().WithField("moduleName", moduleName).WithField("cost", time.Now().UnixMilli()-t).Warnln("doUnload module not gracefully")
			}
		} else {
			shutdownResult[index] = ShutdownResult{
				ModuleName: moduleName,
				Err:        err,
			}
			logger.Logrus().WithField("moduleName", moduleName).WithField("cost", time.Now().UnixMilli()-t).WithError(err).Errorln("doUnload module error")
		}
	}
	return shutdownResult
}

// UnloadByConfig 根据配置规则卸载模块，如果未配置config，将自动使用默认配置进行卸载
// 默认配置： 优先级最低(且不保证顺序) 同步卸载 最大优雅停机等待时机20s
func (m *Module) UnloadByConfig() []ShutdownResult {
	logger.Logrus().Traceln("unload modules by unregisterPriority")
	var wait sync.WaitGroup
	wait.Add(len(m.ModuleLoaders))
	sort.Sort(sortedModuleByUnregisterPriority(m.ModuleLoaders)) // 按照权重重新分配关停顺序
	shutdownResult := make([]ShutdownResult, len(m.ModuleLoaders))
	for index, loader := range m.ModuleLoaders {
		var moduleName string
		if loader.ModuleConfig() == nil || loader.ModuleConfig().ModuleName == "" {
			moduleName = "unnamed"
		} else {
			moduleName = loader.ModuleConfig().ModuleName
		}
		shutdownResult[index] = ShutdownResult{ModuleName: moduleName}
		config := checkModuleConfig(loader.ModuleConfig())
		if config.UnregisterAllowAsync {
			go func(l ModuleLoader, r *ShutdownResult) {
				t := time.Now().UnixMilli()
				defer wait.Done()
				doUnload(l, r)
				if r.Err != nil {
					logger.Logrus().WithField("moduleName", moduleName).WithField("cost", time.Now().UnixMilli()-t).WithError(r.Err).Errorln("async unload module error")
				} else {
					if r.Gracefully {
						logger.Logrus().WithField("moduleName", moduleName).WithField("cost", time.Now().UnixMilli()-t).Traceln("async unload module success")
					} else {
						logger.Logrus().WithField("moduleName", moduleName).WithField("cost", time.Now().UnixMilli()-t).Warnln("async unload module not gracefully")
					}
				}
			}(loader, &shutdownResult[index])
		} else {
			result := &shutdownResult[index]
			t := time.Now().UnixMilli()
			doUnload(loader, result)
			if result.Err != nil {
				logger.Logrus().WithField("moduleName", moduleName).WithField("cost", time.Now().UnixMilli()-t).WithError(result.Err).Errorln("unload module error")
			} else {
				if result.Gracefully {
					logger.Logrus().WithField("moduleName", moduleName).WithField("cost", time.Now().UnixMilli()-t).Traceln("unload module success")
				} else {
					logger.Logrus().WithField("moduleName", moduleName).WithField("cost", time.Now().UnixMilli()-t).Warnln("unload module not gracefully")
				}
			}
			wait.Done()
		}
	}
	wait.Wait()
	logger.Logrus().Traceln("all module unloaded")
	return shutdownResult
}

func doUnload(loader ModuleLoader, shutdownResult *ShutdownResult) {
	gracefully, err := loader.Unregister(checkModuleConfig(loader.ModuleConfig()).UnregisterMaxWaitSeconds)
	if err == nil {
		shutdownResult.Gracefully = gracefully
	} else {
		shutdownResult.Err = err
	}
}

func checkModuleConfig(config *ModuleConfig) *ModuleConfig {
	if config == nil {
		config = &ModuleConfig{
			ModuleName:               "unnamed",
			UnregisterPriority:       defaultConfigUnregisterPriority,
			UnregisterAllowAsync:     false,
			UnregisterMaxWaitSeconds: defaultConfigUnregisterMaxWaitSeconds,
		}
		return config
	} else {
		if config.ModuleName == "" {
			config.ModuleName = "unnamed"
		}
		if config.UnregisterMaxWaitSeconds == 0 {
			config.UnregisterMaxWaitSeconds = defaultConfigUnregisterMaxWaitSeconds
		}
		return config
	}
}
