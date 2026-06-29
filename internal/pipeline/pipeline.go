package pipeline

type Request struct {
	Texts  []string
	Result chan<- Response
}

type Response struct {
	Embeddings [][]byte
	Err        error
}

type Stats struct {
	Requests   int64
	AvgLatency float64
	NumWorkers int
}
