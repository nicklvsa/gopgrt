package main

import (
	"fmt"
	realtime "gopgrt/pkg"
)

func main() {
	dbStr := "host=localhost user=nicklvsa password=postgres1234 dbname=another port=5432 sslmode=disable"

	trigger := realtime.NewTrigger(dbStr, "listener", "events")
	trigger.Create()

	trigger.RegisterTable("users")

	dataChan := make(chan realtime.TriggerEvent)
	errChan := make(chan error)

	go trigger.Serve(2222, nil)
	go trigger.Listen(dataChan, errChan)

	for {
		select {
		case data := <-dataChan:
			fmt.Printf("%+v\n", data.Payload)
		case err := <-errChan:
			panic(err.Error())
		}
	}
}
