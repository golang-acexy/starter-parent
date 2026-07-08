# starter-parent

`starter-parent` 是 golang-acexy starter/cloud 生态的组件生命周期管理模块。

该模块定义统一的 `Starter` 接口和全局唯一的 `StarterLoader`，用于托管各类 `starter-*` 组件，例如 gin、gorm、redis、mongo、grpc、cron、websocket 等。业务项目或 `cloud-*` 模块可以组合多个 starter，并统一交给 parent 完成启动、停止和动态控制。

## 设计定位

golang-acexy 的模块组织参考 Spring Boot + Spring Cloud 生态：

- `starter-*`：封装单一能力组件，例如数据库、缓存、Web 服务、定时任务。
- `cloud-*`：在代码层组合多个 starter，提供套餐式能力封装。
- `starter-parent`：定义所有 starter 的生命周期契约，并提供统一 Loader 管理入口。

同一应用进程中应只初始化一个 `StarterLoader`。所有 starter 应注册到该 Loader，由它统一编排生命周期。

## 安装

当前模块声明的 Go 版本为 `1.25.8`。

```bash
go get github.com/golang-acexy/starter-parent
```

## Starter 接口

```go
type Starter interface {
    Setting() *Setting
    Start() (any, error)
    Stop(maxWaitTime time.Duration) (gracefully, stopped bool, err error)
}
```

- `Setting`：返回组件名称、停止优先级、是否异步停止、停止超时时间和初始化回调。
- `Start`：启动组件，返回组件实例；启动成功后 Loader 会执行 `initHandler`。
- `Stop`：停止组件，组件自身负责根据 `maxWaitTime` 实现优雅停机。

## Loader 使用

```go
loader := parent.InitStarterLoader([]parent.Starter{
    ginStarter,
    redisStarter,
    gormStarter,
})

if err := loader.Start(); err != nil {
    panic(err)
}
```

`InitStarterLoader` 初始化或返回全局唯一 Loader。首次调用时注册传入的 starter；后续需要增加组件时使用 `AddStarter`。

```go
loader.AddStarter(cronStarter)
_ = loader.StartStarter("cron")
```

## 启动与停止顺序

启动顺序由注册方决定：先注册，先启动。`Start` 会按注册顺序启动所有未启动的组件，`StartStarter` 可在进程运行期间启动指定组件。

停止有两种方式：

- `StopAllByRegisteredOrder(maxWaitTime)`：按注册顺序停止，忽略 `Setting` 中的停止配置。
- `StopAllBySetting(allMaxWaitTime...)`：按 `Setting.stopPriority` 停止，值越小越优先；允许配置异步停止。

## 推荐停止优先级

| Module | Priority | Async |
| --- | ---: | :---: |
| nacos | 0 | false |
| gin | 1 | false |
| grpc | 1 | false |
| websocket | 1 | false |
| cron | 10 | false |
| redis | 19 | true |
| gorm | 20 | true |
| mongo | 21 | true |

## 常用 API

- `InitStarterLoader(starters)`：初始化或返回全局 Loader。
- `AddStarter(starters...)`：动态追加组件。
- `Start()`：启动所有未启动组件。
- `StartStarter(name)`：启动指定组件。
- `StopAllByRegisteredOrder(maxWaitTime)`：按注册顺序停止全部组件。
- `StopAllBySetting(...)`：按停止配置停止全部组件。
- `StopStarter(name, maxWaitTime)`：停止指定组件。
- `StoppedStarters()`：查看所有未处于启动状态的组件名称。
