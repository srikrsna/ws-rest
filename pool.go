package wsrest

import (
	"sync"
	"net/http"
)

var pool = sync.Pool{
	New: func() interface{} {
		return &Response{
			Headers: http.Header{},
		}
	},
}

func getRespFromPool() *Response {
	return pool.Get().(*Response)
}

func putRespInPool(r *Response) {
	pool.Put(r)
}
