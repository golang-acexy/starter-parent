# starter-parent

go framework root module

用于管理符合Starter模块定义的所有组件的自动启动/停止

---

#### 功能说明

该模块用于定义/管理/加载/卸载统一所有的组件行为，当一个模块实现Starter接口后，它将可以托管给loader进行统一系统启动加载和系统关闭时停止的行为

```go
type Starter interface {

	// Setting 模块设置
	Setting() *Setting

	// Register 模块注册方法 启动顺序按照接收到的starter顺序依次启动
	Register() (interface{}, error)

	// Unregister 声明模块的卸载关闭方法 模块应当已阻塞的形式实现
	// param	maxWaitSeconds 等待优雅停机的最大时间 (秒)
	// return	gracefully 	是否以优雅停机的形式关闭
	Unregister(maxWaitSeconds uint) (gracefully bool, err error)
}
```

支持功能

- 启动

  - 依次启动组件，反馈组件启动结果
  - 可在主程序不停止的情况下，启动指定的组件

- 停止

  - 按照Starter加载顺序依次停止组件，反馈组件卸载结果
  - 按照Starter卸载配置，按设置按权重依次卸载组件，反馈组件卸载结果
  - 可在主程序不停止的情况下，停止指定的组件

---

#### 默认模块卸载权重 (值越小优先级越高)

 module | priority 
--------|----------
 gin    | 0        
 grpc   | 0        
 nacos  | 1        
 cron   | 10       
 redis  | 19       
 grom   | 20      