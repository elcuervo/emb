package script

import (
	"bytes"
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

// replyBuffer serializes REPLY writer calls into exact RESP2 bytes, so a
// scripted reply can be cached and later replayed verbatim (WriteRaw) without
// re-running the script or the conversion.
type replyBuffer struct {
	b bytes.Buffer
}

func (r *replyBuffer) WriteBulk(data []byte) {
	fmt.Fprintf(&r.b, "$%d\r\n%s\r\n", len(data), data)
}
func (r *replyBuffer) WriteInt(n int) {
	fmt.Fprintf(&r.b, ":%d\r\n", n)
}
func (r *replyBuffer) WriteArray(n int) {
	fmt.Fprintf(&r.b, "*%d\r\n", n)
}
func (r *replyBuffer) WriteNull() {
	r.b.WriteString("$-1\r\n")
}
func (r *replyBuffer) WriteError(msg string) {
	fmt.Fprintf(&r.b, "-%s\r\n", msg)
}

// EncodeReply converts a script result to its RESP2 byte encoding (usable
// verbatim with WriteRaw). It is byte-identical to what Convert writes to a
// live connection.
func EncodeReply(v lua.LValue) ([]byte, error) {
	buf := &replyBuffer{}
	if err := Convert(buf, v); err != nil {
		return nil, err
	}
	return buf.b.Bytes(), nil
}
