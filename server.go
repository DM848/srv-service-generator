package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/julienschmidt/httprouter"
)

func runServer() {

	// web server
	port := os.Getenv("WEB_SERVER_PORT")
	if port == "" {
		panic("missing environment variable WEB_SERVER_PORT")
	}

	router := httprouter.New()
	router.GET("/health", func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		fmt.Fprintf(w, `{"status":"ok"}`)
	})

	log.Fatal(http.ListenAndServe(":"+port, router))
}