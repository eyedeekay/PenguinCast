// Copyright 2019 Setin Sergei
// Licensed under the Apache License, Version 2.0 (the "License")

package ice

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eyedeekay/i2pkeys"
	sam "github.com/eyedeekay/sam3/helper"
	"github.com/ssetin/PenguinCast/src/log"
	"github.com/ssetin/PenguinCast/src/pool"

	"github.com/gorilla/mux"
	"github.com/ssetin/PenguinCast/src/stat"
)

const (
	cServerName = "PenguinCast"
	cVersion    = "0.3.0dev"
)

type Server struct {
	serverName string
	version    string

	Options options

	mux            sync.Mutex
	Started        int32
	StartedTime    time.Time
	ListenersCount int32
	SourcesCount   int32

	statReader stat.ProcStatsReader
	// for monitor
	cpuUsage float64
	memUsage int

	srv         *http.Server
	poolManager PoolManager
	logger      Logger
}

// Init - Load params from config.yaml
func NewServer() (*Server, error) {
	srv := &Server{
		serverName:  cServerName,
		version:     cVersion,
		poolManager: pool.NewPoolManager(),
	}

	err := srv.Options.Load()
	if err != nil {
		return nil, err
	}
	srv.logger, err = log.NewLogger(srv.Options.Logging.LogLevel, srv.Options.Paths.Log)
	if err != nil {
		return nil, err
	}
	err = srv.initMounts()
	if err != nil {
		return nil, err
	}

	srv.logger.Log("%s %s", srv.serverName, srv.version)

	srv.srv = &http.Server{
		Addr:    ":" + strconv.Itoa(srv.Options.Socket.Port),
		Handler: srv.configureRouter(),
	}

	if srv.Options.Logging.UseStat {
		srv.statReader.Init()
		go srv.processStats()
	}

	return srv, nil
}

func (i *Server) configureRouter() *mux.Router {
	r := mux.NewRouter()
	r.StrictSlash(true)

	for _, mnt := range i.Options.Mounts {
		r.HandleFunc("/"+mnt.Name, mnt.write).Methods("SOURCE", "PUT")
		r.HandleFunc("/"+mnt.Name, mnt.read).Methods("GET")
		r.Path("/admin/metadata").Queries("mode", "updinfo", "mount", "/"+mnt.Name).HandlerFunc(mnt.meta).Methods("GET")
	}

	r.HandleFunc("/info", i.infoHandler).Methods("GET")
	r.HandleFunc("/info.json", i.jsonHandler).Methods("GET")
	if i.Options.Logging.UseMonitor {
		r.HandleFunc("/monitor", i.monitorHandler).Methods("GET")
		r.HandleFunc("/updateMonitor", i.updateMonitorHandler)
	}

	r.PathPrefix("/").Handler(NewFsHook(i.Options.Paths.Web))

	return r
}

func (i *Server) initMounts() error {
	var err error
	for idx := range i.Options.Mounts {
		err = i.Options.Mounts[idx].Init(i, i.logger, i.poolManager)
		if err != nil {
			return err
		}
	}
	return nil
}

func (i *Server) incListeners() {
	atomic.AddInt32(&i.ListenersCount, 1)
}

func (i *Server) decListeners() {
	if atomic.LoadInt32(&i.ListenersCount) > 0 {
		atomic.AddInt32(&i.ListenersCount, -1)
	}
}

func (i *Server) checkListeners() bool {
	clientsLimit := atomic.LoadInt32(&i.Options.Limits.Clients)
	if atomic.LoadInt32(&i.ListenersCount) > clientsLimit {
		return false
	}
	return true
}

func (i *Server) incSources() {
	atomic.AddInt32(&i.SourcesCount, 1)
}

func (i *Server) decSources() {
	if atomic.LoadInt32(&i.SourcesCount) > 0 {
		atomic.AddInt32(&i.SourcesCount, -1)
	}
}

func (i *Server) checkSources() bool {
	sourcesLimit := atomic.LoadInt32(&i.Options.Limits.Sources)
	if atomic.LoadInt32(&i.SourcesCount) > sourcesLimit {
		return false
	}
	return true
}

// Close - finish
func (i *Server) Close() {
	if err := i.srv.Shutdown(context.Background()); err != nil {
		i.logger.Error(err.Error())
		i.logger.Log("Error: %s\n", err.Error())
	} else {
		i.logger.Log("Stopped")
	}

	for idx := range i.Options.Mounts {
		i.Options.Mounts[idx].Close()
	}

	i.statReader.Close()
	i.logger.Close()
}

//223.33.152.54 - - [27/Feb/2012:13:37:21 +0300] "GET /gop_aac HTTP/1.1" 200 75638 "-" "WMPlayer/10.0.0.364 guid/3300AD50-2C39-46C0-AE0A-AC7B8159E203" 400
func (i Server) writeAccessLog(host string, startTime time.Time, request string, bytesSend int, refer, userAgent string, seconds int) {
	i.logger.Access("%s - - [%s] \"%s\" %s %d \"%s\" \"%s\" %d\r\n", host, startTime.Format(time.RFC1123Z), request, "200", bytesSend, refer, userAgent, seconds)
}

func (i Server) getHost(addr string) string {
	idx := strings.Index(addr, ":")
	if idx == -1 {
		return addr
	}
	return addr[:idx]
}

func (i *Server) randomPassword() string {
	pass := strconv.FormatInt(rand.Int63(), 10)
	return pass
}

/*Start - start listening port ...*/
func (i *Server) Start() {
	if atomic.LoadInt32(&i.Started) == 1 {
		return
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, os.Kill)
	i.Options.Load()
	if i.Options.Auth.AdminPassword == "admin" {
		i.logger.Log("WARNING: Admin password is default password. Changing it by default. Please check config.yaml for new password.")
		i.Options.Auth.AdminPassword = i.randomPassword()
		if err := i.Options.Save(); err != nil {
			i.logger.Error(err.Error())
			i.logger.Log("Error: %s\n", err.Error())
		}
		i.logger.Log("Admin password: %s", i.Options.Auth.AdminPassword)
	}
	for index, mount := range i.Options.Mounts {
		if mount.Password == "admin" {
			i.logger.Log("WARNING: Mount %s password is default password. Changing it by default. Please check config.yaml for new password.", mount.Name)
			i.Options.Mounts[index].Password = i.randomPassword()
			if err := i.Options.Save(); err != nil {
				i.logger.Error(err.Error())
				i.logger.Log("Error: %s\n", err.Error())
			}
			i.logger.Log("Mount %s password: %s", mount.Name, i.Options.Mounts[index].Password)
		}
	}
	//i.Options.Load()
	go func() {
		if i.Options.UsesI2P {
			go func() {
				i.mux.Lock()
				i.StartedTime = time.Now()
				i.mux.Unlock()
				atomic.StoreInt32(&i.Started, 1)
				for {
					addr := i.Options.Host
					if listener, err := sam.I2PListener(addr, "127.0.0.1:7656", addr); err != nil {
						panic(err)
					} else {
						i.logger.Log("Listening on: %s", listener.Addr().String())
						i.logger.Log("Host address: %s", i.Options.Host)
						if i.Options.DisableClearnet && !strings.Contains(i.Options.Host, listener.Addr().String()) {
							// do the following only if the listener is not on the clearnet, and if the hostname
							// is not the same as the listener address, but only if the hostname is ends in
							// .b32.i2p or a non-I2P hostname
							if strings.HasSuffix(i.Options.Host, ".b32.i2p") || !strings.HasSuffix(i.Options.Host, ".i2p") {
								newAddr := listener.Addr().String() + ":" + strconv.Itoa(i.Options.Socket.Port)
								i.srv.Addr = newAddr
								i.Options.Host = listener.Addr().String()
								if err := os.Rename(addr+".i2p.private", i.Options.Host+".i2p.private"); err != nil {
									i.logger.Log("Error: %s\n", err.Error())
								}
								if err := os.Rename(addr+".i2p.public.txt", i.Options.Host+".i2p.public.txt"); err != nil {
									i.logger.Log("Error: %s\n", err.Error())
								}
								if err := i.Options.Save(); err != nil {
									i.logger.Error(err.Error())
									i.logger.Log("Error: %s\n", err.Error())
								}
							}
						}
						i.logger.Log("Started on %s", listener.Addr().(i2pkeys.I2PAddr).Base32())
						if err := i.srv.Serve(listener); err != nil {
							listener.Close()
							time.Sleep(time.Second * 15)
							continue
						}
					}
				}
			}()
		}
		if !i.Options.DisableClearnet {
			go func() {
				i.mux.Lock()
				i.StartedTime = time.Now()
				i.mux.Unlock()
				atomic.StoreInt32(&i.Started, 1)
				for {
					if listener, err := net.Listen("tcp", i.srv.Addr); err != nil {
						panic(err)
					} else {
						i.logger.Log("Started on %s", i.srv.Addr)
						if err := i.srv.Serve(listener); err != http.ErrServerClosed {
							listener.Close()
							time.Sleep(time.Second * 15)
							continue
						}
					}
				}
			}()
		} else {
			go func() {
				server := http.ServeMux{}
				server.HandleFunc("/", i.hello)
				server.HandleFunc("/index.html", i.hello)
				i.logger.Log("Started on %s", i.srv.Addr)
				if err := http.ListenAndServe(fmt.Sprintf("%s:%d", "127.0.0.1", i.Options.Socket.Port), &server); err != nil {
					panic(err)
				}
			}()
		}
	}()

	<-stop
	atomic.StoreInt32(&i.Started, 0)
}

func (i *Server) hello(w http.ResponseWriter, req *http.Request) {
	scheme := "http://"
	if i.Options.UsesI2P {
		b32, err := ioutil.ReadFile(i.Options.Host + ".i2p.public.txt")
		if err != nil {
			i.logger.Log("Error: %s\n", err.Error())
			return
		}
		addr := fmt.Sprintf("%s%s/info", scheme, string(b32))
		http.Redirect(w, req, addr, http.StatusFound)
	} else {
		addr := fmt.Sprintf("%s%s/info", scheme, i.Options.Host)
		http.Redirect(w, req, addr, http.StatusFound)
	}

}
