// libmgmt example of graph resource
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/purpleidea/mgmt/gapi"
	mgmt "github.com/purpleidea/mgmt/lib"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/resources"
)

// MyGAPI implements the main GAPI interface.
type MyGAPI struct {
	Name     string // graph name
	Interval uint   // refresh interval, 0 to never refresh

	data        gapi.Data
	initialized bool
	closeChan   chan struct{}
	wg          sync.WaitGroup // sync group for tunnel go routines
}

// NewMyGAPI creates a new MyGAPI struct and calls Init().
func NewMyGAPI(data gapi.Data, name string, interval uint) (*MyGAPI, error) {
	obj := &MyGAPI{
		Name:     name,
		Interval: interval,
	}
	return obj, obj.Init(data)
}

// Init initializes the MyGAPI struct.
func (obj *MyGAPI) Init(data gapi.Data) error {
	if obj.initialized {
		return fmt.Errorf("already initialized")
	}
	if obj.Name == "" {
		return fmt.Errorf("the graph name must be specified")
	}
	obj.data = data // store for later
	obj.closeChan = make(chan struct{})
	obj.initialized = true
	return nil
}

// Graph returns a current Graph.
func (obj *MyGAPI) Graph() (*pgraph.Graph, error) {
	if !obj.initialized {
		return nil, fmt.Errorf("libmgmt: MyGAPI is not initialized")
	}

	g, err := pgraph.NewGraph(obj.Name)
	if err != nil {
		return nil, err
	}

	// FIXME: these are being specified temporarily until it's the default!
	metaparams := resources.DefaultMetaParams

	content := "I created a subgraph!\n"
	f0 := &resources.FileRes{
		BaseRes: resources.BaseRes{
			Name:       "README",
			MetaParams: metaparams,
		},
		Path:    "/tmp/mgmt/README",
		Content: &content,
		State:   "present",
	}
	g.AddVertex(f0)

	// create a subgraph to add *into* a graph resource
	subGraph, err := pgraph.NewGraph(fmt.Sprintf("%s->subgraph", obj.Name))
	if err != nil {
		return nil, err
	}

	// add elements into the sub graph
	f1 := &resources.FileRes{
		BaseRes: resources.BaseRes{
			Name:       "file1",
			MetaParams: metaparams,
		},
		Path: "/tmp/mgmt/sub1",

		State: "present",
	}
	subGraph.AddVertex(f1)

	n1 := &resources.NoopRes{
		BaseRes: resources.BaseRes{
			Name:       "noop1",
			MetaParams: metaparams,
		},
	}
	subGraph.AddVertex(n1)

	e0 := &resources.Edge{Name: "e0"}
	e0.Notify = true // send a notification from v0 to v1
	subGraph.AddEdge(f1, n1, e0)

	// create the actual resource to hold the sub graph
	subGraphRes0 := &resources.GraphRes{ // TODO: should we name this SubGraphRes ?
		BaseRes: resources.BaseRes{
			Name:       "subgraph1",
			MetaParams: metaparams,
		},
		Graph: subGraph,
	}
	g.AddVertex(subGraphRes0) // add it to the main graph

	//g, err := config.NewGraphFromConfig(obj.data.Hostname, obj.data.World, obj.data.Noop)
	return g, nil
}

// Next returns nil errors every time there could be a new graph.
func (obj *MyGAPI) Next() chan gapi.Next {
	ch := make(chan gapi.Next)
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		defer close(ch) // this will run before the obj.wg.Done()
		if !obj.initialized {
			next := gapi.Next{
				Err:  fmt.Errorf("libmgmt: MyGAPI is not initialized"),
				Exit: true, // exit, b/c programming error?
			}
			ch <- next
			return
		}
		startChan := make(chan struct{}) // start signal
		close(startChan)                 // kick it off!

		ticker := make(<-chan time.Time)
		if obj.data.NoStreamWatch || obj.Interval <= 0 {
			ticker = nil
		} else {
			// arbitrarily change graph every interval seconds
			t := time.NewTicker(time.Duration(obj.Interval) * time.Second)
			defer t.Stop()
			ticker = t.C
		}
		for {
			select {
			case <-startChan: // kick the loop once at start
				startChan = nil // disable
				// pass
			case <-ticker:
				// pass
			case <-obj.closeChan:
				return
			}

			log.Printf("libmgmt: Generating new graph...")
			select {
			case ch <- gapi.Next{}: // trigger a run
			case <-obj.closeChan:
				return
			}
		}
	}()
	return ch
}

// Close shuts down the MyGAPI.
func (obj *MyGAPI) Close() error {
	if !obj.initialized {
		return fmt.Errorf("libmgmt: MyGAPI is not initialized")
	}
	close(obj.closeChan)
	obj.wg.Wait()
	obj.initialized = false // closed = true
	return nil
}

// Run runs an embedded mgmt server.
func Run() error {

	obj := &mgmt.Main{}
	obj.Program = "libmgmt" // TODO: set on compilation
	obj.Version = "0.0.1"   // TODO: set on compilation
	obj.TmpPrefix = true    // disable for easy debugging
	//prefix := "/tmp/testprefix/"
	//obj.Prefix = &p // enable for easy debugging
	obj.IdealClusterSize = -1
	obj.ConvergedTimeout = -1
	obj.Noop = false // FIXME: careful!

	obj.GAPI = &MyGAPI{ // graph API
		Name:     "libmgmt", // TODO: set on compilation
		Interval: 60 * 10,   // arbitrarily change graph every 15 seconds
	}

	if err := obj.Init(); err != nil {
		return err
	}

	// install the exit signal handler
	exit := make(chan struct{})
	defer close(exit)
	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt) // catch ^C
		//signal.Notify(signals, os.Kill) // catch signals
		signal.Notify(signals, syscall.SIGTERM)

		select {
		case sig := <-signals: // any signal will do
			if sig == os.Interrupt {
				log.Println("Interrupted by ^C")
				obj.Exit(nil)
				return
			}
			log.Println("Interrupted by signal")
			obj.Exit(fmt.Errorf("killed by %v", sig))
			return
		case <-exit:
			return
		}
	}()

	if err := obj.Run(); err != nil {
		return err
	}
	return nil
}

func main() {
	log.Printf("Hello!")
	if err := Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
		return
	}
	log.Printf("Goodbye!")
}