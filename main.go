package main

import (
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

type Handler func(w http.ResponseWriter, r *http.Request)

type ResponseRecorderWriter struct {
	http.ResponseWriter
	Status int
}

func (rr *ResponseRecorderWriter) WriteHeader(status int) {
	rr.Status = status
	rr.ResponseWriter.WriteHeader(status)
}

func LoggingMiddleware(next http.Handler) http.Handler {
	strepeater := func(l int, v interface{}) string {
		value := fmt.Sprint(v)
		return value + strings.Repeat(" ", int(math.Max(float64(l-len(value)), 0)))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		writer := &ResponseRecorderWriter{
			ResponseWriter: w,
			Status:         http.StatusOK,
		}

		next.ServeHTTP(writer, r)

		var (
			mtd   string = strepeater(7, r.Method)
			since string = strepeater(12, time.Since(start))
		)

		log.Printf("| %s | %d | %s | %s", mtd, writer.Status, since, r.URL.Path)
	})
}

func NotFoundHandler(r *mux.Router) http.Handler {
	return r.
		NewRoute().
		BuildOnly().
		Handler(LoggingMiddleware(http.HandlerFunc(http.NotFound))).
		GetHandler()
}

func main() {

	indexView, err := template.ParseFiles("index.html")
	if err != nil {
		log.Fatalln(err)
	}

	router := mux.NewRouter()

	router.NotFoundHandler = NotFoundHandler(router)

	router.Use(LoggingMiddleware)

	router.HandleFunc("/post_not_allowed", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Write([]byte("Not permitted"))
		} else {
			w.Write([]byte("Ok"))
		}
	})
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		indexView.Execute(w, nil)
	})

	srv := &http.Server{
		Handler: router,
		Addr:    "127.0.0.1:8000",
		// Good practice: enforce timeouts for servers you create!
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Println("Listening at port 8000")
	log.Fatalln(srv.ListenAndServe())
}
