package main

import (
    "log"
    "net/http"
    "otel-agent-go/agent"
)

func main() {
    router := agent.StartAgent()

    router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("Hello, World!"))
    }).Methods("GET")

    log.Fatal(http.ListenAndServe(":8080", router))
}
