package pipeline

type Request struct {
	Texts  []string
	Result chan<- Response
}

type Response struct {
	Embeddings [][]byte
	Err        error
}
