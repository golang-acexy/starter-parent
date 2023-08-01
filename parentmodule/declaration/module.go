package declaration

import "errors"

// ModuleLoader 声明starter加载的通用接口
type ModuleLoader interface {

	// Register  声明模块的注册启动方法
	Register(interceptor *func(instance interface{})) error

	// Interceptor 用于执行Register时，获取原始模块实例，完成更多参数初始化 这是一个可选的实现
	// return
	//		func instance 通常是一个模块对应的原始实例，以便于通过interceptor初始化原始模块的更多参数
	Interceptor() *func(instance interface{})

	// Unregister 声明模块的卸载关闭方法 函数会阻塞直到停机完成
	// param
	// 		maxWaitSeconds 等待优雅停机的最大时间 (秒)
	// return
	//		gracefully 	是否以优雅停机的形式关闭
	Unregister(maxWaitSeconds uint) (gracefully bool, err error)
}

func Load(loaders []ModuleLoader) error {
	if len(loaders) == 0 {
		var err error
		for _, loader := range loaders {
			err = loader.Register(loader.Interceptor())
			if err != nil {
				return err
			}
		}
	}
	return errors.New("nil module loader")
}

func Unload() {

}
