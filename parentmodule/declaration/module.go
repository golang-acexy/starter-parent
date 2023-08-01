package declaration

// ModuleLoader 声明starter加载的通用接口
type ModuleLoader interface {

	// Register  声明模块的注册启动方法
	// param
	//		interceptor 通常是一个模块对应的原始实例，以便于通过interceptor初始化原始模块的更多参数
	Register(interceptor *func(instance interface{})) error

	// Unregister 声明模块的卸载关闭方法 函数会阻塞直到停机完成
	// param
	// 		maxWaitSeconds 等待优雅停机的最大时间 (秒)
	// return
	//		gracefully 	是否以优雅停机的形式关闭
	Unregister(maxWaitSeconds uint) (gracefully bool, err error)
}
