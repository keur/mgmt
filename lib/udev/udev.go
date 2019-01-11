package udev

import (
	"fmt"
	"os"
	"path"
	"golang.org/x/sys/unix"
	"bytes"
	"strings"
	"syscall"

	errwrap "github.com/pkg/errors"
)


// Shutdown closes the event file descriptor and unblocks receive by sending
// a message to the pipe file descriptor. It must be called before close, and
// should only be called once.
func (obj *SocketSet) Shutdown() error {
	// close the event socket so no more events are produced
	if err := unix.Close(obj.fdEvents); err != nil {
		return err
	}
	// send a message to the pipe to unblock select
	return unix.Sendto(obj.fdPipe, nil, 0, &unix.SockaddrUnix{
		Name: path.Join(obj.pipeFile),
	})
}

// Close closes the pipe file descriptor. It must only be called after
// shutdown has closed fdEvents, and unblocked receive. It should only be
// called once.
func (obj *SocketSet) Close() error {
	if err := unix.Unlink(obj.pipeFile); err != nil {
		return errwrap.Wrapf(err, "could not unbind %s", obj.pipeFile)
	}
	return unix.Close(obj.fdPipe)
}

// ReceiveBytes waits for bytes from fdEvents and parses them into a slice of
// netlink messages. It will block until an event is produced, or shutdown
// is called.
func (obj *SocketSet) ReceiveBytes() ([]byte, error) {
	// Select will return when any fd in fdSet (fdEvents and fdPipe) is ready
	// to read.
	_, err := unix.Select(obj.nfd(), obj.fdSet(), nil, nil, nil)
	if err != nil {
		// if a system interrupt is caught
		if err == unix.EINTR { // signal interrupt
			return nil, nil
		}
		return nil, errwrap.Wrapf(err, "error selecting on fd")
	}
	// receive the message from the netlink socket into b
	b := make([]byte, os.Getpagesize())
	n, _, err := unix.Recvfrom(obj.fdEvents, b, unix.MSG_DONTWAIT) // non-blocking receive
	if err != nil {
		// if fdEvents is closed
		if err == unix.EBADF { // bad file descriptor
			return nil, nil
		}
		return nil, errwrap.Wrapf(err, "error receiving messages")
	}
	// if we didn't get enough bytes for a header, something went wrong
	if n < unix.NLMSG_HDRLEN {
		return nil, fmt.Errorf("received short header")
	}
	b = b[:n] // truncate b to message length
	return b, nil
}

// ReceiveParsedNetlink is a wrapper around ReceiveBytes that returns a NetlinkMessage.
func (obj *SocketSet) ReceiveParsedNetlink() ([]syscall.NetlinkMessage, error) {
	msgBytes, err := obj.ReceiveBytes()
	if err != nil {
		return nil, err
	}
	// use syscall to parse, as func does not exist in x/sys/unix
	return syscall.ParseNetlinkMessage(msgBytes)
}

// UEvent struct which has attributes passed from a KOBJECT_NETWORK_UEVENT
type UEvent struct {
	// Default keys, as per https://github.com/torvalds/linux/blob/master/lib/kobject_uevent.c
	Action    string
	Devpath   string
	Subsystem string

	// Every other KV pair
	Data      map[string]string
}

// ReceiveUEvent is a wrapper around ReceiveBytes that returns a UEvent
func (obj *SocketSet) ReceiveUEvent() (*UEvent, error) {
	// TODO: can multiple events come in the same socket?
	event := &UEvent{Data: map[string]string{}}

	msgBytes, err := obj.ReceiveBytes()
	if err != nil {
		return nil, err
	}

	submsg := msgBytes[:]
	i := 0
Loop:
	for {
		submsg = submsg[i:]
		n := bytes.IndexByte(submsg, 0x0)
		if n == -1 {
			break Loop
		}
		i = n + 1

		attrLine := string(submsg[:n])
		split := strings.SplitN(attrLine, "=", 2)
		if len(split) < 2 {
			continue
		}
		switch split[0] {
		case "ACTION":
			event.Action = split[1]
		case "DEVPATH":
			event.Devpath = split[1]
		case "SUBSYSTEM":
			event.Subsystem = split[1]
		default:
			event.Data[split[0]] = split[1]
		}
	}

	return event, nil
}

// nfd returns one more than the highest fd value in the struct, for use as as
// the nfds parameter in select. It represents the file descriptor set maximum
// size. See man select for more info.
func (obj *SocketSet) nfd() int {
	if obj.fdEvents > obj.fdPipe {
		return obj.fdEvents + 1
	}
	return obj.fdPipe + 1
}

// fdSet returns a bitmask representation of the integer values of fdEvents
// and fdPipe. See man select for more info.
func (obj *SocketSet) fdSet() *unix.FdSet {
	fdSet := &unix.FdSet{}
	// Generate the bitmask representing the file descriptors in the SocketSet.
	// The rightmost bit corresponds to file descriptor zero, and each bit to
	// the left represents the next file descriptor number in the sequence of
	// all real numbers. E.g. the FdSet containing containing 0 and 4 is 10001.
	fdSet.Bits[obj.fdEvents/64] |= 1 << uint(obj.fdEvents)
	fdSet.Bits[obj.fdPipe/64] |= 1 << uint(obj.fdPipe)
	return fdSet
}

// SocketSet is used to receive events from a socket and shut it down cleanly
// when asked. It contains a socket for events and a pipe socket to unblock
// receive on shutdown.
type SocketSet struct {
	fdEvents int
	fdPipe   int
	pipeFile string
}

func EventSocketSet(groups uint32, name string) (*SocketSet, error) {
	fdEvents, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW, unix.NETLINK_KOBJECT_UEVENT)
	if err != nil {
		return nil, errwrap.Wrapf(err, "error creating netlink socket")
	}

	if err := unix.Bind(fdEvents, &unix.SockaddrNetlink{
		Family: unix.AF_NETLINK,
		Groups: groups,
		Pid: uint32(os.Getpid()),
	}); err != nil {
		return nil, errwrap.Wrapf(err, "error binding netlink socket")
	}

	// This pipe unblocks unix.Select upon closing
	fdPipe, err := unix.Socket(unix.AF_UNIX, unix.SOCK_RAW, unix.PROT_NONE)
	if err != nil {
		return nil, errwrap.Wrapf(err, "error creating pipe socket")
	}

	// Delete the socket file when the program ends
	//if err = unix.Unlink(name); err != nil {
	//	return nil, errwrap.Wrapf(err, "could not unlink socket")
	//}

	if err = unix.Bind(fdPipe, &unix.SockaddrUnix{
		Name: name,
	}); err != nil {
		return nil, errwrap.Wrapf(err, "error binding pipe socket")
	}

	return &SocketSet{
		fdEvents: fdEvents,
		fdPipe:   fdPipe,
		pipeFile: name,
	}, nil
}
