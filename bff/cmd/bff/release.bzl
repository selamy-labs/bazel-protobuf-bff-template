"""The production binary for packaging, built in the plain configuration (spec 009 FR-005, FR-006).

`tools/verify.sh` runs `bazel coverage`, which sets two options that reach every target under `//bff`:
`collect_code_coverage`, which instruments the server, and `instrumentation_filter`, which also changes the
output directory of the generated sources whose paths the Go binary records. Measured on the binaries, those
two were the only differences between the plain and coverage builds (spec 009 research R6). This transition
resets exactly those two to Bazel's defaults for the packaged binary's subgraph, and nothing else: ordinary
targets keep their coverage, and the server inside the image is uninstrumented under either command.

The result is not byte-identical across the two commands: with the same options, Bazel names a transitioned
output directory by its difference from the command's own top-level configuration, and the Go binary records
that directory in the paths of its generated sources. So the image's identity is the digest of a plain build,
which the `image-release` check builds, witnesses and reports (spec 009 contracts/image.md)."""

# Bazel's default for --instrumentation_filter.
_DEFAULT_INSTRUMENTATION_FILTER = "-/javatests[/:],-/test/java[/:]"

def _plain_impl(_settings, _attr):
    return {
        "//command_line_option:collect_code_coverage": False,
        "//command_line_option:instrumentation_filter": _DEFAULT_INSTRUMENTATION_FILTER,
    }

_plain = transition(
    implementation = _plain_impl,
    inputs = [],
    outputs = [
        "//command_line_option:collect_code_coverage",
        "//command_line_option:instrumentation_filter",
    ],
)

def _release_binary_impl(ctx):
    binary = ctx.attr.binary[0]
    executable = binary[DefaultInfo].files_to_run.executable
    return [DefaultInfo(files = depset([executable]), executable = None)]

release_binary = rule(
    implementation = _release_binary_impl,
    attrs = {
        "binary": attr.label(
            cfg = _plain,
            mandatory = True,
            doc = "The binary to package, built in the plain configuration.",
        ),
    },
    doc = "Forwards a binary's executable from the plain configuration.",
)
