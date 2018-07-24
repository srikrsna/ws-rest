package wsrest

import (
	"sync"
)

var pool = sync.Pool{
	New: func() interface{} {
		return new(Response)
	},
}

func getRespFromPool() *Response {
	return pool.Get().(*Response)
}

func putRespInPool(r *Response) {
	pool.Put(r)
}
