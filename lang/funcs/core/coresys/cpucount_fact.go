package coresys

import (
	"fmt"
	"github.com/purpleidea/mgmt/lang/funcs/facts"
	"github.com/purpleidea/mgmt/lang/types"
	"os"
	"strings"
	"strconv"
	"io/ioutil"
)


// CPUCountFact is a fact that returns the current CPU count
type CPUCountFact struct {
	init      *facts.Init
	closeChan chan struct{}
}


func init() {
	facts.ModuleRegister(moduleName, "cpu_count", func() facts.Fact { return &CPUCountFact{} }) // must register the fact and name
}


func (obj *CPUCountFact) Init(init *facts.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

func (obj CPUCountFact) Stream() error {
	startChan := make(chan struct{})
	close(startChan)

	for {
		select {
		case <- startChan:
			startChan = nil // disable
			_, err := os.Open("/proc/cpuinfo") // TODO: fix
			fmt.Println("polling cpuinfo")
			cpuCount, err := getCPUCountProc()
			if err != nil {
				// TODO: log?
				cpuCount = 0
			}
			obj.init.Output <- &types.IntValue{
				V: cpuCount,
			}
		case <- obj.closeChan:
			return nil
		}
	}
}

// Info returns static typing info about what the fact returns
func (obj *CPUCountFact) Info() *facts.Info {
	return &facts.Info{
		Output: types.NewType("int"),
	}
}

func (obj *CPUCountFact) Close() error {
	close(obj.closeChan)
	return nil
}

func getCPUCountProc() (int64, error) {
	dat, err := ioutil.ReadFile("/proc/cpuinfo")
	if err != nil {
		return -1, err
	}


	for _, line := range strings.Split(string(dat), "\n") {
		if strings.HasPrefix(line, "cpu cores") {
			s := strings.Split(line, ":")
			if len(s) != 2 {
				return 0, nil
			}
			cpus := strings.Trim(s[1], " ")
			cpusInt, err := strconv.ParseInt(cpus, 10, 64)
			if err != nil {
				return 0, nil
			}
			return cpusInt, nil
		}
	}
	return 0, nil
}
