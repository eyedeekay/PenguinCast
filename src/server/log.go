// Package iceserver - icecast streaming server
// Copyright 2018 Setin Sergei
// Licensed under the Apache License, Version 2.0 (the "License")
package iceserver

import (
	"fmt"
	"log"
	"os"
)

/*
	Log Level
	1 - Errors; 2 - Warning; 3 - Info; 4 - Debug
*/

func (i *IceServer) initLog() error {
	var err error
	i.logErrorFile, err = os.OpenFile(i.Props.Paths.Log+"error.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return err
	}

	i.logAccessFile, err = os.OpenFile(i.Props.Paths.Log+"access.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return err
	}

	if i.Props.Logging.UseStat {
		i.statFile, err = os.OpenFile(i.Props.Paths.Log+"stat.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
		if err != nil {
			return err
		}
		fmt.Fprintln(i.statFile, "#Time	#Listeners	#CpuUsage	#MemUsage")
	}

	i.logError = log.New(i.logErrorFile, "", log.Ldate|log.Ltime)
	i.logAccess = log.New(i.logAccessFile, "", 0)

	return nil
}

func (i *IceServer) printError(errorlevel int, format string, v ...interface{}) {
	if errorlevel <= i.Props.Logging.Loglevel {
		var mark string
		if errorlevel == 1 {
			mark = "Error: "
		} else if errorlevel == 2 {
			mark = "Warn: "
		} else if errorlevel == 3 {
			mark = "Info: "
		} else if errorlevel == 4 {
			mark = "Debug: "
		}
		i.logError.Printf(mark+format, v...)
	}
}

func (i *IceServer) printAccess(format string, v ...interface{}) {
	i.logAccess.Printf(format, v...)
}