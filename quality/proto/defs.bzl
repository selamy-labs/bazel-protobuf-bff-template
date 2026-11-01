"""Build-time code generation with the pinned buf (specs/003-notes-contract research R1).

Lives in the quality module so its own registry schema uses the same rule (spec 006 R4).
"""

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
mkdir -p "$work/src/{schema_dir}" "$work/home"
cp "{schema}" "$work/src/{schema_path}"
{imports}
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
        imports = (
            'mkdir -p "$work/src/buf/validate"\ncp "%s" "$work/src/buf/validate/validate.proto"' % ctx.file.validate.path
        ) if ctx.file.validate else "",
        config = ctx.file._config.path,
        template = json.encode(template),
        plugin = plugin.path,
        buf = ctx.executable._buf.path,
        include = "--include-imports" if ctx.attr.include_imports else "",
        moves = moves,
    )
    ctx.actions.run_shell(
        command = script,
        inputs = [schema, ctx.file._config] + ([ctx.file.validate] if ctx.file.validate else []),
        outputs = ctx.outputs.outs,
        tools = [plugin, ctx.executable._buf],
        env = {"BAZEL_BINDIR": ctx.bin_dir.path},
        mnemonic = "BufGenerate",
        progress_message = "Generating %{label}",
    )
    return [DefaultInfo(files = depset(ctx.outputs.outs))]

def _relative(ctx, out):
    parts = [part for part in [ctx.bin_dir.path, ctx.label.workspace_root, ctx.label.package] if part]
    prefix = "/".join(parts) + "/"
    return out.path[len(prefix):]

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
        "validate": attr.label(allow_single_file = True, doc = "The buf/validate schema, for a schema that imports it."),
        "_buf": attr.label(default = "@bff_template_quality//tools:buf", executable = True, cfg = "exec"),
        "_config": attr.label(default = "@bff_template_quality//proto:buf.yaml", allow_single_file = True),
    },
)
