# starter-parent

go framework root module

用于管理符合Starter模块定义的所有组件，提供统一的启动/停止行为控制，支持主程序不退出的情况直接控制Starter状态

---

#### 功能说明

该模块用于定义和管理组件行为，当一个模块实现Starter接口后，它将可以托管给loader进行统一调度

顶层定义Starter接口

```go
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
```

定义组件

```go
// redis module
type redis struct {
}

func (r redis) Setting() *parent.Setting {
return parent.NewSettings("redis", 3, true, time.Second*3, nil)
}

func (r redis) Start() (interface{}, error) {
time.Sleep(time.Second)
return &redis{}, nil
}

func (r redis) Stop(maxWaitTime time.Duration) (gracefully bool, stopped bool, err error) {
ctx, cancelFunc := context.WithCancel(context.Background())
go func () {
defer cancelFunc()
time.Sleep(time.Second)
}()
select {
case <-time.After(maxWaitTime):
return false, true, errors.New("timeout")
case <-ctx.Done():
return true, true, err
}
}
```

统一管理

```go
func TestStartAndStop(t *testing.T) {
loader := parent.NewStarterLoader(starters)
err := loader.Start()
if err != nil {
println(err)
return
}
err = loader.Start() // 重复启动

result, err := loader.Stop(time.Second)
if err != nil {
println(err)
}
showStopResult(result)
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
 mongo  | 21       