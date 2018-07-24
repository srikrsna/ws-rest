package wsrest

import (
	"net/http"
	"encoding/json"
	"io"
	"sync"
	"net/url"
	"io/ioutil"
	"bytes"
	"github.com/mailru/easyjson"
	"context"
)

//go:generate easyjson -all -lower_camel_case $GOFILE

var ridKey = struct{}{}

type WSRequest struct {
	ID         string `json:"id"`
	Method     string `json:"method"`
	RequestURI string `json:"requestUri"`

	Header http.Header     `json:"header"`
	Body   json.RawMessage `json:"body"`
}

type WSResponse struct {
	ID         string `json:"id"`
	StatusCode int    `json:"statusCode"`
	RequestURI string `json:"requestUri,omitempty"`

	Headers http.Header     `json:"header"`
	Body    json.RawMessage `json:"body"`
}

type readWriter struct{}

var httpPool = sync.Pool{
	New: func() interface{} {
		return new(http.Request)
	},
}

var wsReqPool = sync.Pool{
	New: func() interface{} {
		return new(WSRequest)
	},
}

var wsResPool = sync.Pool{
	New: func() interface{} {
		return new(WSResponse)
	},
}

// NewReadWriter ...
func NewReadWriter() ReadWriter {
	return &readWriter{}
}

func (*readWriter) Read(rdr io.Reader, r *http.Request) (*http.Request, error) {
	req := httpPool.Get().(*http.Request)
	req = req.WithContext(r.Context())

	req.TLS = r.TLS
	req.ProtoMajor = r.ProtoMajor
	req.ProtoMinor = r.ProtoMinor
	req.Proto = r.Proto
	req.Host = r.Host

	wsReq := wsReqPool.Get().(*WSRequest)
	defer wsReqPool.Put(wsReq)

	if err := easyjson.UnmarshalFromReader(rdr, wsReq); err != nil {
		return nil, err
	}

	u, err := url.ParseRequestURI(wsReq.RequestURI)
	if err != nil {
		return nil, err
	}

	req.RequestURI = wsReq.RequestURI
	req.Method = wsReq.Method
	req.Body = ioutil.NopCloser(bytes.NewReader(wsReq.Body))
	req.Header = wsReq.Header
	req.URL = u

	req = req.WithContext(context.WithValue(r.Context(), ridKey, wsReq.ID))

	return req, nil
}

func (*readWriter) Write(wrt io.Writer, r *http.Request, res *Response) error {
	wsRes := wsResPool.Get().(*WSResponse)

	wsRes.ID = res.request.Context().Value(ridKey).(string)
	wsRes.Body = res.Body()
	wsRes.RequestURI = r.RequestURI
	wsRes.Headers = res.Headers
	wsRes.StatusCode = res.StatusCode

	httpPool.Put(r)

	_, err := easyjson.MarshalToWriter(wsRes, wrt)
	wsResPool.Put(wsRes)
	return err
}
