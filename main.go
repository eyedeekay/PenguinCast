package main

import (
	"log"

	"github.com/ssetin/PenguinCast/src/ice"
	"i2pgit.org/idk/dialeverything"
)

func main() {
	server, err := ice.NewServer()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Starting server")
	if server.Options.UsesI2P {
		log.Println("Starting in I2P mode")
		if err := dialeverything.Setup(); err != nil {
			log.Fatal(err)
		}
		defer dialeverything.Destroy()
	} else {
		dialeverything.Destroy()
	}
	log.Println("Ready to start server")
	if err != nil {
		log.Println(err.Error())
		return
	}
	defer server.Close()

	server.Start()
}
