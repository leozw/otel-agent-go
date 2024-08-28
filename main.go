package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/leozw/otel-agent-go/agent"
)

type Response struct {
	Message string      `json:"message,omitempty"`
	URL     string      `json:"url,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func main() {
	router := agent.StartAgent()
	port := 3000

	fileManagerURL := os.Getenv("NEXT_PUBLIC_FILE_MANAGER_URL")
	if fileManagerURL == "" {
		fileManagerURL = "http://localhost:8085"
	}
	fmt.Println("NEXT_PUBLIC_FILE_MANAGER_URL:", fileManagerURL)

	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		respondWithJSON(w, http.StatusOK, Response{Message: "Hello, Open!"})
	}).Methods("GET")

	router.HandleFunc("/buteco", func(w http.ResponseWriter, r *http.Request) {
		respondWithJSON(w, http.StatusOK, Response{Message: "Bora tomar uma?"})
	}).Methods("GET")

	router.HandleFunc("/file-manager-url", func(w http.ResponseWriter, r *http.Request) {
		respondWithJSON(w, http.StatusOK, Response{URL: fileManagerURL})
	}).Methods("GET")

	router.HandleFunc("/external-service-1", func(w http.ResponseWriter, r *http.Request) {
		response, err := http.Get("https://jsonplaceholder.typicode.com/posts/1")
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Erro ao chamar o serviço externo 1")
			return
		}
		defer response.Body.Close()

		var data map[string]interface{}
		if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
			respondWithError(w, http.StatusInternalServerError, "Erro ao decodificar a resposta do serviço externo 1")
			return
		}

		respondWithJSON(w, http.StatusOK, Response{Data: data})
	}).Methods("GET")

	router.HandleFunc("/external-service-2", func(w http.ResponseWriter, r *http.Request) {
		response, err := http.Get("https://jsonplaceholder.typicode.com/users/1")
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Erro ao chamar o serviço externo 2")
			return
		}
		defer response.Body.Close()

		var data map[string]interface{}
		if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
			respondWithError(w, http.StatusInternalServerError, "Erro ao decodificar a resposta do serviço externo 2")
			return
		}

		respondWithJSON(w, http.StatusOK, Response{Data: data})
	}).Methods("GET")

	router.HandleFunc("/local-service", func(w http.ResponseWriter, r *http.Request) {
		url := fmt.Sprintf("http://localhost:%d/buteco", port)
		response, err := http.Get(url)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Erro ao chamar o serviço local")
			return
		}
		defer response.Body.Close()

		var data map[string]interface{}
		if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
			respondWithError(w, http.StatusInternalServerError, "Erro ao decodificar a resposta do serviço local")
			return
		}

		respondWithJSON(w, http.StatusOK, Response{Data: data})
	}).Methods("GET")

	router.HandleFunc("/file-manager-service", func(w http.ResponseWriter, r *http.Request) {
		url := fmt.Sprintf("%s/files", fileManagerURL)
		response, err := http.Get(url)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Erro ao chamar o serviço de file manager")
			return
		}
		defer response.Body.Close()

		var data map[string]interface{}
		if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
			respondWithError(w, http.StatusInternalServerError, "Erro ao decodificar a resposta do serviço de file manager")
			return
		}

		respondWithJSON(w, http.StatusOK, Response{Data: data})
	}).Methods("GET")

	router.HandleFunc("/external-service-3", func(w http.ResponseWriter, r *http.Request) {
		response, err := http.Get("https://jsonplaceholder.typicode.com/albums/1")
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Erro ao chamar o serviço externo 3")
			return
		}
		defer response.Body.Close()

		var data map[string]interface{}
		if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
			respondWithError(w, http.StatusInternalServerError, "Erro ao decodificar a resposta do serviço externo 3")
			return
		}

		respondWithJSON(w, http.StatusOK, Response{Data: data})
	}).Methods("GET")

	log.Printf("Server is running on http://0.0.0.0:%d\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), router))
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, Response{Error: message})
}

func respondWithJSON(w http.ResponseWriter, code int, payload Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(payload)
}
