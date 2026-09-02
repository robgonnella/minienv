package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

func handleHome(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "hello world")
}

type helloData struct {
	Name string `json:"name"`
}

func handleHello(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	hello := &helloData{}
	if err := json.Unmarshal(data, hello); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, "hello %s", hello.Name)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handleHome)
	mux.HandleFunc("POST /hello", handleHello)
	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}
	log.Println("Server starting on 0.0.0.0:8080")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed to start: %v", err)
	}
}
