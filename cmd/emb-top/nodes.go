package main

import (
	"fmt"
	"strings"
)

// nodeList collects repeatable -node flags.
type nodeList []string

func (n *nodeList) String() string { return strings.Join(*n, ",") }

func (n *nodeList) Set(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return fmt.Errorf("empty node specification")
	}
	*n = append(*n, v)
	return nil
}
