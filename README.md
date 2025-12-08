# starter-parent

go framework root module

用于管理符合Starter模块定义的所有组件，提供统一的启动/停止行为控制，支持主程序不退出的情况直接控制Starter状态

---

#### 功能说明

该模块用于定义和管理组件行为，当一个模块实现Starter接口后，它将可以托管给loader进行统一调度顶层定义Starter接口

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

 module    | priority | async 
-----------|----------|-------
 nacos     | 0        | false 
 gin       | 1        | false 
 grpc      | 1        | false 
 websocket | 1        | false 
 cron      | 10       | false 
 redis     | 19       | true  
 grom      | 20       | true  
 mongo     | 21       | true  