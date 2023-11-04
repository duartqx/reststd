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

type ResponseRecorderWriter struct {
	http.ResponseWriter
	Status int
}

func (rr *ResponseRecorderWriter) WriteHeader(status int) {
	rr.Status = status
	rr.ResponseWriter.WriteHeader(status)
}

type RequestLogger struct {
	method string
	status int
	since  time.Duration
	path   string
}

func (rl RequestLogger) pad(length int, value interface{}) string {
	var (
		v string = fmt.Sprint(value)
		r int    = int(math.Max(float64(length-len(v)), 0))
	)
	return v + strings.Repeat(" ", r)
}

func (rl RequestLogger) GetMethod() string {
	return rl.pad(7, rl.method)
}

func (rl RequestLogger) GetSince() string {
	return rl.pad(12, rl.since)
}

func (rl RequestLogger) GetStatus() int {
	return rl.status
}

func (rl RequestLogger) GetPath() string {
	return rl.path
}

func (rl RequestLogger) String() string {
	return fmt.Sprintf(
		"| %s | %d | %s | %s",
		rl.GetMethod(),
		rl.GetStatus(),
		rl.GetSince(),
		rl.GetPath(),
	)
}

func LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		writer := &ResponseRecorderWriter{
			ResponseWriter: w,
			Status:         http.StatusOK,
		}

		next.ServeHTTP(writer, r)

		log.Println(RequestLogger{
			method: r.Method,
			status: writer.Status,
			since:  time.Since(start),
			path:   r.URL.Path,
		})
	})
}

func NotFoundHandler(r *mux.Router) http.Handler {
	return r.
		NewRoute().
		BuildOnly().
		Handler(LoggerMiddleware(http.HandlerFunc(http.NotFound))).
		GetHandler()
}

func MethodNotAllowedHandler(r *mux.Router) http.Handler {
	e := func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", http.StatusMethodNotAllowed)
	}

	return r.
		NewRoute().
		BuildOnly().
		Handler(LoggerMiddleware(http.HandlerFunc(e))).
		GetHandler()
}

func main() {

	indexView, err := template.ParseFiles("index.html")
	if err != nil {
		log.Fatalln(err)
	}

	router := mux.NewRouter()

	router.NotFoundHandler = NotFoundHandler(router)
	router.MethodNotAllowedHandler = MethodNotAllowedHandler(router)

	router.Use(LoggerMiddleware)

	router.
		Name("get_not_allowed").
		Path("/get_not_allowed").
		HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("Ok: " + r.Method))
		}).
		Methods("POST", "PUT")

	router.
		Name("index").
		Path("/").
		HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			indexView.Execute(w, nil)
		}).
		Methods("GET")

	port := ":8000"

	srv := &http.Server{
		Handler:      router,
		Addr:         "127.0.0.1" + port,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Println("Listening at port " + port)
	log.Fatalln(srv.ListenAndServe())
}
