package coresys

import (
	"fmt"
	"github.com/purpleidea/mgmt/lang/funcs/facts"
	"github.com/purpleidea/mgmt/lang/types"
	"testing"
)

var fact *CPUCountFact


func TestSimple(t *testing.T) {
	fact = &CPUCountFact{}

	output := make(chan types.Value)
	err := fact.Init(&facts.Init{
		Hostname: "test",
		Output: output,
	})
	if err != nil {
		t.Errorf("could not init CPUCountFact")
		return
	}

	// Set up the go function
	go func() {
		defer fact.Close()
		count := 0
	Loop:
		for {
			select {
			case cpus := <- output:
                fmt.Printf("CPUS: %d\n", cpus.Int())
				count++
				if count > 3 {
					break Loop
				}
			}
		}
	}()

	// Now start the stream
	err = fact.Stream()
	if err != nil {
		t.Error(err)
	}
}
