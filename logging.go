package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

type requestLog struct {
	id    uint64
	port  string
	start time.Time
}

type requestLogKeyType struct{}

var requestLogKey requestLogKeyType

var requestSeq uint64

func newRequestLog(r *http.Request, port string) (*requestLog, *http.Request) {
	rec := &requestLog{
		id:    atomic.AddUint64(&requestSeq, 1),
		port:  port,
		start: time.Now(),
	}
	return rec, r.WithContext(context.WithValue(r.Context(), requestLogKey, rec))
}

func requestLogFrom(r *http.Request) *requestLog {
	if r == nil {
		return nil
	}
	rec, _ := r.Context().Value(requestLogKey).(*requestLog)
	return rec
}

func (rec *requestLog) prefix() string {
	if rec == nil {
		return "[req -]"
	}
	return fmt.Sprintf("[req %06d :%s]", rec.id, rec.port)
}

func (rec *requestLog) duration() time.Duration {
	if rec == nil {
		return 0
	}
	return time.Since(rec.start)
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += int64(n)
	return n, err
}

func (w *loggingResponseWriter) ReadFrom(src io.Reader) (int64, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if readerFrom, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		n, err := readerFrom.ReadFrom(src)
		w.bytes += n
		return n, err
	}
	n, err := io.Copy(w.ResponseWriter, src)
	w.bytes += n
	return n, err
}

func (w *loggingResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func (w *loggingResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
