"""Hermetic protoc plugin generating typed Connect-over-HTTP unary Python clients (#29)."""

from __future__ import annotations

import re
import sys

from google.protobuf.compiler import plugin_pb2


def _camel_to_snake(name: str) -> str:
    step = re.sub(r"(.)([A-Z][a-z]+)", r"\1_\2", name)
    return re.sub(r"([a-z0-9])([A-Z])", r"\1_\2", step).lower()


def _short_type_name(full_type: str, package: str) -> str:
    prefix = f".{package}." if package else "."
    if full_type.startswith(prefix):
        return full_type[len(prefix) :]
    return full_type.lstrip(".")


def _render_connect_module(proto_file) -> str:
    package = proto_file.package
    module_base = proto_file.name.removesuffix(".proto").replace("/", ".")
    pb2_module = f"{module_base}_pb2"
    pb2_alias = pb2_module.rsplit(".", 1)[-1]
    pb2_parent = pb2_module.rsplit(".", 1)[0] if "." in pb2_module else ""
    import_stmt = f"from {pb2_parent} import {pb2_alias}" if pb2_parent else f"import {pb2_alias}"

    lines = [
        '"""Generated Connect unary RPC client bindings. Do not edit by hand."""',
        "",
        "from __future__ import annotations",
        "",
        "import base64",
        "import dataclasses",
        "import http.client",
        "import json",
        "import math",
        "import socket",
        "import urllib.error",
        "import urllib.parse",
        "import urllib.request",
        "from typing import TypeVar",
        "",
        "from buf.validate import validate_pb2",
        "from google.protobuf import message as _pb_message",
        import_stmt,
        "",
        '_RespT = TypeVar("_RespT", bound=_pb_message.Message)',
        '_LOOPBACK_HOSTS = frozenset({"127.0.0.1", "localhost", "::1"})',
        "_HTTP_STATUS_CODES = {",
        '    400: "invalid_argument",',
        '    401: "unauthenticated",',
        '    403: "permission_denied",',
        '    404: "not_found",',
        '    408: "deadline_exceeded",',
        '    409: "already_exists",',
        '    429: "resource_exhausted",',
        '    499: "canceled",',
        '    502: "unavailable",',
        '    503: "unavailable",',
        '    504: "deadline_exceeded",',
        "}",
        "",
        "",
        "@dataclasses.dataclass(frozen=True)",
        "class FieldViolation:",
        '    """One typed protovalidate field violation decoded from a Connect error detail."""',
        "",
        "    field: str",
        "    rule_id: str",
        "    message: str",
        "",
        "",
        "class ConnectError(Exception):",
        '    """Typed Connect RPC error preserving code, message, and protovalidate details."""',
        "",
        "    def __init__(",
        "        self,",
        "        code: str,",
        "        message: str,",
        "        *,",
        "        violations: tuple[FieldViolation, ...] = (),",
        "        http_status: int | None = None,",
        "    ) -> None:",
        "        super().__init__(f'{code}: {message}')",
        "        self.code = code",
        "        self.message = message",
        "        self.violations = violations",
        "        self.http_status = http_status",
        "",
        "",
        "def _decode_violations(details: object) -> tuple[FieldViolation, ...]:",
        "    if not isinstance(details, list):",
        "        return ()",
        "    found: list[FieldViolation] = []",
        "    for item in details:",
        "        if not isinstance(item, dict):",
        "            continue",
        '        if item.get("type") != "buf.validate.Violations":',
        "            continue",
        '        raw_val = item.get("value")',
        "        if not isinstance(raw_val, str) or not raw_val:",
        "            continue",
        "        padded = raw_val + '=' * (-len(raw_val) % 4)",
        "        try:",
        "            wire = base64.b64decode(padded)",
        "            parsed = validate_pb2.Violations()",
        "            parsed.ParseFromString(wire)",
        "        except Exception:",
        "            continue",
        "        for violation in parsed.violations:",
        "            parts = [elem.field_name for elem in violation.field.elements if elem.field_name]",
        "            field_path = '.'.join(parts)",
        "            found.append(",
        "                FieldViolation(",
        "                    field=field_path,",
        "                    rule_id=violation.rule_id,",
        "                    message=violation.message,",
        "                )",
        "            )",
        "    return tuple(found)",
        "",
        "",
        "def _parse_connect_http_error(err: urllib.error.HTTPError) -> ConnectError:",
        "    raw_body = b''",
        "    try:",
        "        raw_body = err.read()",
        "    except Exception:",
        "        raw_body = b''",
        "    finally:",
        "        try:",
        "            err.close()",
        "        except Exception:",
        "            pass",
        "    if 300 <= err.code < 400:",
        "        return ConnectError(",
        '            "failed_precondition",',
        '            f"RPC redirect refused (HTTP {err.code})",',
        "            http_status=err.code,",
        "        )",
        '    fallback_code = _HTTP_STATUS_CODES.get(err.code, "unknown")',
        '    fallback_msg = err.reason or f"HTTP {err.code}"',
        "    try:",
        '        payload = json.loads(raw_body.decode("utf-8"))',
        "    except Exception:",
        "        return ConnectError(fallback_code, fallback_msg, http_status=err.code)",
        "    if not isinstance(payload, dict):",
        "        return ConnectError(fallback_code, fallback_msg, http_status=err.code)",
        '    code = payload.get("code") if isinstance(payload.get("code"), str) else fallback_code',
        '    message = payload.get("message") if isinstance(payload.get("message"), str) else fallback_msg',
        '    violations = _decode_violations(payload.get("details"))',
        "    return ConnectError(code, message, violations=violations, http_status=err.code)",
        "",
        "",
        "class _NoRedirectHandler(urllib.request.HTTPRedirectHandler):",
        '    """Refuse HTTP redirects so RPC calls never cross origins or forward credentials."""',
        "",
        "    def http_error_302(self, req, fp, code, msg, headers):",
        "        raise urllib.error.HTTPError(req.full_url, code, msg, headers, fp)",
        "",
        "    http_error_301 = http_error_303 = http_error_307 = http_error_308 = http_error_302",
        "",
        "",
        "def _validate_endpoint(",
        "    endpoint: str,",
        "    *,",
        "    allow_test_identity: bool,",
        "    has_bearer_token: bool,",
        ") -> str:",
        "    cleaned = endpoint.strip().rstrip('/')",
        "    parsed = urllib.parse.urlparse(cleaned)",
        "    host = (parsed.hostname or '').lower()",
        "    if parsed.scheme not in ('http', 'https') or not parsed.netloc or not host:",
        '        raise ValueError("endpoint must be a valid http:// or https:// URL")',
        "    if parsed.username is not None or parsed.password is not None:",
        '        raise ValueError("endpoint URL must not embed userinfo credentials")',
        "    if parsed.query or parsed.fragment:",
        '        raise ValueError("endpoint URL must not include a query string or fragment")',
        "    if allow_test_identity and host not in _LOOPBACK_HOSTS:",
        '        raise ValueError("test_subject is restricted to loopback endpoints (127.0.0.1, localhost, ::1)")',
        "    if has_bearer_token and parsed.scheme != 'https' and host not in _LOOPBACK_HOSTS:",
        '        raise ValueError("bearer_token requires an https:// endpoint for non-loopback hosts")',
        "    return cleaned",
        "",
        "",
        "def _validate_connect_response(status: int, content_type: str, content_encoding: str) -> None:",
        "    if status != 200:",
        "        raise ConnectError(",
        '            "unknown",',
        '            f"unexpected HTTP status {status} for Connect unary response",',
        "            http_status=status,",
        "        )",
        "    media_type = content_type.split(';', 1)[0].strip().lower()",
        '    if media_type != "application/proto":',
        "        raise ConnectError(",
        '            "unknown",',
        "            f\"unexpected Connect response content-type {content_type!r}; expected 'application/proto'\",",
        "            http_status=status,",
        "        )",
        "    encoding = content_encoding.strip().lower()",
        '    if encoding and encoding != "identity":',
        "        raise ConnectError(",
        '            "unimplemented",',
        '            f"unsupported Connect response content-encoding {encoding!r}",',
        "            http_status=status,",
        "        )",
        "",
        "",
        "class _BaseConnectClient:",
        '    """Unary Connect binary Protobuf transport shared by generated service clients."""',
        "",
        "    def __init__(",
        "        self,",
        "        endpoint: str,",
        "        *,",
        "        bearer_token: str | None = None,",
        "        test_subject: str | None = None,",
        "        timeout_seconds: float = 5.0,",
        "    ) -> None:",
        "        if bearer_token and test_subject:",
        '            raise ValueError("specify at most one of bearer_token or test_subject")',
        '        if test_subject and ("\\r" in test_subject or "\\n" in test_subject):',
        '            raise ValueError("test_subject must not contain newline characters")',
        '        if bearer_token and ("\\r" in bearer_token or "\\n" in bearer_token):',
        '            raise ValueError("bearer_token must not contain newline characters")',
        "        if not math.isfinite(timeout_seconds) or timeout_seconds <= 0:",
        '            raise ValueError("timeout_seconds must be a positive finite number")',
        "        self._endpoint = _validate_endpoint(",
        "            endpoint,",
        "            allow_test_identity=bool(test_subject),",
        "            has_bearer_token=bool(bearer_token),",
        "        )",
        "        self._timeout_seconds = float(timeout_seconds)",
        "        if test_subject:",
        '            self._authorization: str | None = f"Bearer test-identity.{test_subject}"',
        "        elif bearer_token:",
        '            self._authorization = f"Bearer {bearer_token}"',
        "        else:",
        "            self._authorization = None",
        "        self._opener = urllib.request.build_opener(",
        "            urllib.request.ProxyHandler({}),",
        "            _NoRedirectHandler(),",
        "        )",
        "",
        "    def _unary(",
        "        self,",
        "        procedure: str,",
        "        request: _pb_message.Message,",
        "        response_cls: type[_RespT],",
        "        *,",
        "        timeout_seconds: float | None = None,",
        "    ) -> _RespT:",
        "        effective_timeout = self._timeout_seconds if timeout_seconds is None else float(timeout_seconds)",
        "        if not math.isfinite(effective_timeout) or effective_timeout <= 0:",
        '            raise ValueError("timeout_seconds must be a positive finite number")',
        "        headers = {",
        '            "Content-Type": "application/proto",',
        '            "Connect-Protocol-Version": "1",',
        '            "Connect-Timeout-Ms": str(max(1, int(effective_timeout * 1000))),',
        "        }",
        "        if self._authorization is not None:",
        '            headers["Authorization"] = self._authorization',
        "        req = urllib.request.Request(",
        '            url=f"{self._endpoint}{procedure}",',
        "            data=request.SerializeToString(),",
        "            headers=headers,",
        '            method="POST",',
        "        )",
        "        try:",
        "            with self._opener.open(req, timeout=effective_timeout) as resp:",
        "                status = resp.status",
        '                content_type = resp.headers.get("Content-Type", "")',
        '                content_encoding = resp.headers.get("Content-Encoding", "")',
        "                raw = resp.read()",
        "        except urllib.error.HTTPError as err:",
        "            raise _parse_connect_http_error(err) from None",
        "        except TimeoutError as err:",
        '            raise ConnectError("deadline_exceeded", f"request timed out: {err}") from None',
        "        except urllib.error.URLError as err:",
        "            if isinstance(err.reason, (TimeoutError, socket.timeout)):",
        '                raise ConnectError("deadline_exceeded", f"request timed out: {err.reason}") from None',
        '            raise ConnectError("unavailable", f"service unreachable: {err.reason}") from None',
        "        except http.client.IncompleteRead as err:",
        '            raise ConnectError("unavailable", f"incomplete response body: {err}") from None',
        "        except http.client.HTTPException as err:",
        '            raise ConnectError("unavailable", f"HTTP protocol error: {err}") from None',
        "        except OSError as err:",
        '            raise ConnectError("unavailable", f"service transport error: {err}") from None',
        "        _validate_connect_response(status, content_type, content_encoding)",
        "        message = response_cls()",
        "        try:",
        "            message.ParseFromString(raw)",
        "        except _pb_message.DecodeError as err:",
        "            raise ConnectError(",
        '                "internal",',
        '                f"malformed protobuf response payload: {err}",',
        "                http_status=status,",
        "            ) from None",
        "        return message",
    ]

    for service in proto_file.service:
        full_service = f"{package}.{service.name}" if package else service.name
        lines.extend(
            [
                "",
                "",
                f"class {service.name}Client(_BaseConnectClient):",
                f'    """Generated Connect client for `{full_service}`."""',
                "",
                f'    SERVICE_NAME = "{full_service}"',
            ]
        )
        for method in service.method:
            if method.client_streaming or method.server_streaming:
                continue
            py_method = _camel_to_snake(method.name)
            in_type = _short_type_name(method.input_type, package)
            out_type = _short_type_name(method.output_type, package)
            procedure = f"/{full_service}/{method.name}"
            lines.extend(
                [
                    "",
                    f"    def {py_method}(",
                    "        self,",
                    f"        request: {pb2_alias}.{in_type},",
                    "        *,",
                    "        timeout_seconds: float | None = None,",
                    f"    ) -> {pb2_alias}.{out_type}:",
                    "        return self._unary(",
                    f'            "{procedure}",',
                    "            request,",
                    f"            {pb2_alias}.{out_type},",
                    "            timeout_seconds=timeout_seconds,",
                    "        )",
                ]
            )

    lines.append("")
    return "\n".join(lines)


def main() -> int:
    request = plugin_pb2.CodeGeneratorRequest()
    request.ParseFromString(sys.stdin.buffer.read())

    targets = set(request.file_to_generate)
    response = plugin_pb2.CodeGeneratorResponse()
    for proto_file in request.proto_file:
        if proto_file.name not in targets or not proto_file.service:
            continue
        out_file = response.file.add()
        out_file.name = proto_file.name.removesuffix(".proto") + "_connect.py"
        out_file.content = _render_connect_module(proto_file)

    sys.stdout.buffer.write(response.SerializeToString())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
