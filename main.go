package main

import (
	"log"

	"github.com/ssetin/PenguinCast/src/ice"
	"i2pgit.org/idk/dialeverything"
)

func main() {
	if err := dialeverything.Init(); err != nil {
		log.Fatal(err)
	}
	defer dialeverything.Destroy()
	server, err := ice.NewServer()

	if err != nil {
		log.Println(err.Error())
		return
	}
	defer server.Close()

	server.Start()
}
