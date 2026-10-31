"""Build-time code generation with the pinned buf (specs/003-notes-contract research R1)."""

def _buf_generate_impl(ctx):
    plugin = ctx.executable.plugin
    template = {
        "version": "v2",
        "plugins": [{
            "local": "@PLUGIN@",
            "out": "@OUT@",
            "opt": ctx.attr.opts,
        }],
    }
    schema = ctx.file.schema
    import_path = ctx.attr.schema_path
    moves = "\n".join([
        'cp "$work/out/%s" "%s"' % (_relative(ctx, out), out.path)
        for out in ctx.outputs.outs
    ])
    script = """set -eu
root="$PWD"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/src/{schema_dir}" "$work/src/buf/validate" "$work/home"
cp "{schema}" "$work/src/{schema_path}"
cp "{validate}" "$work/src/buf/validate/validate.proto"
cp "{config}" "$work/src/buf.yaml"
template='{template}'
template="${{template//@PLUGIN@/$root/{plugin}}}"
template="${{template//@OUT@/$work/out}}"
HOME="$work/home" BUF_CACHE_DIR="$work/home/cache" "{buf}" generate "$work/src" --template "$template" --path "$work/src/{schema_path}" {include}
{moves}
""".format(
        schema = schema.path,
        schema_dir = import_path.rsplit("/", 1)[0],
        schema_path = import_path,
        validate = ctx.file._validate.path,
        config = ctx.file._config.path,
        template = json.encode(template),
        plugin = plugin.path,
        buf = ctx.executable._buf.path,
        include = "--include-imports" if ctx.attr.include_imports else "",
        moves = moves,
    )
    ctx.actions.run_shell(
        command = script,
        inputs = [schema, ctx.file._validate, ctx.file._config],
        outputs = ctx.outputs.outs,
        tools = [plugin, ctx.executable._buf],
        env = {"BAZEL_BINDIR": ctx.bin_dir.path},
        mnemonic = "BufGenerate",
        progress_message = "Generating %{label}",
    )
    return [DefaultInfo(files = depset(ctx.outputs.outs))]

def _relative(ctx, out):
    prefix = ctx.label.package + "/" if ctx.label.package else ""
    path = out.short_path
    return path[len(prefix):] if path.startswith(prefix) else path

buf_generate = rule(
    implementation = _buf_generate_impl,
    doc = "Runs one protoc plugin over one schema with `buf generate`. Actions have no network.",
    attrs = {
        "include_imports": attr.bool(doc = "Also generate imported schemas."),
        "opts": attr.string_list(doc = "Plugin options."),
        "outs": attr.output_list(mandatory = True, doc = "Generated files, as the plugin names them."),
        "plugin": attr.label(mandatory = True, executable = True, cfg = "exec"),
        "schema": attr.label(mandatory = True, allow_single_file = [".proto"]),
        "schema_path": attr.string(mandatory = True, doc = "The schema's path in the buf module."),
        "_buf": attr.label(default = "@bff_template_quality//tools:buf", executable = True, cfg = "exec"),
        "_config": attr.label(default = "@bff_template_quality//proto:buf.yaml", allow_single_file = True),
        "_validate": attr.label(default = "@protovalidate_proto//file", allow_single_file = True),
    },
)
