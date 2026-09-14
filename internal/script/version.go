package script

// APIVersion identifies the host-function surface available to scripts, so a
// script written against a newer server can detect an older one and report a
// capability error itself. It changes when host functions are added, removed,
// or change semantics, and stays stable for performance-only changes.
//
// History:
//
//	1.0.0 — emb.run / emb.run_batch / emb.tokenize.* / emb.math.{sigmoid,softmax,argmax,float32_bytes} / emb.image.{preprocess,info} / json
//	1.1.0 — emb.embed / emb.image.embed / emb.similarity / emb.distance; packed and selective emb.run outputs; emb.math.{dot,cosine,l2,norm,mean_pool,cls,topk,gather,slice,scale,add}
//	1.2.0 — array (default-form) outputs carry dtype; json.null round-trips inside arrays; scalar softmax/argmax; one empty-operand rule for the math module; integer-only scalar math arguments; host API version folded into the reply-cache key
const APIVersion = "1.2.0"
