package test

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang-acexy/starter-parent/parent"
	"testing"
	"time"
)

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
	go func() {
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

// gorm module
type gorm struct {
}

func (g gorm) Setting() *parent.Setting {
	return parent.NewSettings("gorm", 1, true, time.Second, func(instance interface{}) {
		_, ok := instance.(*gorm)
		if ok {
			fmt.Println("init invoker")
		}
	})
}

func (g gorm) Start() (interface{}, error) {
	return &gorm{}, nil
}

func (g gorm) Stop(maxWaitTime time.Duration) (gracefully bool, stopped bool, err error) {
	time.Sleep(time.Second)
	return true, true, err
}

// gin module
type gin struct {
}

func (g gin) Setting() *parent.Setting {
	return parent.NewSettings("gin", 2, true, time.Second, nil)
}

func (g gin) Start() (interface{}, error) {
	return &gin{}, nil
}

func (g gin) Stop(maxWaitTime time.Duration) (gracefully bool, stopped bool, err error) {
	return false, false, errors.New("something error")
}

var starters []parent.Starter

func init() {
	starters = []parent.Starter{
		&redis{},
		&gorm{},
		&gin{},
	}
}

func showStopResult(result []*parent.StopResult) {
	for _, v := range result {
		fmt.Printf("%+v\n", v)
	}
}

// Test

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

func TestStartAndStopBySetting(t *testing.T) {
	loader := parent.NewStarterLoader(starters)
	err := loader.Start()
	if err != nil {
		println(err)
		return
	}
	time.Sleep(time.Second * 3)
	result, err := loader.StopBySetting()
	if err != nil {
		println(err)
	}
	showStopResult(result)
}

func TestStarterControl(t *testing.T) {
	loader := parent.NewStarterLoader(starters)
	err := loader.Start()
	if err != nil {
		println(err)
		return
	}
	result, err := loader.StopStarter("gorm", time.Second)
	if err != nil {
		println(err)
	}
	showStopResult([]*parent.StopResult{result})
	fmt.Println(loader.NotStarted())
	_ = loader.Start()
	fmt.Println(loader.NotStarted())
}
