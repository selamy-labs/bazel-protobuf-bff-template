"""The production binary for packaging, built without coverage instrumentation (spec 009 FR-005).

`tools/verify.sh` runs `bazel coverage`, which instruments every target under `//bff`. An image built in that
configuration would hold an instrumented server, a different artifact whose stderr carries coverage warnings.
This transition resets the one option, so the image holds the same binary a plain build produces."""

def _uninstrumented_impl(_settings, _attr):
    return {"//command_line_option:collect_code_coverage": False}

_uninstrumented = transition(
    implementation = _uninstrumented_impl,
    inputs = [],
    outputs = ["//command_line_option:collect_code_coverage"],
)

def _release_binary_impl(ctx):
    binary = ctx.attr.binary[0]
    executable = binary[DefaultInfo].files_to_run.executable
    return [DefaultInfo(files = depset([executable]), executable = None)]

release_binary = rule(
    implementation = _release_binary_impl,
    attrs = {
        "binary": attr.label(
            cfg = _uninstrumented,
            mandatory = True,
            doc = "The binary to package, rebuilt with coverage instrumentation off.",
        ),
    },
    doc = "Forwards a binary's executable from the uninstrumented configuration.",
)
