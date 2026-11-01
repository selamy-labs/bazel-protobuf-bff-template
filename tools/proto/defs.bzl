"""The repository's schema generation: the shared rule with its one pinned schema import."""

load("@bff_template_quality//proto:defs.bzl", _buf_generate = "buf_generate")

def buf_generate(name, **kwargs):
    _buf_generate(name = name, validate = "@protovalidate_proto//file", **kwargs)
