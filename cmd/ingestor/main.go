package main

import (
	"log"

	"itswork.app/internal/app"
)

var runMain = app.RunMain

func main() {
	if err := runMain(); err != nil {
		log.Fatal("Application failed:", err)
	}
}
