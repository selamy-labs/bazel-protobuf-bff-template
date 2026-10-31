"""Fetches the pinned browser's shared libraries into one directory (spec 005 research R1)."""

load(":debs.bzl", "DEBS")

def _browser_libs_impl(rctx):
    if rctx.os.name != "linux" or rctx.os.arch != "amd64":
        rctx.file("BUILD.bazel", 'filegroup(name = "libs", srcs = [], visibility = ["//visibility:public"])\n')
        return
    for name, (urls, sha256) in sorted(DEBS.items()):
        # A .deb is an ar archive that holds a compressed tar of its files.
        rctx.download_and_extract(urls, output = "ar/" + name, sha256 = sha256, type = "deb")
        members = [m for m in rctx.path("ar/" + name).readdir() if m.basename.startswith("data.tar")]
        if len(members) != 1:
            fail("%s: expected one data.tar member, found %s" % (name, members))
        rctx.extract(members[0], output = "debs/" + name)
    level = [rctx.path("debs")]
    for _ in range(12):  # Starlark has no while; package trees are far shallower than this.
        directories = []
        for directory in level:
            for entry in directory.readdir():
                if entry.is_dir:
                    directories.append(entry)
                elif ".so" in entry.basename and entry.exists:
                    rctx.symlink(entry.realpath, "lib/" + entry.basename)
        level = directories
    rctx.file("BUILD.bazel", 'filegroup(name = "libs", srcs = glob(["lib/**"]), visibility = ["//visibility:public"])\n')

browser_libs = repository_rule(
    implementation = _browser_libs_impl,
    doc = "Downloads each package by sha256 and flattens its shared libraries into lib/.",
)

def _browser_tree_impl(ctx):
    tree = ctx.actions.declare_directory(ctx.label.name)
    ctx.actions.run_shell(
        inputs = ctx.files.browser + ctx.files.libs,
        outputs = [tree],
        arguments = [tree.path, ctx.file.executable.dirname],
        command = """set -eu
tree="$1"
source="$2"
shift 2
mkdir -p "$tree/lib"
cp -RL "$source/." "$tree/"
while IFS= read -r library; do cp -L "$library" "$tree/lib/"; done <<LIBRARIES
%s
LIBRARIES
""" % "\n".join([f.path for f in ctx.files.libs]),
        mnemonic = "BrowserTree",
        progress_message = "Assembling the pinned browser",
    )
    return [DefaultInfo(files = depset([tree]), runfiles = ctx.runfiles(files = [tree]))]

browser_tree = rule(
    implementation = _browser_tree_impl,
    doc = "One directory holding the browser and, in lib/, every library it loads.",
    attrs = {
        "browser": attr.label(allow_files = True, mandatory = True),
        "executable": attr.label(allow_single_file = True, mandatory = True),
        "libs": attr.label(allow_files = True, mandatory = True),
    },
)
