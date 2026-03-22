package main

import (
	"log"

	"itswork.app/internal/app"
)

func main() {
	if err := app.RunMain(); err != nil {
		log.Fatal("Application failed:", err)
	}
}
