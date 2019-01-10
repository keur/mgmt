
package udev

import (
	"testing"
	"fmt"
)

func TestSocketSet(t *testing.T) {
	err := udev()
	if err != nil {
		fmt.Print(err)
	}
}
