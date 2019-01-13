// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package coresys

import (
	"fmt"
	"github.com/purpleidea/mgmt/lang/funcs/facts"
	"github.com/purpleidea/mgmt/lang/types"
	"strings"
	"strconv"
	"io/ioutil"
	"regexp"

	"github.com/purpleidea/mgmt/lib/socketset"
	"golang.org/x/sys/unix"

	errwrap "github.com/pkg/errors"
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


// Init run startup code for this fact. Initializes the closeChan
// and sets the facts.Init variable
func (obj *CPUCountFact) Init(init *facts.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream returns the changing values that this fact has over time.
// It will first poll sysfs to get the initial cpu count, and then
// receieves UEvents from the kernel as CPUs are added/removed.
func (obj CPUCountFact) Stream() error {
	defer close(obj.init.Output) // Signal when we're done

	ss, err := socketset.NewSocketSet(rtmGrps, socketFile, unix.NETLINK_KOBJECT_UEVENT)
	if err != nil {
		return errwrap.Wrapf(err, "error creating socket set")
	}

	eventChan := make(chan *socketset.UEvent) // updated when we receive uevent
	defer close(eventChan)

	// Start waiting for kernel to poke us about new
	// device changes on the system
	go func() {
		defer ss.Shutdown()
		defer ss.Close()
		for {
			event, _ := ss.ReceiveUEvent()
			//fmt.Printf("Sending %s\n", event.Data["SEQNUM"])
			// TODO: use struct with error field
			select {
			case eventChan <- event:
			default: // don't block
			}
		}
	}()

	startChan := make(chan struct{})
	close(startChan) // trigger the first event
	var cpuCount int64
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
		break
	case <- obj.closeChan:
		return nil
	}

	for {
		select {
		case event, ok := <- eventChan:
			if !ok {
				return nil
			}
			//fmt.Printf("Receiving %s\n", event.Data["SEQNUM"])
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

// Close runs cleanup code for the fact and turns off the Stream.
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

// When we receieve a udev event, we filter only those that
// indicate a CPU is being added or removed.
// TODO: instead of checking for added/removed CPUS, check for online/offline?
func processUdev(event *socketset.UEvent) (int64, bool) {
	if event.Subsystem != "cpu" {
		return 0, false
	}
	// is this a valid cpu path in sysfs?
	m, err := regexp.MatchString(cpuDevpathRegex, event.Devpath)
	if !m || err != nil {
		// TODO: log error?
		return 0, false
	}
	if event.Action == "add" {
		return 1, true
	} else if event.Action == "remove" {
		return -1, true
	} else {
		return 0, false
	}
}
