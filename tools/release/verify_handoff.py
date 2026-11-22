#!/usr/bin/env python3
"""Verify a credentialless release handoff directory before credentialed publishing."""

from __future__ import annotations

import argparse
import gzip
import hashlib
import json
import os
import re
import sys
import tarfile
import zlib
from pathlib import Path
from typing import Any

DIGEST_RE = re.compile(r"^sha256:([0-9a-f]{64})$")
REVISION_RE = re.compile(r"^[0-9a-f]{40}$")
TAG_RE = re.compile(r"^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$")
SUM_LINE_RE = re.compile(r"^([0-9a-f]{64})  (?:\./)?([A-Za-z0-9._-]+)$")
ARCHIVE_CFG_RE = re.compile(r"^(?:blobs/sha256/)?[0-9a-f]{64}(?:\.json)?$")
UNCOMPRESSED_LAYER_MEDIA_TYPES = frozenset(
    {
        "application/vnd.oci.image.layer.v1.tar",
        "application/vnd.docker.image.rootfs.diff.tar",
    }
)
GZIP_LAYER_MEDIA_TYPES = frozenset(
    {
        "application/vnd.oci.image.layer.v1.tar+gzip",
        "application/vnd.docker.image.rootfs.diff.tar.gzip",
    }
)


class HandoffError(Exception):
    """Raised when a release handoff artifact fails integrity or binding checks."""


def sha256_bytes(data: bytes) -> str:
    """Return a sha256:<hex> digest for raw bytes."""
    return f"sha256:{hashlib.sha256(data).hexdigest()}"


def sha256_file(path: Path) -> str:
    """Return lowercase hex SHA-256 for a regular non-symlink file."""
    if path.is_symlink() or not path.is_file():
        raise HandoffError(f"verify-handoff: expected regular file: {path.name}")
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        while chunk := handle.read(65536):
            digest.update(chunk)
    return digest.hexdigest()


def parse_checksum_file(path: Path, expected_names: set[str], label: str) -> dict[str, str]:
    """Parse a deterministic sha256sum manifest and require exact filename set."""
    if path.is_symlink() or not path.is_file():
        raise HandoffError(f"verify-handoff: missing {label}")
    parsed: dict[str, str] = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        match = SUM_LINE_RE.match(line)
        if not match:
            raise HandoffError(f"verify-handoff: malformed line in {label}")
        hex_digest, name = match.groups()
        if name in parsed:
            raise HandoffError(f"verify-handoff: duplicate entry {name} in {label}")
        parsed[name] = hex_digest
    if set(parsed) != expected_names:
        raise HandoffError(f"verify-handoff: {label} filenames do not match expected set")
    return parsed


def verify_checksums(handoff: Path, sums: dict[str, str], label: str) -> None:
    """Verify every listed regular non-symlink file matches its expected SHA-256."""
    for name, expected in sums.items():
        actual = sha256_file(handoff / name)
        if actual != expected:
            raise HandoffError(f"verify-handoff: checksum mismatch for {name} in {label}")


def normalize_tar_path(raw: str) -> str:
    """Normalize a tar member path and reject traversal or absolute paths."""
    if "\0" in raw or "\\" in raw or raw.startswith("/"):
        raise HandoffError(f"verify-handoff: unsafe archive path {raw!r}")
    parts = [part for part in raw.split("/") if part not in ("", ".")]
    if any(part == ".." for part in parts):
        raise HandoffError(f"verify-handoff: path traversal in archive {raw!r}")
    return "/".join(parts)


def read_tar_files(archive: Path, extract_to: Path | None = None) -> dict[str, bytes]:
    """Validate every tar member safely, optionally extract regular files/dirs, and return regular file map."""
    if archive.is_symlink() or not archive.is_file():
        raise HandoffError(f"verify-handoff: missing archive {archive.name}")
    if extract_to is not None:
        if extract_to.is_symlink():
            raise HandoffError("verify-handoff: extract target must not be a symlink")
        if extract_to.exists() and any(extract_to.iterdir()):
            raise HandoffError("verify-handoff: extract target directory must be empty")
        extract_to.mkdir(parents=True, exist_ok=True, mode=0o700)
        os.chmod(extract_to, 0o700)
    files: dict[str, bytes] = {}
    root_resolved = extract_to.resolve() if extract_to is not None else None
    mode = "r:gz" if archive.name.endswith(".gz") else "r:"
    with tarfile.open(archive, mode) as tar:
        for member in tar.getmembers():
            rel = normalize_tar_path(member.name)
            if member.isdir():
                if rel and extract_to is not None and root_resolved is not None:
                    dest_dir = (extract_to / rel).resolve()
                    dest_dir.relative_to(root_resolved)
                    dest_dir.mkdir(parents=True, exist_ok=True, mode=0o700)
                continue
            if not member.isfile() or not rel:
                raise HandoffError(f"verify-handoff: unsupported tar member {member.name!r} in {archive.name}")
            if rel in files:
                raise HandoffError(f"verify-handoff: duplicate tar entry {rel} in {archive.name}")
            stream = tar.extractfile(member)
            if stream is None:
                raise HandoffError(f"verify-handoff: unreadable tar entry {rel} in {archive.name}")
            payload = stream.read()
            files[rel] = payload
            if extract_to is not None and root_resolved is not None:
                dest = (extract_to / rel).resolve()
                dest.relative_to(root_resolved)
                dest.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
                dest.write_bytes(payload)
                os.chmod(dest, 0o600)
    return files


def verify_blob(files: dict[str, bytes], descriptor: Any, label: str) -> bytes:
    """Verify an OCI descriptor resolves in the layout with exact byte size and SHA-256."""
    if not isinstance(descriptor, dict):
        raise HandoffError(f"verify-handoff: {label} descriptor is not an object")
    digest = descriptor.get("digest")
    size = descriptor.get("size")
    if not isinstance(digest, str):
        raise HandoffError(f"verify-handoff: {label} digest missing")
    match = DIGEST_RE.match(digest)
    if not match or not isinstance(size, int) or isinstance(size, bool) or size < 0:
        raise HandoffError(f"verify-handoff: invalid {label} descriptor")
    blob_path = f"blobs/sha256/{match.group(1)}"
    data = files.get(blob_path)
    if data is None:
        raise HandoffError(f"verify-handoff: missing {label} blob {blob_path}")
    if len(data) != size or sha256_bytes(data) != digest:
        raise HandoffError(f"verify-handoff: {label} blob digest or size mismatch")
    return data


def verify_oci_layer_diff_id(layer_desc: dict[str, Any], layer_bytes: bytes, diff_id: Any, idx: int) -> str:
    """Verify an OCI layer uses a supported mediaType and its uncompressed bytes match diff_id."""
    if not isinstance(diff_id, str) or not DIGEST_RE.match(diff_id):
        raise HandoffError(f"verify-handoff: invalid OCI rootfs.diff_ids[{idx}]")
    media_type = layer_desc.get("mediaType")
    if media_type in UNCOMPRESSED_LAYER_MEDIA_TYPES:
        if sha256_bytes(layer_bytes) != diff_id:
            raise HandoffError(f"verify-handoff: OCI layer[{idx}] digest does not match diff_id")
        return "uncompressed"
    if media_type in GZIP_LAYER_MEDIA_TYPES:
        try:
            uncompressed = gzip.decompress(layer_bytes)
        except (OSError, EOFError, zlib.error) as exc:
            raise HandoffError(f"verify-handoff: invalid gzip payload in OCI layer[{idx}]") from exc
        if sha256_bytes(uncompressed) != diff_id:
            raise HandoffError(f"verify-handoff: OCI gzip layer[{idx}] content does not match diff_id")
        return "gzip"
    raise HandoffError(f"verify-handoff: unsupported OCI layer[{idx}] mediaType {media_type!r}")


def verify_gzip_tar_layer(tar_layer_bytes: bytes, oci_layer_digest: str, diff_id: str, idx: int) -> None:
    """Verify a Docker tarball layer against a verified gzip OCI layer and its uncompressed diff_id."""
    tar_layer_digest = sha256_bytes(tar_layer_bytes)
    if tar_layer_digest in (diff_id, oci_layer_digest):
        return
    if tar_layer_bytes[:2] == b"\x1f\x8b":
        try:
            uncompressed = gzip.decompress(tar_layer_bytes)
        except (OSError, EOFError, zlib.error) as exc:
            raise HandoffError(f"verify-handoff: invalid gzip payload in release tarball layer[{idx}]") from exc
        if sha256_bytes(uncompressed) == diff_id:
            return
    raise HandoffError(f"verify-handoff: release tarball layer[{idx}] content does not match OCI layer")


def verify_single_tarball_layer(
    tarball_files: dict[str, bytes],
    tar_entry: Any,
    layer_desc: dict[str, Any],
    oci_bytes: bytes,
    diff_id: Any,
    idx: int,
) -> None:
    """Verify one ordered Docker archive layer entry resolves to a safe regular member bound to diff_id."""
    encoding = verify_oci_layer_diff_id(layer_desc, oci_bytes, diff_id, idx)
    if not isinstance(tar_entry, str):
        raise HandoffError(f"verify-handoff: release tarball Layers[{idx}] is not a string")
    layer_rel = normalize_tar_path(tar_entry)
    if not layer_rel:
        raise HandoffError(f"verify-handoff: empty release tarball layer path at index {idx}")
    tar_layer_bytes = tarball_files.get(layer_rel)
    if tar_layer_bytes is None:
        raise HandoffError(f"verify-handoff: release tarball missing layer member {layer_rel!r}")
    if encoding == "gzip":
        verify_gzip_tar_layer(tar_layer_bytes, layer_desc["digest"], diff_id, idx)
        return
    if len(tar_layer_bytes) != len(oci_bytes) or sha256_bytes(tar_layer_bytes) != diff_id:
        raise HandoffError(f"verify-handoff: release tarball layer[{idx}] content does not match diff_id")


def verify_tarball_layers(
    tarball_files: dict[str, bytes],
    tar_layers: list[Any],
    oci_layers: list[tuple[dict[str, Any], bytes]],
    diff_ids: list[Any],
) -> None:
    """Verify each ordered Docker archive Layers entry resolves to a regular member bound to diff_id and OCI layer."""
    if len(tar_layers) != len(oci_layers) or len(tar_layers) != len(diff_ids):
        raise HandoffError("verify-handoff: release tarball layer count does not match OCI layout")
    for idx, (tar_entry, (layer_desc, oci_bytes), diff_id) in enumerate(
        zip(tar_layers, oci_layers, diff_ids, strict=True)
    ):
        verify_single_tarball_layer(tarball_files, tar_entry, layer_desc, oci_bytes, diff_id, idx)


def verify_layout_and_tarball(
    layout_files: dict[str, bytes], tarball_files: dict[str, bytes], expected_digest: str
) -> None:
    """Verify OCI layout closure matches expected_digest and binds to the release Docker tarball."""
    oci_layout = json.loads(layout_files.get("oci-layout", b"{}").decode("utf-8"))
    if oci_layout.get("imageLayoutVersion") != "1.0.0":
        raise HandoffError("verify-handoff: missing or unsupported oci-layout version")

    index_raw = layout_files.get("index.json")
    if index_raw is None:
        raise HandoffError("verify-handoff: missing index.json in OCI layout")
    index = json.loads(index_raw.decode("utf-8"))
    manifests = index.get("manifests")
    if not isinstance(manifests, list) or len(manifests) != 1:
        raise HandoffError("verify-handoff: index.json must describe exactly one manifest")
    root_desc = manifests[0]
    if not isinstance(root_desc, dict) or root_desc.get("digest") != expected_digest:
        raise HandoffError("verify-handoff: OCI index manifest digest does not match image-digest file")

    manifest_bytes = verify_blob(layout_files, root_desc, "manifest")
    manifest = json.loads(manifest_bytes.decode("utf-8"))
    config_desc = manifest.get("config")
    config_bytes = verify_blob(layout_files, config_desc, "config")
    layers = manifest.get("layers")
    if not isinstance(layers, list) or not layers:
        raise HandoffError("verify-handoff: manifest must declare at least one layer")
    oci_layers: list[tuple[dict[str, Any], bytes]] = []
    for idx, layer in enumerate(layers):
        layer_bytes = verify_blob(layout_files, layer, f"layer[{idx}]")
        oci_layers.append((layer, layer_bytes))

    oci_cfg = json.loads(config_bytes.decode("utf-8"))
    oci_rootfs = oci_cfg.get("rootfs")
    if not isinstance(oci_rootfs, dict) or oci_rootfs.get("type") != "layers":
        raise HandoffError("verify-handoff: OCI config missing rootfs.layers")
    diff_ids = oci_rootfs.get("diff_ids")
    if not isinstance(diff_ids, list) or len(diff_ids) != len(oci_layers):
        raise HandoffError("verify-handoff: OCI rootfs.diff_ids count mismatches layer count")

    tar_manifest_raw = tarball_files.get("manifest.json")
    if tar_manifest_raw is None:
        raise HandoffError("verify-handoff: release tarball missing manifest.json")
    tar_manifests = json.loads(tar_manifest_raw.decode("utf-8"))
    if not isinstance(tar_manifests, list) or len(tar_manifests) != 1 or not isinstance(tar_manifests[0], dict):
        raise HandoffError("verify-handoff: release tarball must describe one image")
    cfg_entry = tar_manifests[0].get("Config")
    tar_layers = tar_manifests[0].get("Layers")
    if not isinstance(cfg_entry, str) or not isinstance(tar_layers, list):
        raise HandoffError("verify-handoff: release tarball manifest missing Config or Layers")
    cfg_rel = normalize_tar_path(cfg_entry)
    if not ARCHIVE_CFG_RE.match(cfg_rel):
        raise HandoffError("verify-handoff: unexpected release tarball config path")
    tar_cfg_bytes = tarball_files.get(cfg_rel)
    if tar_cfg_bytes is None:
        raise HandoffError("verify-handoff: release tarball config blob missing")
    if sha256_bytes(tar_cfg_bytes) != config_desc["digest"]:
        raise HandoffError("verify-handoff: release tarball config digest does not match OCI layout")
    tar_cfg = json.loads(tar_cfg_bytes.decode("utf-8"))
    if tar_cfg.get("rootfs") != oci_rootfs:
        raise HandoffError("verify-handoff: release tarball rootfs does not match OCI layout")
    verify_tarball_layers(tarball_files, tar_layers, oci_layers, diff_ids)


def verify_handoff(handoff: Path, tag: str, revision: str, extract_layout: Path | None) -> str:
    """Validate complete handoff directory and return the verified sha256:<hex> image digest."""
    if not TAG_RE.match(tag):
        raise HandoffError(f"verify-handoff: invalid release tag {tag!r}")
    if not REVISION_RE.match(revision):
        raise HandoffError(f"verify-handoff: invalid source revision {revision!r}")
    if handoff.is_symlink() or not handoff.is_dir():
        raise HandoffError("verify-handoff: handoff directory does not exist or is a symlink")

    public_assets = {
        "LICENSE",
        f"bff-image-{tag}.tar",
        "go-x509roots-BSD-3-Clause.txt",
        f"image-digest-{tag}.txt",
        f"provenance-{tag}.md",
        f"runtime-acceptance-{tag}.tar.gz",
        f"source-revision-{tag}.txt",
        f"verify-receipt-{tag}.json",
    }
    handoff_files = public_assets | {"SHA256SUMS", f"image-layout-{tag}.tar.gz"}

    actual_entries = {entry.name for entry in handoff.iterdir()} - {"HANDOFF.sha256"}
    if actual_entries != handoff_files:
        raise HandoffError("verify-handoff: handoff directory entries do not match expected asset set")

    handoff_sums = parse_checksum_file(handoff / "HANDOFF.sha256", handoff_files, "HANDOFF.sha256")
    verify_checksums(handoff, handoff_sums, "HANDOFF.sha256")

    public_sums = parse_checksum_file(handoff / "SHA256SUMS", public_assets, "SHA256SUMS")
    verify_checksums(handoff, public_sums, "SHA256SUMS")

    recorded_rev = (handoff / f"source-revision-{tag}.txt").read_text(encoding="utf-8").strip()
    if recorded_rev != revision:
        raise HandoffError("verify-handoff: source-revision file does not match checked-out revision")

    expected_digest = (handoff / f"image-digest-{tag}.txt").read_text(encoding="utf-8").strip()
    if not DIGEST_RE.match(expected_digest):
        raise HandoffError("verify-handoff: image-digest file does not contain a sha256 digest")

    layout_files = read_tar_files(handoff / f"image-layout-{tag}.tar.gz", extract_to=extract_layout)
    tarball_files = read_tar_files(handoff / f"bff-image-{tag}.tar")
    verify_layout_and_tarball(layout_files, tarball_files, expected_digest)
    return expected_digest


def main(argv: list[str] | None = None) -> int:
    """CLI entry point returning 0 on verified handoff and 1 on any rejection."""
    parser = argparse.ArgumentParser(description="Verify credentialless release handoff.")
    parser.add_argument("--handoff", required=True, type=Path)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--revision", required=True)
    parser.add_argument("--extract-layout", type=Path, default=None)
    args = parser.parse_args(argv)
    try:
        digest = verify_handoff(args.handoff, args.tag, args.revision, args.extract_layout)
    except (OSError, ValueError, KeyError, json.JSONDecodeError, tarfile.TarError, HandoffError) as exc:
        print(str(exc), file=sys.stderr)
        return 1
    print(digest)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
