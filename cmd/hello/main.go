// Package main is a trivial http service used as a minienv deploy target in
// the example compose project and the specs.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const (
	listenAddr   = ":8080"
	readTimeout  = 5 * time.Second
	writeTimeout = 10 * time.Second
	idleTimeout  = 15 * time.Second
)

func handleHome(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)

	if _, err := fmt.Fprint(w, "hello world"); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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

	if _, err := fmt.Fprintf(w, "hello %s", hello.Name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handleHome)
	mux.HandleFunc("POST /hello", handleHello)

	server := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	log.Println("Server starting on 0.0.0.0" + listenAddr)

	if err := server.ListenAndServe(); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Server failed to start: %v", err)
	}
}
