package main

import (
	"log"
	"sync"
	"testing"
	"time"

	"github.com/ssetin/PenguinCast/src/client"
)

// ================================== Setup ========================================
const (
	listenersCount = 6000 // total number of listeners
	incStep        = 25   // number of listeners, to increase with each step
	waitStep       = 5    // seconds between each step
	secToListen    = 5400 // seconds to listen by each connection
	mountName      = "RockRadio96"
	hostAddr       = "192.168.10.2:8008"
)

/*
var IcySrv iceserver.IceServer

func startServer() {
	err := IcySrv.Init()
	defer IcySrv.Close()
	if err != nil {
		log.Println(err.Error())
		return
	}
	IcySrv.Start()
}
*/

// ================================== Benchmarks ===========================================

func BenchmarkListenersCount(b *testing.B) {
	// run server in another process to monitor it separately from clients
	/*
		go startServer()
		time.Sleep(time.Second * 2)
		log.Println("Waiting for SOURCE to connect...")
		time.Sleep(time.Second * 10)
	*/
	log.Println("Start creating listeners...")

	wg := &sync.WaitGroup{}

	for i := 0; i < listenersCount/incStep; i++ {
		wg.Add(incStep)
		for k := 0; k < incStep; k++ {
			go func(wg *sync.WaitGroup, i int) {
				defer wg.Done()
				time.Sleep(time.Millisecond * 200)
				cl := &iceclient.PenguinClient{}
				//if i < 30 {
				//	cl.Init(hostAddr, mountName, "dump/"+mountName+"."+strconv.Itoa(i)+".mp3")
				//} else {
				cl.Init(hostAddr, mountName, "")
				//}
				err := cl.Listen(secToListen)
				if err != nil {
					log.Println(err)
				}
			}(wg, i)
		}
		time.Sleep(time.Second * waitStep)

	}
	log.Println("Waiting for listeners to finito...")
	wg.Wait()
}

/*
	go test -race -bench . -benchmem -timeout 300m main_test.go
	go test -bench . -benchmem -cpuprofile=cpu.out -memprofile=mem.out -timeout 300m main_test.go
	mp3check -e -a -S -T -E -v dump/*.mp3
	ulimit -n 63000
*/