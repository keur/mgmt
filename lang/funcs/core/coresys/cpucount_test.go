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
		for {
			select {
			case count := <- output:
				fmt.Println(count)
				break
			}
		}
	}()
	// Now start the stream
	fact.Stream()


	err = fact.Close()
	if err != nil {
		t.Errorf("Could not close CPUCountFact")
		return
	}

}
