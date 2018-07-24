package wsrest

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
)

type options struct {
	Handler http.Handler

	Upgrader       *websocket.Upgrader
	ResponseHeader http.Header

	ErrorLog *log.Logger

	ReadWriter ReadWriter
}

// Option ...
type Option func(*options)

// New returns a new http.Handler that accepts web socket connections and starts serving them on
func New(h http.Handler, ops ...Option) http.Handler {
	opt := &options{
		Handler:    h,
		ErrorLog:   log.New(os.Stderr, "wsrest", log.LstdFlags),
		Upgrader:   &websocket.Upgrader{},
		ReadWriter: NewReadWriter(),
	}

	for _, f := range ops {
		f(opt)
	}

	return opt
}

// WithUpgrader ....
func WithUpgrader(u *websocket.Upgrader) Option {
	return func(o *options) {
		o.Upgrader = u
	}
}

// WithReadWriter ....
func WithReadWriter(rw ReadWriter) Option {
	return func(o *options) {
		o.ReadWriter = rw
	}
}

func (o *options) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := o.Upgrader.Upgrade(w, r, o.ResponseHeader)
	if err != nil {
		o.ErrorLog.Printf("error upgrading request: %v\n", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader := o.read(ctx, conn, r)
	writer := o.process(ctx, reader)
	o.write(conn, writer)
}

func (o *options) write(conn *websocket.Conn, res <-chan *Response) {
	for w := range res {
		conn.EnableWriteCompression(true)
		wc, err := conn.NextWriter(websocket.TextMessage)
		if err != nil {
			return
		}

		if err := o.ReadWriter.Write(wc, w.request, w); err != nil {
			return
		}
		wc.Close()

		putRespInPool(w)
	}
}

func (o *options) process(ctx context.Context, req <-chan *http.Request) <-chan *Response {
	res := make(chan *Response)

	go func() {
		defer func() {
			close(res)
			if err := recover(); err != nil {
				o.ErrorLog.Println("panic while processing request: ", err)
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case r, ok := <-req:
				if !ok { // If request channel is closed end processing replacement for for loop based approach
					return
				}

				go func() {
					w := getRespFromPool()
					w.reset()

					w.request = r

					o.Handler.ServeHTTP(w, r)

					res <- w
				}()
			}
		}
	}()

	return res
}

func (o *options) read(ctx context.Context, conn *websocket.Conn, r *http.Request) <-chan *http.Request {
	readCh := make(chan *http.Request)

	go func() {
		defer close(readCh)

		for {
			select {
			case <-ctx.Done():
				return
			default:
				_, rdr, err := conn.NextReader()
				if err != nil {
					o.ErrorLog.Printf("error obtaining next reader: %v\n", err)
					return
				}

				req, err := o.ReadWriter.Read(rdr, r)
				if err != nil {
					o.ErrorLog.Printf("error deserializing request: %v\n", err)
					return
				}

				readCh <- req.WithContext(ctx)
			}

		}
	}()

	return readCh
}

// ReadWriter ...
type Reader interface {
	Read(io.Reader, *http.Request) (*http.Request, error)
}

// ReadWriter ....
type Writer interface {
	Write(io.Writer, *http.Request, *Response) error
}

type ReadWriter interface {
	Reader
	Writer
}

// Response ....
type Response struct {
	StatusCode int
	Headers    http.Header
	buf        bytes.Buffer

	request *http.Request
}

// Header implements http.ResponseHeader
func (w *Response) Header() http.Header {
	return w.Headers
}

// Write implements http.ResponseHeader
func (w *Response) Write(b []byte) (int, error) {
	return w.buf.Write(b)
}

// WriteHeader implements http.ResponseHeader
func (w *Response) WriteHeader(statusCode int) {
	w.StatusCode = statusCode
}

func (w *Response) Body() []byte {
	return w.buf.Bytes()
}

func (w *Response) Read(p []byte) (n int, err error) {
	return w.buf.Read(p)
}

func (w *Response) reset() {
	w.request = nil
	w.buf.Reset()
	for k := range w.Headers {
		delete(w.Headers, k)
	}
}
