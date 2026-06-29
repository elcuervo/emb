package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
)

type ReferenceData struct {
	Model      string      `json:"model"`
	Dim        int         `json:"dim"`
	Sentences  []string    `json:"sentences"`
	Embeddings [][]float64 `json:"embeddings"`
}

func main() {
	addr := "127.0.0.1:6379"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}

	data, err := os.ReadFile("reference-embeddings.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading reference: %v\n", err)
		os.Exit(1)
	}

	var ref ReferenceData
	if err := json.Unmarshal(data, &ref); err != nil {
		fmt.Fprintf(os.Stderr, "parsing reference: %v\n", err)
		os.Exit(1)
	}

	passed := 0
	failed := 0

	for i, sentence := range ref.Sentences {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "connecting: %v\n", err)
			os.Exit(1)
		}

		cmd := fmt.Sprintf("*3\r\n$3\r\nEMB\r\n$6\r\nminilm\r\n$%d\r\n%s\r\n", len(sentence), sentence)
		conn.Write([]byte(cmd))

		emb, err := readEMBResponse(conn, ref.Dim)
		conn.Close()
		if err != nil {
			fmt.Printf("  ✗ %d: %v\n", i+1, err)
			failed++
			continue
		}

		refEmb := ref.Embeddings[i]
		cos := cosineSimilarity(emb, refEmb)
		status := "✓"
		if cos < 0.999 {
			status = "✗"
			failed++
		} else {
			passed++
		}
		fmt.Printf("  %s %d: cosine=%.6f  (%s)\n", status, i+1, cos, sentence)
	}

	total := passed + failed
	fmt.Printf("\n%d/%d passed, %d failed\n", passed, total, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func readEMBResponse(conn net.Conn, dim int) ([]float64, error) {
	// Read header: $<len>\r\n
	header := make([]byte, 0, 16)
	for {
		b := make([]byte, 1)
		_, err := conn.Read(b)
		if err != nil {
			return nil, fmt.Errorf("reading header: %w", err)
		}
		header = append(header, b[0])
		if len(header) >= 3 && header[len(header)-2] == '\r' && header[len(header)-1] == '\n' {
			break
		}
	}
	if header[0] != '$' {
		return nil, fmt.Errorf("expected bulk string, got %q", string(header))
	}

	var dataLen int
	fmt.Sscanf(string(header[1:]), "%d", &dataLen)

	data := make([]byte, dataLen)
	_, err := conn.Read(data)
	if err != nil {
		return nil, fmt.Errorf("reading data: %w", err)
	}

	trail := make([]byte, 2)
	_, _ = conn.Read(trail)

	return bytesToFloats(data, dim), nil
}

func bytesToFloats(data []byte, dim int) []float64 {
	vals := make([]float64, dim)
	for i := range dim {
		bits := uint32(data[i*4]) | uint32(data[i*4+1])<<8 | uint32(data[i*4+2])<<16 | uint32(data[i*4+3])<<24
		vals[i] = float64(math.Float32frombits(bits))
	}
	return vals
}

func cosineSimilarity(a []float64, b []float64) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	denom := math.Sqrt(na) * math.Sqrt(nb)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
