package declaration

import (
	"errors"
	"strconv"
)

type Module struct {
	moduleLoaders []ModuleLoader
}

// ModuleConfig 卸载模块时对应的配置
// 注意	直接执行Unload函数，卸载配置将忽略，执行按照加载顺序反向卸载
type ModuleConfig struct {

	// 模块名称
	ModuleName string

	// 卸载时优先级，权重越小，优先级越高
	// 注意，相同的优先级会导致不稳定排序出现不稳定的同优先级先后顺序
	UnregisterPriority uint

	// 是否允许该模块异步卸载
	// true	执行异步卸载，触发卸载后不立即等待卸载结果
	UnregisterAllowSync bool

	// 等待优雅停机的最大时间 (秒)
	UnregisterMaxWaitSeconds uint
}

// ModuleLoader 声明starter加载的通用接口
type ModuleLoader interface {

	// ModuleConfig 设置卸载模块时配置
	ModuleConfig() *ModuleConfig

	// Register  声明模块的注册启动方法
	Register(interceptor *func(instance interface{})) error

	// Interceptor 用于执行Register时，获取原始模块实例，完成更多参数初始化 这是一个可选的实现
	// return	func instance 通常是一个模块对应的原始实例，以便于通过interceptor初始化原始模块的更多参数
	Interceptor() *func(instance interface{})

	// Unregister 声明模块的卸载关闭方法 函数会阻塞直到停机完成
	// param	maxWaitSeconds 等待优雅停机的最大时间 (秒)
	// return	gracefully 	是否以优雅停机的形式关闭
	Unregister(maxWaitSeconds uint) (gracefully bool, err error)
}

// Load 依次启动每个模块
// 采用同步的模式，仅当上一个模块启动正常后执行后续启动
func (m *Module) Load(loaders []ModuleLoader) error {
	if len(loaders) != 0 {
		var err error
		for _, loader := range loaders {
			err = loader.Register(loader.Interceptor())
			if err != nil {
				return err
			}
		}
		m.moduleLoaders = loaders
		return nil
	} else {
		return errors.New("nil module loader")
	}
}

// Unload 依次卸载每个模块 仅在上一个模块成功卸载后处理下一个 忽略UnregisterConfig配置
// param	maxWaitSeconds 等待优雅停机的最大时间(秒) 该时间将分别作用于每个模块
// return	返回每个模块名对应是否优雅停机结果
func (m *Module) Unload(maxWaitSeconds uint) (map[string]bool, error) {
	shutdownResult := make(map[string]bool, len(m.moduleLoaders))
	for index, loader := range m.moduleLoaders {
		gracefully, err := loader.Unregister(maxWaitSeconds)
		if err == nil {
			if loader.ModuleConfig() == nil || loader.ModuleConfig().ModuleName == "" {
				shutdownResult["unnamed "+strconv.Itoa(index)] = gracefully
			} else {
				shutdownResult[loader.ModuleConfig().ModuleName] = gracefully
			}
		} else {
			return shutdownResult, err
		}
	}
	return shutdownResult, nil
}

// UnloadByConfig 根据配置规则卸载模块，如果未配置config，将自动使用默认配置进行卸载
// 默认配置 优先级最低(且不保证顺序) 同步卸载 最大优雅停机等待时机10s
func (m *Module) UnloadByConfig(maxWaitSeconds uint) (map[string]bool, error) {
	for _, loader := range m.moduleLoaders {
		config := loader.ModuleConfig()
		if config == nil {

		}
	}
	return nil, nil
}
