"""Fetches the pinned browser's shared libraries and fonts (spec 005 research R1)."""

load(":debs.bzl", "DEBS")

def _browser_libs_impl(rctx):
    if rctx.os.name != "linux" or rctx.os.arch != "amd64":
        rctx.file("BUILD.bazel", """
filegroup(name = "fonts", srcs = [], visibility = ["//visibility:public"])
filegroup(name = "libs", srcs = [], visibility = ["//visibility:public"])
""")
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
                elif entry.basename.endswith(".ttf"):
                    rctx.symlink(entry.realpath, "fonts/" + entry.basename)
        level = directories
    rctx.file("BUILD.bazel", """
filegroup(name = "fonts", srcs = glob(["fonts/**"], allow_empty = False), visibility = ["//visibility:public"])
filegroup(name = "libs", srcs = glob(["lib/**"]), visibility = ["//visibility:public"])
""")

browser_libs = repository_rule(
    implementation = _browser_libs_impl,
    doc = "Downloads each package by sha256 and flattens its libraries into lib/ and fonts into fonts/.",
)

def _browser_tree_impl(ctx):
    tree = ctx.actions.declare_directory(ctx.label.name)
    ctx.actions.run_shell(
        inputs = ctx.files.browser + ctx.files.libs + ctx.files.fonts + [ctx.file.font_config],
        outputs = [tree],
        arguments = [tree.path, ctx.file.executable.dirname, ctx.file.font_config.path],
        command = """set -eu
tree="$1"
source="$2"
font_config="$3"
fontconfig_root="$tree/fontconfig-root"
mkdir -p "$tree/lib" "$fontconfig_root/etc/fonts" "$fontconfig_root/usr/share/fonts"
cp -RL "$source/." "$tree/"
cp -L "$font_config" "$fontconfig_root/etc/fonts/fonts.conf"
while IFS= read -r library; do cp -L "$library" "$tree/lib/"; done <<LIBRARIES
%s
LIBRARIES
while IFS= read -r font; do cp -L "$font" "$fontconfig_root/usr/share/fonts/"; done <<FONTS
%s
FONTS
""" % ("\n".join([f.path for f in ctx.files.libs]), "\n".join([f.path for f in ctx.files.fonts])),
        mnemonic = "BrowserTree",
        progress_message = "Assembling the pinned browser",
    )
    return [DefaultInfo(files = depset([tree]), runfiles = ctx.runfiles(files = [tree]))]

browser_tree = rule(
    implementation = _browser_tree_impl,
    doc = "One directory holding the browser, in lib/ every library it loads, and in fontconfig-root/ its fonts and their configuration.",
    attrs = {
        "browser": attr.label(allow_files = True, mandatory = True),
        "executable": attr.label(allow_single_file = True, mandatory = True),
        "font_config": attr.label(allow_single_file = True, mandatory = True),
        "fonts": attr.label(allow_files = True, mandatory = True),
        "libs": attr.label(allow_files = True, mandatory = True),
    },
)
