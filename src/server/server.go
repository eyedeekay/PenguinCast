// Package iceserver - icecast streaming server
package iceserver

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ssetin/PenguinCast/src/fastStat"
)

const (
	cServerName = "PenguinCast"
	cVersion    = "0.9.0dev"
)

var (
	vCommands = [...]string{"metadata"}
)

// IceServer ...
type IceServer struct {
	serverName string
	version    string

	Props Properties

	mux            sync.Mutex
	Started        int32
	StartedTime    time.Time
	ListenersCount int32
	SourcesCount   int32

	statReader fastStat.ProcStatsReader
	// for monitor
	cpuUsage float64
	memUsage int

	// p2p relay's stuff
	relayPoints  map[string]int
	listenPoints map[string]int

	srv           *http.Server
	poolManager   PoolManager
	logError      *log.Logger
	logErrorFile  *os.File
	logAccess     *log.Logger
	logAccessFile *os.File
	statFile      *os.File
}

// Init - Load params from config.json
func (i *IceServer) Init() error {
	var err error

	i.serverName = cServerName
	i.version = cVersion

	i.ListenersCount = 0
	i.SourcesCount = 0
	i.Started = 0

	log.Println("Init " + i.serverName + " " + i.version)

	err = i.initConfig()
	if err != nil {
		return err
	}
	err = i.initLog()
	if err != nil {
		return err
	}
	err = i.initMounts()
	if err != nil {
		return err
	}

	i.srv = &http.Server{
		Addr: ":" + strconv.Itoa(i.Props.Socket.Port),
	}

	if i.Props.Logging.UseStat {
		i.statReader.Init()
		go i.processStats()
	}

	i.relayPoints = make(map[string]int)
	i.listenPoints = make(map[string]int)

	return nil
}

func (i *IceServer) initMounts() error {
	var err error
	for idx := range i.Props.Mounts {
		err = i.Props.Mounts[idx].Init(i)
		if err != nil {
			return err
		}
	}
	return nil
}

func (i *IceServer) incListeners() {
	atomic.AddInt32(&i.ListenersCount, 1)
}

func (i *IceServer) decListeners() {
	if atomic.LoadInt32(&i.ListenersCount) > 0 {
		atomic.AddInt32(&i.ListenersCount, -1)
	}
}

func (i *IceServer) checkListeners() bool {
	clientsLimit := atomic.LoadInt32(&i.Props.Limits.Clients)
	if atomic.LoadInt32(&i.ListenersCount) > clientsLimit {
		return false
	}
	return true
}

func (i *IceServer) incSources() {
	atomic.AddInt32(&i.SourcesCount, 1)
}

func (i *IceServer) decSources() {
	if atomic.LoadInt32(&i.SourcesCount) > 0 {
		atomic.AddInt32(&i.SourcesCount, -1)
	}
}

func (i *IceServer) checkSources() bool {
	sourcesLimit := atomic.LoadInt32(&i.Props.Limits.Sources)
	if atomic.LoadInt32(&i.SourcesCount) > sourcesLimit {
		return false
	}
	return true
}

// Close - finish
func (i *IceServer) Close() {

	if err := i.srv.Shutdown(context.Background()); err != nil {
		i.printError(1, err.Error())
		log.Printf("Error: %s\n", err.Error())
	} else {
		log.Println("Stopped")
	}

	for idx := range i.Props.Mounts {
		i.Props.Mounts[idx].Close()
	}

	i.statReader.Close()
	i.logErrorFile.Close()
	i.logAccessFile.Close()
	if i.statFile != nil {
		i.statFile.Close()
	}
}

func (i *IceServer) checkIsMount(page string) int {
	for idx := range i.Props.Mounts {
		if i.Props.Mounts[idx].Name == page {
			return idx
		}
	}
	return -1
}

func (i *IceServer) checkIsCommand(page string, r *http.Request) int {
	for idx := range vCommands {
		if vCommands[idx] == page && r.URL.Query().Get("mode") == "updinfo" {
			return idx
		}
	}
	return -1
}

func (i *IceServer) piHandler(w http.ResponseWriter, r *http.Request) {
	var peer peer
	var isRelayPoint bool

	peer.addr = r.Header.Get("MyAddr")
	if r.Header.Get("Flag") == "relay" {
		isRelayPoint = true
	}

	if isRelayPoint {
		log.Println("New relay point: " + peer.addr)
		i.relayPoints[peer.addr] = 0
	} else {
		log.Println("New listen point: " + peer.addr)
		i.listenPoints[peer.addr] = 0
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		i.printError(1, "webserver doesn't support hijacking")
		http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
		return
	}

	conn, bufrw, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	bufrw.WriteString("HTTP/1.0 200 OK\r\n")
	bufrw.WriteString("Server: ")
	bufrw.WriteString(i.serverName)
	bufrw.WriteString(" ")
	bufrw.WriteString(i.version)
	if !isRelayPoint && len(i.relayPoints) >= 1 {
		for a, count := range i.relayPoints {
			if count == 0 {
				bufrw.WriteString("\r\nAddress: ")
				bufrw.WriteString(a)
			}
		}
	} else if len(i.listenPoints) >= 1 {
		for a, count := range i.listenPoints {
			if count == 0 {
				bufrw.WriteString("\r\nAddress: ")
				bufrw.WriteString(a)
			}
		}
	}
	bufrw.WriteString("\r\n")
	bufrw.Flush()
}

func (i *IceServer) handler(w http.ResponseWriter, r *http.Request) {
	if i.Props.Logging.Loglevel == 4 {
		i.logHeaders(w, r)
	}

	page, mountidx, cmdidx, err := i.checkPage(w, r)
	if err != nil {
		i.printError(1, err.Error())
		return
	}

	if mountidx >= 0 {
		i.openMount(mountidx, w, r)
		return
	}

	if cmdidx >= 0 {
		i.runCommand(cmdidx, w, r)
		return
	}

	if strings.HasSuffix(page, "info.html") || strings.HasSuffix(page, "info.json") || strings.HasSuffix(page, "monitor.html") {
		i.renderPage(w, r, page)
	} else {
		http.ServeFile(w, r, page)
	}
}

/*
	runCommand
*/
func (i *IceServer) runCommand(idx int, w http.ResponseWriter, r *http.Request) {
	if idx == 0 {
		mountname := path.Base(r.URL.Query().Get("mount"))
		i.printError(4, "runCommand 0 with "+mountname)
		midx := i.checkIsMount(mountname)
		if midx >= 0 {
			i.Props.Mounts[midx].updateMeta(w, r)
		}
	}

}

func (i *IceServer) wrapHandlerWithStreaming(wrappedHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		//streamWriter := newStreamResponseWritter(w)
		wrappedHandler.ServeHTTP(w, req)
	})
}

/*Start - start listening port ...*/
func (i *IceServer) Start() {
	if atomic.LoadInt32(&i.Started) == 1 {
		return
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, os.Kill)

	if i.Props.Logging.UseMonitor {
		http.HandleFunc("/updateMonitor", func(w http.ResponseWriter, r *http.Request) {
			ws, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Fatal(err)
			}
			go i.sendMonitorInfo(ws)
		})
	}

	//streamedHandler := i.wrapHandlerWithStreaming(http.HandlerFunc(i.handler))
	//http.Handle("/", streamedHandler)
	http.HandleFunc("/Pi", i.piHandler)
	http.HandleFunc("/", i.handler)

	go func() {
		i.mux.Lock()
		i.StartedTime = time.Now()
		i.mux.Unlock()
		atomic.StoreInt32(&i.Started, 1)
		log.Print("Started on " + i.srv.Addr)

		if err := i.srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal(err)
		}

	}()

	<-stop
	atomic.StoreInt32(&i.Started, 0)
}
