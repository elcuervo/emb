#!/usr/bin/env python3
"""Generate `testdata/plain-emb/model.onnx`, the plain-`EMB` gate's miniature.

The committed `testdata/laya/model.onnx` cannot serve the plain `EMB` path: its
graph declares five inputs (`input_ids`, `attention_mask`, `marker_pos`,
`marker_mask`, `qtype`) and the embedding pool feeds only the first two, so the
batch-determinism probe fails and every plain request errors out. This miniature
is the smallest graph that exercises the real plain path (tokenize -> one
batched ONNX run -> pool -> normalize) at a cost a burst can saturate:

    input_ids [batch, seq] i64, attention_mask [batch, seq] i64   (the pool's feed)
    Gather(tok_table, input_ids)                                  -> [batch, seq, dim]
    Tanh(x @ W_i + b_i) for each layer                            -> [batch, seq, dim]
    Mul(h, Cast(Unsqueeze(attention_mask, -1)))                   -> last_hidden_state

It is deliberately not a transformer: what the gate needs from a model is a
deterministic output and a per-run cost that scales with the token count, and
the dense stack gives both with a graph small enough to commit. Embedding
quality is irrelevant here — the fixture pins bytes, not semantics.

Deterministic by construction: the weights come from a fixed LCG seed, so this
script always writes the same bytes. `expected.json` is generated from the
server's reply to these bytes (see README.md), and the smoke gate fails if the
two disagree.

Only the Python standard library is used, and no ONNX runtime is needed to
write the file: it is encoded straight to the protobuf wire format that
`onnx.proto` defines. Regenerate with:

    python3 testdata/plain-emb/build-model.py
"""

import hashlib
import json
import math
import os
import struct

HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(HERE, "model.onnx")

# Model shape. DIM and LAYERS set the per-run cost: one row of SEQ tokens costs
# roughly LAYERS * 2 * DIM^2 * SEQ flops, which is what makes a burst fill the
# admission gate instead of finishing inside one scheduler tick.
VOCAB = 128
DIM = 128
LAYERS = 8
IR_VERSION = 9
OPSET = 20
FLOAT = 1
INT64 = 7

# ---------------------------------------------------------------- protobuf ---
# A minimal proto2 wire-format writer, enough for the ONNX messages below. No
# field here uses proto3 packing, so repeated scalars are emitted one tag each.


def varint(value):
    out = bytearray()
    while True:
        byte = value & 0x7F
        value >>= 7
        if value:
            out.append(byte | 0x80)
        else:
            out.append(byte)
            return bytes(out)


def key(field, wire):
    return varint((field << 3) | wire)


def msg(field, payload):
    return key(field, 2) + varint(len(payload)) + payload


def text(field, value):
    return msg(field, value.encode())


def ints(field, values):
    return b"".join(key(field, 0) + varint(v) for v in values)


def tensor(name, dims, data_type, raw):
    out = ints(1, dims)
    out += key(2, 0) + varint(data_type)
    out += msg(9, raw)
    out += text(8, name)
    return out


def dim(value):
    if isinstance(value, int):
        return key(1, 0) + varint(value)
    return text(2, value)


def value_info(name, elem_type, dims):
    shape = b"".join(msg(1, dim(d)) for d in dims)
    tensor_type = key(1, 0) + varint(elem_type) + msg(2, shape)
    return text(1, name) + msg(2, msg(1, tensor_type))


def attr_int(name, value):
    # AttributeProto.type == INT (2).
    return text(1, name) + key(3, 0) + varint(value) + key(20, 0) + varint(2)


def node(name, op_type, inputs, outputs, attrs=()):
    out = b"".join(text(1, i) for i in inputs)
    out += b"".join(text(2, o) for o in outputs)
    out += text(3, name)
    out += text(4, op_type)
    for attribute in attrs:
        out += msg(5, attribute)
    return out


# ----------------------------------------------------------------- weights ---

def floats(seed, count, scale):
    """A deterministic pseudo-random table, scaled like a tanh fan-in."""
    state = seed
    values = []
    for _ in range(count):
        state = (state * 1103515245 + 12345) & 0x7FFFFFFF
        values.append(((state / 0x7FFFFFFF) * 2 - 1) * scale)
    return struct.pack("<%df" % count, *values)


def build():
    nodes = []
    initializers = [
        tensor("tok_table", [VOCAB, DIM], FLOAT,
               floats(0x5EED, VOCAB * DIM, 0.6)),
        tensor("unsqueeze_axes", [1], INT64, struct.pack("<q", -1)),
    ]

    nodes.append(node("gather", "Gather", ["tok_table", "input_ids"], ["h0"],
                      [attr_int("axis", 0)]))
    # Xavier-ish scaling keeps tanh in its linear region for a few layers, so
    # different texts keep visibly different pooled vectors.
    scale = math.sqrt(6.0 / (2 * DIM))
    for i in range(LAYERS):
        weight, bias = "layer%d.weight" % i, "layer%d.bias" % i
        initializers.append(tensor(weight, [DIM, DIM], FLOAT, floats(0xA5 + i, DIM * DIM, scale)))
        initializers.append(tensor(bias, [DIM], FLOAT, floats(0xB7 + i, DIM, scale)))
        nodes.append(node("matmul%d" % i, "MatMul", ["h%d" % i, weight], ["m%d" % i]))
        nodes.append(node("add%d" % i, "Add", ["m%d" % i, bias], ["a%d" % i]))
        nodes.append(node("tanh%d" % i, "Tanh", ["a%d" % i], ["h%d" % (i + 1)]))
    nodes.append(node("unsqueeze_mask", "Unsqueeze", ["attention_mask", "unsqueeze_axes"], ["mask_u"]))
    nodes.append(node("cast_mask", "Cast", ["mask_u"], ["mask_f"], [attr_int("to", FLOAT)]))
    nodes.append(node("masked", "Mul", ["h%d" % LAYERS, "mask_f"], ["last_hidden_state"]))

    graph = b"".join(msg(1, n) for n in nodes)
    graph += text(2, "plain_emb_miniature")
    graph += b"".join(msg(5, i) for i in initializers)
    graph += msg(11, value_info("input_ids", INT64, ["batch", "seq"]))
    graph += msg(11, value_info("attention_mask", INT64, ["batch", "seq"]))
    graph += msg(12, value_info("last_hidden_state", FLOAT, ["batch", "seq", DIM]))

    model = key(1, 0) + varint(IR_VERSION)
    model += text(2, "emb plain-emb fixture")
    model += msg(8, key(2, 0) + varint(OPSET))
    model += msg(7, graph)
    return model


def main():
    data = build()
    with open(OUT, "wb") as handle:
        handle.write(data)
    digest = hashlib.sha256(data).hexdigest()
    print("wrote %s (%d bytes, sha256 %s)" % (OUT, len(data), digest))
    print(json.dumps({"vocab": VOCAB, "dim": DIM, "layers": LAYERS, "model_sha256": digest}))


if __name__ == "__main__":
    main()
