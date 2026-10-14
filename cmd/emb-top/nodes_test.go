package main

import (
	"strings"
	"testing"
)

func TestParseFlagsNodeForms(t *testing.T) {
	cases := []struct {
		args []string
		want []string // expected specs, as host:port
	}{
		{nil, []string{"localhost:6379"}},
		{[]string{"-addr", "a:1"}, []string{"a:1"}},
		{[]string{"-node", "a:1"}, []string{"a:1"}},
		{[]string{"-node", "a:1", "-node", "b:2"}, []string{"a:1", "b:2"}},
		{[]string{"-nodes", "a:1,b:2"}, []string{"a:1", "b:2"}},
		{[]string{"-addr", "a:1", "-nodes", "b:2"}, []string{"a:1", "b:2"}},
		{[]string{"-node", "name"}, []string{"name:6379"}},
	}
	for _, tc := range cases {
		o, err := parseFlags(tc.args, func(string) string { return "" })
		if err != nil {
			t.Errorf("parseFlags(%v): %v", tc.args, err)
			continue
		}
		var got []string
		for _, s := range o.specs {
			got = append(got, s.Addr())
		}
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("parseFlags(%v) specs = %v, want %v", tc.args, got, tc.want)
		}
	}
}
