// Copyright 2019 Setin Sergei
// Licensed under the Apache License, Version 2.0 (the "License")

package ice

import (
	"io/ioutil"
	"os"

	logl "log"

	"github.com/ssetin/PenguinCast/src/log"

	"gopkg.in/yaml.v3"
)

type options struct {
	Name            string `yaml:"Name"`
	Admin           string `yaml:"Admin,omitempty"`
	Location        string `yaml:"Location,omitempty"`
	UsesI2P         bool   `yaml:"UsesI2P,omitempty"`
	DisableClearnet bool   `yaml:"DisableClearnet,omitempty"`
	Host            string `yaml:"Host"`

	Socket struct {
		Port int `yaml:"Port"`
	} `yaml:"Socket"`

	Limits struct {
		Clients                int32 `yaml:"Clients"`
		Sources                int32 `yaml:"Sources"`
		SourceIdleTimeOut      int   `yaml:"SourceIdleTimeOut"`
		EmptyBufferIdleTimeOut int   `yaml:"EmptyBufferIdleTimeOut"`
		WriteTimeOut           int   `yaml:"WriteTimeOut"`
	} `yaml:"Limits"`

	Auth struct {
		AdminPassword string `yaml:"AdminPassword"`
	} `yaml:"Auth"`

	Paths struct {
		Base string `yaml:"Base"`
		Web  string `yaml:"Web"`
		Log  string `yaml:"Log"`
	} `yaml:"Paths"`

	Logging struct {
		LogLevel        log.LogsLevel `yaml:"LogLevel"`
		LogSize         int           `yaml:"LogSize"`
		UseMonitor      bool          `yaml:"UseMonitor"`
		MonitorInterval int           `yaml:"MonitorInterval"`
		UseStat         bool          `yaml:"UseStat"`
		StatInterval    int           `yaml:"StatInterval"`
	} `yaml:"Logging"`

	Mounts []*mount `yaml:"Mounts"`
}

func (o *options) Load() error {
	if _, err := os.Stat("config.yaml"); err != nil {
		return err
	}
	yamlFile, err := ioutil.ReadFile("config.yaml")
	if err != nil {
		return err
	}
	return yaml.Unmarshal(yamlFile, o)
}

func (o *options) Save() error {
	info, err := os.Stat("config.yaml")
	if err != nil {
		return err
	}
	preservedMode := info.Mode()
	bytes, err := yaml.Marshal(o)
	if err != nil {
		return err
	}
	logl.Println(string(bytes))

	err = ioutil.WriteFile("config.new.yaml", bytes, preservedMode)
	if err != nil {
		return err
	}
	err = os.Rename("config.yaml", "config.yaml.bak")
	if err != nil {
		return err
	}
	err = os.Rename("config.new.yaml", "config.yaml")
	if err != nil {
		return err
	}
	return nil
}
