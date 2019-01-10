package coresys

import (
	"fmt"
	"github.com/purpleidea/mgmt/lang/funcs/facts"
	"github.com/purpleidea/mgmt/lang/types"
	"strings"
	"strconv"
	"io/ioutil"
	"regexp"

	errwrap "github.com/pkg/errors"
	netlink "github.com/purpleidea/mgmt/lib/udev"
)

const (
	rtmGrps = 0x1 // make me a multicast reciever
	socketFile = "pipe.sock"
	cpuDevpathRegex = "/devices/system/cpu/cpu[0-9]"
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
	defer close(obj.init.Output) // Signal when we're done

	ss, err := netlink.EventSocketSet(rtmGrps, socketFile)
	if err != nil {
		return errwrap.Wrapf(err, "error creating socket set")
	}
	defer ss.Close()
	defer ss.Shutdown()

	eventChan := make(chan *netlink.UEvent) // updated when we receive uevent

	// Start waiting for kernel to poke us about new
	// device changes on the system
	go func() error {
		for {
			event, err := ss.ReceiveUEvent()
			if err != nil {
				// TODO: log here instead?
				return errwrap.Wrapf(err, "error receiving uevent data")
			}
			// pass the new event
			eventChan <- event
		}
		return nil
	}()

	startChan := make(chan struct{})
	close(startChan) // trigger the first event
	for {
		var cpuCount int64 // NOTE: gets set to 0
		select {
		case <- startChan:
			startChan = nil // disable
			fmt.Println("polling cpuinfo")
			cpuCount, _ = initCPUCount()
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
		select {
		case event := <- eventChan:
			fmt.Println("udev reports cpu change")
			cpus, cpuEvent := processUdev(event)
			if cpuEvent {
				cpuCount += cpus
				obj.init.Output <- &types.IntValue {
					V: cpuCount,
				}
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

// initCPUCount looks in procfs to get the number of CPUs.
// This is just for initializing the fact, and should not be polled.
func initCPUCount() (int64, error) {
	var count int64
	dat, err := ioutil.ReadFile("/sys/devices/system/cpu/present") // TODO: change this to online?
	if err != nil {
		return 0, err
	}

	for _, line := range strings.Split(string(dat), ",") {
		cpuRange := strings.SplitN(line, "-", 2)
		if len(cpuRange) == 1 {
			count++
		} else if len(cpuRange) == 2 {
			lo, err := strconv.ParseInt(cpuRange[0], 10, 64)
			if err != nil {
				return 0, err
			}
			hi, err := strconv.ParseInt(strings.TrimRight(cpuRange[1], "\n"), 10, 64)
			if err != nil {
				return 0, err
			}
			count += hi - lo  + 1
		}
	}
	return count, nil
}

func processUdev(event *netlink.UEvent) (int64, bool) {
	if event.Subsystem != "cpu" {
		return 0, false
	}
	// is this a valid cpu path in sysfs?
	m, err := regexp.MatchString(cpuDevpathRegex, event.Devpath)
	if !m || err != nil {
		// TODO: log error?
		return 0, false
	}
	// TODO: check for ONLINE and OFFLINE?
	if event.Action == "ADD" {
		return 1, true
	} else if event.Action == "Remove" {
		return -1, true
	} else {
		return 0, false
	}
}
