package main

import (
	"log"
	"net/http"
)

func main() {
	server := NewServer()
	addr := ":8080"
	log.Println("listening on", addr)
	log.Fatal(http.ListenAndServe(addr, server.Handler()))
}
