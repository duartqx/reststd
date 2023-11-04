package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

type Colors struct {
	Red     string
	Green   string
	Yellow  string
	Blue    string
	Magenta string
	Cyan    string
	Reset   string
}

var colors *Colors = &Colors{
	Red:     "\033[31m",
	Green:   "\033[32m",
	Yellow:  "\033[33m",
	Blue:    "\033[34m",
	Magenta: "\033[35m",
	Cyan:    "\033[36m",
	Reset:   "\033[0m",
}

func GetStatusColor(status int) string {
	switch {
	case status >= 100 && status < 200:
		return colors.Cyan
	case status >= 200 && status < 300:
		return colors.Green
	case status >= 300 && status < 400:
		return colors.Yellow
	case status >= 400 && status < 500:
		return colors.Magenta
	default:
		return colors.Red
	}
}

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
	color  string
}

func NewRequestLoggerBuilder() *RequestLogger {
	return &RequestLogger{}
}

func (rl *RequestLogger) SetMethod(method string) *RequestLogger {
	rl.method = method
	return rl
}

func (rl *RequestLogger) SetPath(path string) *RequestLogger {
	rl.path = path
	return rl
}

func (rl *RequestLogger) SetSince(since time.Duration) *RequestLogger {
	rl.since = since
	return rl
}

func (rl *RequestLogger) SetStatus(status int) *RequestLogger {
	rl.status = status
	rl.color = GetStatusColor(status)
	return rl
}

func (rl RequestLogger) GetMethod() string {
	return rl.method
}

func (rl RequestLogger) GetStatus() int {
	return rl.status
}

func (rl RequestLogger) GetSince() time.Duration {
	return rl.since
}

func (rl RequestLogger) GetPath() string {
	return rl.path
}

func (rl RequestLogger) String() string {
	return fmt.Sprintf(
		"| %s | %s | %s | %s",
		rl.padAndColor(7, rl.GetMethod()),
		rl.padAndColor(0, rl.GetStatus()),
		rl.pad(12, rl.GetSince()),
		rl.GetPath(),
	)
}

func (rl RequestLogger) PanicString(err interface{}) string {

	stringer := func(e string) string {
		coloredError := rl.color + e + colors.Reset
		const tmpl string = "| %s | %s |             | %s %s"
		return fmt.Sprintf(
			tmpl,
			rl.padAndColor(7, rl.GetMethod()),
			rl.padAndColor(0, rl.GetStatus()),
			rl.GetPath(),
			coloredError,
		)
	}

	e, ok := err.(error)
	if !ok {
		return stringer("Unknown")
	}
	return stringer(e.Error())
}

func (rl RequestLogger) pad(padding int, value interface{}) string {
	var (
		v string = fmt.Sprint(value)
		r int    = int(math.Max(float64(padding-len(v)), 0))
	)
	return v + strings.Repeat(" ", r)
}

func (rl RequestLogger) padAndColor(padding int, value interface{}) string {
	if padding > 0 {
		return rl.color + rl.pad(padding, fmt.Sprint(value)) + colors.Reset
	}
	return rl.color + fmt.Sprint(value) + colors.Reset
}

func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				rl :=
					NewRequestLoggerBuilder().
						SetMethod(r.Method).
						SetStatus(http.StatusInternalServerError).
						SetPath(r.URL.Path)

				log.Println(rl.PanicString(err))
				w.WriteHeader(rl.GetStatus())
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		writer := &ResponseRecorderWriter{
			ResponseWriter: w,
			Status:         http.StatusOK,
		}

		next.ServeHTTP(writer, r)

		log.Println(
			NewRequestLoggerBuilder().
				SetMethod(r.Method).
				SetStatus(writer.Status).
				SetPath(r.URL.Path).
				SetSince(time.Since(start)),
		)
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

	router.Use(RecoveryMiddleware, LoggerMiddleware)

	router.
		Name("get_not_allowed").
		Path("/get_not_allowed").
		HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("Ok: " + r.Method))
		}).
		Methods("POST", "PUT")

	// Proposital nil pointer panic
	router.
		Name("nil_pointer").
		Path("/nil_pointer").
		HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var s struct{ n *struct{ n int } }
			w.Write([]byte(fmt.Sprint(s.n.n)))
		}).
		Methods("GET")

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

	log.Println("| Listening at port " + port)
	// Run our server in a goroutine so that it doesn't block.
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()

	c := make(chan os.Signal, 1)
	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt)

	// Block until we receive our signal.
	<-c

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	srv.Shutdown(ctx)
	// Optionally, you could run srv.Shutdown in a goroutine and block on
	// <-ctx.Done() if your application should wait for other services
	// to finalize based on context cancellation.
	log.Println("| Shutting down")
	os.Exit(0)
}
