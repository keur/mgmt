
package socketset

import (
	"testing"

	"golang.org/x/sys/unix"
)

// test cases for socketSet.fdSet()
var fdSetTests = []struct {
	in  *SocketSet
	out *unix.FdSet
}{
	{
		&SocketSet{
			fdEvents: 3,
			fdPipe:   4,
		},
		&unix.FdSet{
			Bits: [16]int64{0x18}, // 11000
		},
	},
	{
		&SocketSet{
			fdEvents: 12,
			fdPipe:   8,
		},
		&unix.FdSet{
			Bits: [16]int64{0x1100}, // 1000100000000
		},
	},
	{
		&SocketSet{
			fdEvents: 9,
			fdPipe:   21,
		},
		&unix.FdSet{
			Bits: [16]int64{0x200200}, // 1000000000001000000000
		},
	},
}

// test socketSet.fdSet()
func TestFdSet(t *testing.T) {
	for _, test := range fdSetTests {
		result := test.in.fdSet()
		if *result != *test.out {
			t.Errorf("fdSet test wanted: %b, got: %b", *test.out, *result)
		}
	}
}

// test cases for socketSet.nfd()
var nfdTests = []struct {
	in  *SocketSet
	out int
}{
	{
		&SocketSet{
			fdEvents: 3,
			fdPipe:   4,
		},
		5,
	},
	{
		&SocketSet{
			fdEvents: 8,
			fdPipe:   4,
		},
		9,
	},
	{
		&SocketSet{
			fdEvents: 90,
			fdPipe:   900,
		},
		901,
	},
}

// test socketSet.nfd()
func TestNfd(t *testing.T) {
	for _, test := range nfdTests {
		result := test.in.nfd()
		if result != test.out {
			t.Errorf("nfd test wanted: %d, got: %d", test.out, result)
		}
	}
}
