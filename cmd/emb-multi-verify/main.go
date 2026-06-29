package main

import (
	"fmt"
	"net"
	"os"
)

type ModelConfig struct {
	Name string
	Dim  int
}

func main() {
	addr := "127.0.0.1:6379"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}

	models := []ModelConfig{
		{Name: "minilm", Dim: 384},
		{Name: "siglip2", Dim: 768},
	}

	passed := 0
	failed := 0

	check := func(name string, ok bool) {
		if ok {
			fmt.Printf("  ✓ %s\n", name)
			passed++
		} else {
			fmt.Printf("  ✗ %s\n", name)
			failed++
		}
	}

	// 1. EMB.MULTI across two different models
	fmt.Println("\nTest 1: Cross-model EMB.MULTI")
	multiResp := sendEMBMULTI(addr, models, []string{"hello world", "query: test"}, check)
	if multiResp == nil {
		return
	}

	// 2. Compare against sequential EMB calls
	fmt.Println("\nTest 2: Byte-equality vs sequential EMB")
	compareSequential(addr, models, []string{"hello world", "query: test"}, multiResp, check)

	// 3. Same model, multiple pairs (batcher test)
	fmt.Println("\nTest 3: Same-model EMB.MULTI (batcher)")
	sameModelResp := sendEMBMULTISame(addr, models[0], []string{"a", "b", "c"}, check)
	if sameModelResp != nil {
		compareSequentialSame(addr, models[0], []string{"a", "b", "c"}, sameModelResp, check)
	}

	total := passed + failed
	fmt.Printf("\n%d/%d passed, %d failed\n", passed, total, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func sendEMBMULTI(addr string, models []ModelConfig, texts []string, check func(string, bool)) [][]byte {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connecting: %v\n", err)
		return nil
	}
	defer conn.Close()

	// Build RESP: EMB.MULTI model1 text1 model2 text2
	parts := []string{"EMB.MULTI"}
	for i, m := range models {
		parts = append(parts, m.Name, texts[i])
	}

	cmd := buildRESPArray(parts)
	conn.Write([]byte(cmd))

	resp, err := readArrayResponse(conn, len(models))
	if err != nil {
		check(fmt.Sprintf("response: %v", err), false)
		return nil
	}

	if len(resp) != len(models) {
		check(fmt.Sprintf("expected %d embeddings, got %d", len(models), len(resp)), false)
		return nil
	}

	for i, emb := range resp {
		if emb == nil {
			check(fmt.Sprintf("%s: got nil", models[i].Name), false)
			return nil
		}
		expectedLen := models[i].Dim * 4
		if len(emb) != expectedLen {
			check(fmt.Sprintf("%s: expected %d bytes, got %d", models[i].Name, expectedLen, len(emb)), false)
			return nil
		}
	}
	check(fmt.Sprintf("EMB.MULTI returned %d embeddings with correct dims", len(models)), true)
	return resp
}

func compareSequential(addr string, models []ModelConfig, texts []string, multiResp [][]byte, check func(string, bool)) {
	for i, m := range models {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			check(fmt.Sprintf("%s: connect: %v", m.Name, err), false)
			continue
		}

		cmd := buildRESPArray([]string{"EMB", m.Name, texts[i]})
		conn.Write([]byte(cmd))

		emb, err := readBulkResponse(conn)
		conn.Close()
		if err != nil {
			check(fmt.Sprintf("%s: read: %v", m.Name, err), false)
			continue
		}

		if len(emb) != len(multiResp[i]) {
			check(fmt.Sprintf("%s: size mismatch: %d vs %d", m.Name, len(emb), len(multiResp[i])), false)
			continue
		}

		match := true
		for j := range emb {
			if emb[j] != multiResp[i][j] {
				match = false
				break
			}
		}
		check(fmt.Sprintf("%s: byte-identical to EMB", m.Name), match)
	}
}

func sendEMBMULTISame(addr string, model ModelConfig, texts []string, check func(string, bool)) [][]byte {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connecting: %v\n", err)
		return nil
	}
	defer conn.Close()

	parts := []string{"EMB.MULTI"}
	for _, t := range texts {
		parts = append(parts, model.Name, t)
	}

	cmd := buildRESPArray(parts)
	conn.Write([]byte(cmd))

	resp, err := readArrayResponse(conn, len(texts))
	if err != nil {
		check(fmt.Sprintf("same-model response: %v", err), false)
		return nil
	}

	if len(resp) != len(texts) {
		check(fmt.Sprintf("expected %d embeddings, got %d", len(texts), len(resp)), false)
		return nil
	}

	for i, emb := range resp {
		if emb == nil {
			check(fmt.Sprintf("text %q: got nil", texts[i]), false)
			return nil
		}
		expectedLen := model.Dim * 4
		if len(emb) != expectedLen {
			check(fmt.Sprintf("text %q: expected %d bytes, got %d", texts[i], expectedLen, len(emb)), false)
			return nil
		}
	}
	check(fmt.Sprintf("same-model EMB.MULTI returned %d embeddings", len(texts)), true)
	return resp
}

func compareSequentialSame(addr string, model ModelConfig, texts []string, multiResp [][]byte, check func(string, bool)) {
	for i, text := range texts {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			check(fmt.Sprintf("%s: connect: %v", text, err), false)
			continue
		}

		cmd := buildRESPArray([]string{"EMB", model.Name, text})
		conn.Write([]byte(cmd))

		emb, err := readBulkResponse(conn)
		conn.Close()
		if err != nil {
			check(fmt.Sprintf("%s: read: %v", text, err), false)
			continue
		}

		match := len(emb) == len(multiResp[i])
		if match {
			for j := range emb {
				if emb[j] != multiResp[i][j] {
					match = false
					break
				}
			}
		}
		check(fmt.Sprintf("%q byte-identical to EMB", text), match)
	}
}

func buildRESPArray(parts []string) string {
	resp := fmt.Sprintf("*%d\r\n", len(parts))
	for _, p := range parts {
		resp += fmt.Sprintf("$%d\r\n%s\r\n", len(p), p)
	}
	return resp
}

func readBulkResponse(conn net.Conn) ([]byte, error) {
	header := make([]byte, 0, 16)
	for {
		b := make([]byte, 1)
		if _, err := conn.Read(b); err != nil {
			return nil, fmt.Errorf("reading header: %w", err)
		}
		header = append(header, b[0])
		if len(header) >= 3 && header[len(header)-2] == '\r' && header[len(header)-1] == '\n' {
			break
		}
		if len(header) > 16 {
			return nil, fmt.Errorf("malformed header: %q", string(header))
		}
	}

	if header[0] == '-' {
		return nil, fmt.Errorf("server error: %s", string(header[1:len(header)-2]))
	}
	if header[0] == '$' && header[1] == '-' {
		return nil, nil
	}
	if header[0] != '$' {
		return nil, fmt.Errorf("expected bulk string, got %q", string(header))
	}

	var dataLen int
	fmt.Sscanf(string(header[1:]), "%d", &dataLen)

	data := make([]byte, dataLen)
	if _, err := conn.Read(data); err != nil {
		return nil, fmt.Errorf("reading data: %w", err)
	}

	trail := make([]byte, 2)
	conn.Read(trail)

	return data, nil
}

func readArrayResponse(conn net.Conn, _ int) ([][]byte, error) {
	header := make([]byte, 0, 16)
	for {
		b := make([]byte, 1)
		if _, err := conn.Read(b); err != nil {
			return nil, fmt.Errorf("reading array header: %w", err)
		}
		header = append(header, b[0])
		if len(header) >= 3 && header[len(header)-2] == '\r' && header[len(header)-1] == '\n' {
			break
		}
		if len(header) > 16 {
			return nil, fmt.Errorf("malformed array header: %q", string(header))
		}
	}

	if header[0] == '-' {
		return nil, fmt.Errorf("server error: %s", string(header[1:len(header)-2]))
	}
	if header[0] != '*' {
		return nil, fmt.Errorf("expected array, got %q", string(header))
	}

	var count int
	fmt.Sscanf(string(header[1:]), "%d", &count)
	if count < 0 {
		return nil, nil
	}

	result := make([][]byte, count)
	for i := range count {
		emb, err := readBulkResponse(conn)
		if err != nil {
			return nil, fmt.Errorf("element %d: %w", i, err)
		}
		result[i] = emb
	}

	return result, nil
}
