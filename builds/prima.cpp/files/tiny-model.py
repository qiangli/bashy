#!/usr/bin/env python3
"""Deterministic synthetic GGUF fixture; no downloaded weights or Python packages."""
import math
import struct
import sys

def u32(n): return struct.pack('<I', n)
def u64(n): return struct.pack('<Q', n)
def string(s):
    s = s.encode()
    return u64(len(s)) + s

def make(path):
    tokens = ['<unk>', '<s>', '</s>', '▁'] + list('abcdefghijklmnopqrstuvwxyz') + ['.', '!']
    kv = {'general.architecture': (8, string('llama')),
          'general.name': (8, string('bashy-synthetic-egress-fixture')),
          'tokenizer.ggml.model': (8, string('llama')),
          'tokenizer.ggml.tokens': (9, u32(8)+u64(len(tokens))+b''.join(map(string,tokens))),
          'tokenizer.ggml.scores': (9, u32(6)+u64(len(tokens))+struct.pack('<32f', *([0.0]*32))),
          'tokenizer.ggml.token_type': (9, u32(5)+u64(32)+struct.pack('<32i', 2,3,3,*([1]*29))),
          'llama.attention.layer_norm_rms_epsilon': (6, struct.pack('<f', 1e-5))}
    for k,v in {'llama.context_length':128,'llama.embedding_length':16,'llama.block_count':2,
                'llama.feed_forward_length':32,'llama.attention.head_count':2,
                'llama.attention.head_count_kv':2,'llama.rope.dimension_count':8,
                'tokenizer.ggml.bos_token_id':1,'tokenizer.ggml.eos_token_id':2,
                'tokenizer.ggml.unknown_token_id':0}.items(): kv[k]=(4,u32(v))
    tensors=[('token_embd.weight',[16,32]),('output_norm.weight',[16]),('output.weight',[16,32])]
    for i in range(2):
        for name,dims in [('attn_norm',[16]),('attn_q',[16,16]),('attn_k',[16,16]),
                          ('attn_v',[16,16]),('attn_output',[16,16]),('ffn_norm',[16]),
                          ('ffn_gate',[16,32]),('ffn_down',[32,16]),('ffn_up',[16,32])]:
            tensors.append((f'blk.{i}.{name}.weight',dims))
    header=b'GGUF'+u32(3)+u64(len(tensors))+u64(len(kv))
    header+=b''.join(string(k)+u32(t)+v for k,(t,v) in kv.items())
    data=b''
    for name,dims in tensors:
        header+=string(name)+u32(len(dims))+b''.join(map(u64,dims))+u32(0)+u64(len(data))
        n=math.prod(dims)
        data+=struct.pack('<'+str(n)+'f', *([1.0 if len(dims)==1 else 0.01]*n))
        data+=b'\0'*((-len(data))%32)
    header+=b'\0'*((-len(header))%32)
    open(path,'wb').write(header+data)

if __name__=='__main__': make(sys.argv[1])
