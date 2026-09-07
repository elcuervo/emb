package script

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// ReplyWriter is the minimal RESP2 write surface the conversion needs.
// redcon.Conn satisfies it directly in the server.
type ReplyWriter interface {
	WriteBulk(data []byte)
	WriteInt(n int)
	WriteArray(n int)
	WriteNull()
	WriteError(msg string)
}

// Convert writes `v` to the REPLY writer using the Redis-faithful grammar:
//
//	Lua string            → bulk (byte-safe: raw bytes pass through)
//	Lua number (integral) → integer reply
//	Lua number (fraction) → bulk string (RESP2 has no double; no truncation)
//	Lua true              → integer 1
//	Lua nil / false       → null
//	list table            → array of converted values
//	string-keyed table    → hash: flat field/value pairs (HGETALL shape)
//	{err = "msg"}         → error reply
//
// Tables nest recursively. Keys are converted in sorted order so identical
// tables always produce identical bytes (content-addressed caching depends on
// deterministic conversion). Mixed key tables (ints and strings) are an error.
func Convert(w ReplyWriter, v lua.LValue) error {
	if err := convertValue(w, v); err != nil {
		return fmt.Errorf("converting script result: %w", err)
	}
	return nil
}

func convertValue(w ReplyWriter, v lua.LValue) error {
	switch t := v.(type) {
	case lua.LBool:
		if bool(t) {
			w.WriteInt(1)
		} else {
			w.WriteNull()
		}
	case lua.LNumber:
		n := float64(t)
		if math.Trunc(n) == n {
			w.WriteInt(int(n))
		} else {
			w.WriteBulk([]byte(strconv.FormatFloat(n, 'g', -1, 64)))
		}
	case lua.LString:
		w.WriteBulk([]byte(t))
	case *lua.LTable:
		return convertTable(w, t)
	default:
		w.WriteNull()
	}
	return nil
}

func convertTable(w ReplyWriter, t *lua.LTable) error {
	keys := make([]lua.LValue, 0, t.Len())
	intLike := true
	strOnly := true
	hasErr := false
	t.ForEach(func(k, val lua.LValue) {
		keys = append(keys, k)
		if n, ok := k.(lua.LNumber); ok && math.Trunc(float64(n)) == float64(n) && float64(n) >= 1 {
			// integer key
		} else {
			intLike = false
		}
		if _, ok := k.(lua.LString); !ok {
			strOnly = false
		}
		if s, ok := k.(lua.LString); ok && string(s) == "err" {
			hasErr = true
		}
	})

	// {err = "msg"}: a single-field table errors the reply (Redis semantics).
	if hasErr && len(keys) == 1 {
		msg := t.RawGetString("err")
		if s, ok := msg.(lua.LString); ok {
			// A CR/LF inside the message would splice a second RESP frame onto
			// the wire (and, when cached, be replayed verbatim via WriteRaw),
			// breaking command/response alignment. Reject instead of writing.
			if strings.ContainsAny(string(s), "\r\n") {
				return fmt.Errorf("error reply contains CR or LF")
			}
			w.WriteError(string(s))
			return nil
		}
	}

	// List form: contiguous integer keys 1..n.
	if intLike && len(keys) == t.Len() && t.Len() > 0 {
		if err := checkContiguous(keys); err != nil {
			return err
		}
		list := sortedIntKeys(keys)
		w.WriteArray(len(list))
		for _, k := range list {
			if err := convertValue(w, t.RawGetInt(k)); err != nil {
				return err
			}
		}
		return nil
	}
	if intLike && len(keys) == 0 {
		w.WriteArray(0)
		return nil
	}

	// Hash form: all string keys → flat field/value pair array.
	if strOnly && len(keys) > 0 {
		names := make([]string, 0, len(keys))
		for _, k := range keys {
			names = append(names, k.String())
		}
		sort.Strings(names)
		w.WriteArray(2 * len(names))
		for _, name := range names {
			w.WriteBulk([]byte(name))
			if err := convertValue(w, t.RawGetString(name)); err != nil {
				return err
			}
		}
		return nil
	}

	return fmt.Errorf("mixed or unsupported table keys")
}

func checkContiguous(keys []lua.LValue) error {
	n := len(keys)
	seen := make(map[int]bool, n)
	for _, k := range keys {
		i := int(k.(lua.LNumber))
		if i < 1 || i > n || seen[i] {
			return fmt.Errorf("non-contiguous list keys")
		}
		seen[i] = true
	}
	return nil
}

func sortedIntKeys(keys []lua.LValue) []int {
	out := make([]int, 0, len(keys))
	for _, k := range keys {
		out = append(out, int(k.(lua.LNumber)))
	}
	sort.Ints(out)
	return out
}
