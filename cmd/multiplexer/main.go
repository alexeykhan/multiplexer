package main

import (
	"log"

	"github.com/alexeykhan/multiplexer/internal/app"
)

func main() {
	a, err := app.New()
	if err != nil {
		log.Fatalf("instantiate app: %s\n", err.Error())
	}
	if err = a.Run(); err != nil {
		log.Fatal(err)
	}
}