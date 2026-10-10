package main

import (
	"log"
	"net/http"
)

func main() {
	cfg, err := loadConfig("./server/allowed_connections.json")
	if err != nil {
		log.Fatal(err)
	}

	auth, err := newAuthTable(cfg)
	if err != nil {
		log.Fatal(err)
	}

	server := NewServer(auth)
	addr := ":8080"
	log.Println("listening on", addr)
	log.Fatal(http.ListenAndServe(addr, server.Handler()))
}
