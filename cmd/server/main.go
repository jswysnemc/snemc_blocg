package main

import (
	"log"

	"github.com/snemc/snemc-blog/internal/server"
)

func main() {
	if err := server.Run(); err != nil {
		log.Fatal(err)
	}
}
