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
	"testing"
)

var fact *CPUCountFact


func TestSimple(t *testing.T) {
	fact = &CPUCountFact{}

	output := make(chan types.Value)
	err := fact.Init(&facts.Init{
		Hostname: "mgmt",
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
				// TODO: right now this only supports the initial
				// reading from proc
				if count > 1 {
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
