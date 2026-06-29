package tokenizer

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"
)

type modelConfig struct {
	Type     string           `json:"type"`
	Vocab    map[string]int32 `json:"vocab"`
	UnkToken string           `json:"unk_token"`
}

type tokenizerFile struct {
	Model       modelConfig  `json:"model"`
	AddedTokens []addedToken `json:"added_tokens"`
}

type addedToken struct {
	ID         int32  `json:"id"`
	Content    string `json:"content"`
	SingleWord bool   `json:"single_word"`
	LStrip     bool   `json:"lstrip"`
	RStrip     bool   `json:"rstrip"`
}

type HFTokenizer struct {
	vocab     map[string]int32
	unkID     int32
	clsID     int32
	sepID     int32
	modelType string
}

func NewHFTokenizer(path string) (*HFTokenizer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading tokenizer file: %w", err)
	}

	var tf tokenizerFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("parsing tokenizer json: %w", err)
	}

	if tf.Model.Vocab == nil {
		return nil, fmt.Errorf("tokenizer has no vocab")
	}

	t := &HFTokenizer{
		vocab:     tf.Model.Vocab,
		unkID:     0,
		clsID:     101,
		sepID:     102,
		modelType: tf.Model.Type,
	}

	if tf.Model.UnkToken != "" {
		if id, ok := tf.Model.Vocab[tf.Model.UnkToken]; ok {
			t.unkID = id
		}
	}

	for _, at := range tf.AddedTokens {
		switch at.Content {
		case "[CLS]":
			t.clsID = at.ID
		case "[SEP]":
			t.sepID = at.ID
		case "[UNK]":
			t.unkID = at.ID
		}
	}

	return t, nil
}

func (t *HFTokenizer) Encode(text string, maxLength int) ([]int64, []int64, error) {
	switch t.modelType {
	case "WordPiece", "BERT":
		return t.encodeWordPiece(text, maxLength)
	case "BPE":
		return t.encodeBPE(text, maxLength)
	default:
		return t.encodeWordPiece(text, maxLength)
	}
}

func (t *HFTokenizer) encodeWordPiece(text string, maxLength int) ([]int64, []int64, error) {
	tokens := []int64{int64(t.clsID)}
	words := preTokenize(text)
	for _, word := range words {
		pieces := t.wordPiece(word)
		for _, p := range pieces {
			tokens = append(tokens, int64(p))
			if len(tokens) >= maxLength-1 {
				goto done
			}
		}
	}
done:
	tokens = append(tokens, int64(t.sepID))
	if len(tokens) > maxLength {
		tokens = tokens[:maxLength]
	}

	return tokens, makeMask(len(tokens)), nil
}

func makeMask(n int) []int64 {
	mask := make([]int64, n)
	for i := range n {
		mask[i] = 1
	}
	return mask
}

func (t *HFTokenizer) encodeBPE(text string, maxLength int) ([]int64, []int64, error) {
	tokens := []int64{}
	words := preTokenize(text)
	for _, word := range words {
		pieces := t.bpeEncode(word)
		for _, p := range pieces {
			tokens = append(tokens, int64(p))
			if len(tokens) >= maxLength {
				goto done
			}
		}
	}
done:
	if len(tokens) > maxLength {
		tokens = tokens[:maxLength]
	}

	return tokens, makeMask(len(tokens)), nil
}

func preTokenize(text string) []string {
	var words []string
	var word []rune
	for _, r := range text {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			if len(word) > 0 {
				words = append(words, string(word))
				word = nil
			}
			if !unicode.IsSpace(r) {
				words = append(words, string(r))
			}
		} else {
			word = append(word, r)
		}
	}
	if len(word) > 0 {
		words = append(words, string(word))
	}
	return words
}

func (t *HFTokenizer) wordPiece(word string) []int32 {
	runes := []rune(word)
	fullWord := strings.ToLower(string(runes))

	if id, ok := t.vocab[fullWord]; ok {
		return []int32{id}
	}

	var pieces []int32
	start := 0
	for start < len(fullWord) {
		end := len(fullWord)
		found := false
		for end > start {
			substr := fullWord[start:end]
			if start > 0 {
				substr = "##" + substr
			}
			if id, ok := t.vocab[substr]; ok {
				pieces = append(pieces, id)
				found = true
				break
			}
			end--
			if end-start == 1 && start > 0 {
				break
			}
		}
		if !found {
			pieces = append(pieces, t.unkID)
			break
		}
		start = end
	}

	return pieces
}

func (t *HFTokenizer) bpeEncode(word string) []int32 {
	word = strings.ToLower(word)
	if id, ok := t.vocab[word]; ok {
		return []int32{id}
	}

	chars := strings.Split(word, "")
	tokens := make([]string, len(chars))
	for i, c := range chars {
		if _, ok := t.vocab[c]; ok {
			tokens[i] = c
		} else {
			return []int32{t.unkID}
		}
	}

	for len(tokens) > 1 {

		bestPair := ""
		bestScore := -1
		for i := range len(tokens) - 1 {
			pair := tokens[i] + tokens[i+1]
			if id, ok := t.vocab[pair]; ok {
				if int(id) > bestScore {
					bestScore = int(id)
					bestPair = pair
				}
			}
		}
		if bestPair == "" {
			break
		}
		newTokens := make([]string, 0, len(tokens))
		i := 0
		for i < len(tokens) {
			if i < len(tokens)-1 && tokens[i]+tokens[i+1] == bestPair {
				newTokens = append(newTokens, bestPair)
				i += 2
			} else {
				newTokens = append(newTokens, tokens[i])
				i++
			}
		}
		tokens = newTokens
	}

	result := make([]int32, len(tokens))
	for i, token := range tokens {
		result[i] = t.vocab[token]
	}
	return result
}

func (t *HFTokenizer) Close() error {
	return nil
}
