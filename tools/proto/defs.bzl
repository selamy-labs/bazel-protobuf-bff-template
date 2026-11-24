"""The repository's schema generation: buf codegen plus hermetic Python protobuf/Connect codegen."""

load("@bff_template_quality//proto:defs.bzl", _buf_generate = "buf_generate")

def buf_generate(name, **kwargs):
    _buf_generate(name = name, validate = "@protovalidate_proto//file", **kwargs)

def _py_proto_connect_generate_impl(ctx):
    schema = ctx.file.schema
    import_path = ctx.attr.schema_path
    schema_dir = import_path.rsplit("/", 1)[0]
    wkp_files = ctx.files._well_known_protos
    timestamp_proto = None
    for f in wkp_files:
        if f.path.endswith("google/protobuf/timestamp.proto"):
            timestamp_proto = f.path
            break
    if not timestamp_proto:
        fail("well_known_protos missing google/protobuf/timestamp.proto")
    wkp_root = timestamp_proto[:-len("/google/protobuf/timestamp.proto")]

    out_rel_paths = []
    moves = []
    for out in ctx.outputs.outs:
        rel = _relative(ctx, out)
        out_rel_paths.append(rel)
        moves.append('cp "$work/out/%s" "%s"' % (rel, out.path))

    init_touches = []
    for rel in out_rel_paths:
        if rel.endswith("/__init__.py") or rel == "__init__.py":
            init_touches.append('mkdir -p "$work/out/$(dirname "%s")" && : >"$work/out/%s"' % (rel, rel))

    script = """set -eu
root="$PWD"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/src/{schema_dir}" "$work/src/buf/validate" "$work/out"
cp "{schema}" "$work/src/{schema_path}"
cp "{validate}" "$work/src/buf/validate/validate.proto"
"{protoc}" \\
  -I "$work/src" \\
  -I "$root/{wkp_root}" \\
  --python_out="$work/out" \\
  --plugin="protoc-gen-connect_py=$root/{plugin}" \\
  --connect_py_out="$work/out" \\
  "$work/src/buf/validate/validate.proto" \\
  "$work/src/{schema_path}"
{init_touches}
{moves}
""".format(
        schema = schema.path,
        schema_dir = schema_dir,
        schema_path = import_path,
        validate = ctx.file._validate.path,
        protoc = ctx.executable._protoc.path,
        wkp_root = wkp_root,
        plugin = ctx.executable._plugin.path,
        init_touches = "\n".join(init_touches),
        moves = "\n".join(moves),
    )

    ctx.actions.run_shell(
        command = script,
        inputs = [schema, ctx.file._validate] + wkp_files,
        outputs = ctx.outputs.outs,
        tools = [ctx.executable._protoc, ctx.attr._plugin[DefaultInfo].files_to_run],
        mnemonic = "PyProtoConnectGenerate",
        progress_message = "Generating Python Protobuf/Connect bindings for %{label}",
    )
    return [DefaultInfo(files = depset(ctx.outputs.outs))]

def _relative(ctx, out):
    parts = [part for part in [ctx.bin_dir.path, ctx.label.workspace_root, ctx.label.package] if part]
    prefix = "/".join(parts) + "/"
    return out.path[len(prefix):]

py_proto_connect_generate = rule(
    implementation = _py_proto_connect_generate_impl,
    doc = "Generates Python protobuf messages and Connect unary RPC client bindings via pinned protoc.",
    attrs = {
        "outs": attr.output_list(mandatory = True, doc = "Generated Python files relative to this package."),
        "schema": attr.label(mandatory = True, allow_single_file = [".proto"]),
        "schema_path": attr.string(mandatory = True, doc = "Import path of the proto schema."),
        "_plugin": attr.label(
            default = "//tools/proto:protoc_gen_connect_py",
            executable = True,
            cfg = "exec",
        ),
        "_protoc": attr.label(
            default = "//tools/proto:protoc",
            allow_single_file = True,
            executable = True,
            cfg = "exec",
        ),
        "_validate": attr.label(
            default = "@protovalidate_proto//file",
            allow_single_file = True,
        ),
        "_well_known_protos": attr.label(
            default = "//tools/proto:well_known_protos",
        ),
    },
)
