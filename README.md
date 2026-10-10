# starter-parent

`starter-parent` is the lifecycle foundation of the Golang Acexy starter/cloud ecosystem. It defines the shared `Starter` contract and owns the process-wide `StarterLoader` used to register, start, stop, and coordinate infrastructure components.

## Ecosystem Role

The ecosystem follows a Spring Boot and Spring Cloud inspired component model while keeping dependency composition explicit in Go:

- `starter-*` modules wrap one infrastructure capability.
- `cloud-*` modules combine starters into higher-level application patterns.
- `starter-parent` provides the lifecycle contract shared by every starter.

One application process should initialize one loader. Components start in registration order and can stop by registration order or by their own shutdown settings.

## Requirements

- Go `1.26.7`

## Installation

```bash
go get github.com/golang-acexy/starter-parent
```

## Starter Contract

```go
type Starter interface {
    Setting() *Setting
    Start() (any, error)
    Stop(maxWaitTime time.Duration) (gracefully, stopped bool, err error)
}
```

- `Setting` defines the component name, restart policy, stop priority, asynchronous-stop policy, stop timeout, and initialization callback.
- `Start` initializes the component and returns its raw instance. The loader invokes the configured initialization callback after a successful start.
- `Stop` performs component-owned graceful shutdown and accurately reports whether the component stopped.

## Register and Start Components

```go
loader := parent.InitStarterLoader([]parent.Starter{
    gormStarter,
    redisStarter,
    ginStarter,
})

if err := loader.Start(); err != nil {
    panic(err)
}
```

`InitStarterLoader` initializes or returns the process-wide loader. Only the first call registers its input. Use `AddStarter` to append components later:

```go
loader.AddStarter(cronStarter)
if err := loader.StartStarter("Cron-Starter"); err != nil {
    panic(err)
}
```

Registered starters must be non-nil, provide a non-nil `Setting`, and use a non-empty unique name. Lifecycle operations return explicit validation errors when the registration set is invalid.

## Lifecycle Ordering

`Start` visits components in registration order. Register dependencies before consumers—for example, database and cache starters before HTTP or gRPC servers.

Two coordinated shutdown strategies are available:

- `StopAllByRegisteredOrder(maxWaitTime)` stops components in registration order and ignores individual stop settings.
- `StopAllBySetting(allMaxWaitTime...)` sorts by `Setting.StopPriority()`, supports asynchronous stops, and optionally limits the total wait time.

Asynchronous stops run concurrently while synchronous stops preserve priority order. Returned results always follow the sorted starter order. If the total timeout expires, unfinished entries are `nil`; background stops do not modify the returned snapshot or cancel when the wait expires.

Default standard-starter settings are:

| Module | Restart | Stop priority | Async stop |
| --- | :---: | ---: | :---: |
| Nacos | false | 0 | false |
| Gin | false | 1 | false |
| gRPC | false | 1 | false |
| WebSocket | true | 1 | false |
| Cron | false | 10 | false |
| Redis | false | 19 | true |
| GORM | false | 20 | true |
| MongoDB | false | 21 | true |

## Restart Policy

A component starts normally when it has never run. Starting it again while it is already running is idempotent at the loader level.

After a component has stopped successfully, another `Start` or `StartStarter` call is a restart. The loader permits that transition only when `Setting.AllowRestart()` is true; otherwise it returns `ErrStarterRestartDisabled` without calling the component's `Start` method. Among the standard starters, only `starter-websocket` currently enables restart.

## Common API

- `InitStarterLoader(starters)` initializes or returns the global loader.
- `AddStarter(starters...)` appends components to the registration list.
- `Start()` starts all components that are not currently running.
- `StartStarter(name)` starts one named component and enforces its restart policy.
- `StopStarter(name, maxWaitTime)` stops one named component.
- `StopAllByRegisteredOrder(maxWaitTime)` stops all components in registration order.
- `StopAllBySetting(...)` stops all components according to their settings.
- `StoppedStarters()` returns components not currently in the started state.

## Usage Constraints

- Prefer one application startup followed by one shutdown. Overlapping lifecycle calls are not readiness barriers; use one startup flow to preserve dependency order.
- Repeated initialization returns the existing loader and ignores new registration input.
- Supply valid registrations before startup. A startup error leaves earlier components running; invalid registrations can also prevent loader-based shutdown.
- A starter owns resource initialization, partial-start cleanup, and shutdown deadlines. The loader does not recover callback panics or force resource closure.
- Report `stopped` accurately and inspect every shutdown result as well as the method error. A batch timeout leaves unfinished entries `nil` and does not cancel their background work.
- Use `errors.Is` with the exported errors to distinguish lifecycle failures. `ErrMissStarters` and `ErrNoStarterSet` are aliases of `ErrNoStarter`.
