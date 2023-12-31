package declaration

import (
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"
)

type Module1 struct {
}

func (Module1) ModuleConfig() *ModuleConfig {
	return &ModuleConfig{UnregisterPriority: 1, UnregisterAllowAsync: true, ModuleName: "module1"}
}

func (Module1) Register(interceptor *func(instance interface{})) error {
	return nil
}

func (Module1) Interceptor() *func(instance interface{}) {
	return nil
}

func (Module1) Unregister(maxWaitSeconds uint) (gracefully bool, err error) {
	time.Sleep(time.Second * 3)
	return true, nil
}

type Module2 struct {
}

func (Module2) ModuleConfig() *ModuleConfig {
	return &ModuleConfig{ModuleName: "module2", UnregisterPriority: 0, UnregisterAllowAsync: true}
}

func (Module2) Register(interceptor *func(instance interface{})) error {
	return nil
}

func (Module2) Interceptor() *func(instance interface{}) {
	return nil
}

func (Module2) Unregister(maxWaitSeconds uint) (gracefully bool, err error) {
	time.Sleep(time.Second * 1)
	return false, nil
}

type Module3 struct {
}

func (Module3) ModuleConfig() *ModuleConfig {
	return nil
}

func (Module3) Register(interceptor *func(instance interface{})) error {
	return nil
}

func (Module3) Interceptor() *func(instance interface{}) {
	return nil
}

func (Module3) Unregister(maxWaitSeconds uint) (gracefully bool, err error) {
	time.Sleep(time.Second * 2)
	return false, errors.New("ERROR")
}

func TestSortModuleByUnregisterPriority(t *testing.T) {
	modules := []ModuleLoader{Module1{}, Module3{}, Module2{}}

	for _, m := range modules {
		v := checkModuleConfig(m.ModuleConfig())
		fmt.Println(v.ModuleName, v.UnregisterPriority)
	}
	sort.Sort(sortedModuleByUnregisterPriority(modules))
	fmt.Println("sorted")
	for _, m := range modules {
		v := checkModuleConfig(m.ModuleConfig())
		fmt.Println(v.ModuleName, v.UnregisterPriority)
	}
}

func TestLoadAndUnload(t *testing.T) {
	m := Module{ModuleLoaders: []ModuleLoader{Module1{}, Module3{}, Module2{}}}
	err := m.Load()
	if err != nil {
		fmt.Printf("%+v\n", err)
		return
	}

	time.Sleep(2 * time.Second)

	st := time.Now().UnixMilli()

	//result := m.Unload(10)
	//fmt.Printf("%+v\n cost %+d \n", result, time.Now().UnixMilli()-st)

	result := m.UnloadByConfig()
	fmt.Printf("%+v\n cost %+d \n", result, time.Now().UnixMilli()-st)
}
