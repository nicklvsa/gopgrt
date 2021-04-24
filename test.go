package main

import (
	realtime "gopgrt/pkg"
)

func main() {
	dbStr := "host=localhost user=nicklvsa password=postgres1234 dbname=another port=5432 sslmode=disable"

	trigger := realtime.NewTrigger(dbStr, "listener", "events")
	trigger.Create()

	trigger.RegisterTable("users")

	trigger.Serve(2222, nil)
}
