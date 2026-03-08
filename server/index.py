# -*- coding: utf-8 -*-
import base64
import json
import ssl
import urllib.error
import urllib.request


def handler(event, context):
    try:
        data = parse_payload(event)
        if not isinstance(data, dict):
            return mk_response(400, {"error": "Request payload must be a JSON object"})

        method = data.get("method", "GET")
        url = data.get("url", "")
        headers = {
            str(key): str(value)
            for key, value in (data.get("headers", {}) or {}).items()
        }
        body_content = data.get("body", "")

        if not url and isinstance(data.get("body"), str):
            try:
                nested = json.loads(data["body"])
                if isinstance(nested, dict):
                    data = nested
                    method = data.get("method", "GET")
                    url = data.get("url", "")
                    headers = {
                        str(key): str(value)
                        for key, value in (data.get("headers", {}) or {}).items()
                    }
                    body_content = data.get("body", "")
            except Exception:
                pass

        if not url:
            debug = {"keys": list(data.keys())[:20]}
            if "body" in data:
                debug["body_preview"] = str(data.get("body"))[:300]
            return mk_response(400, {"error": "Missing URL", "debug": debug})

        payload = None
        if body_content:
            if data.get("is_body_base64", False):
                payload = base64.b64decode(body_content)
            else:
                payload = body_content.encode("utf-8")

        request = urllib.request.Request(url, data=payload, headers=headers, method=method)

        ctx = ssl.create_default_context()
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE

        try:
            with urllib.request.urlopen(request, timeout=10, context=ctx) as response:
                content = response.read()
                return mk_proxy_response(response.getcode(), dict(response.info()), content)
        except urllib.error.HTTPError as exc:
            content = exc.read()
            return mk_proxy_response(exc.code, dict(exc.headers), content)
        except urllib.error.URLError as exc:
            return mk_response(502, {"error": f"Upstream Error: {exc.reason}"})
    except ValueError as exc:
        return mk_response(400, {"error": str(exc)})
    except Exception as exc:
        return mk_response(500, {"error": f"Internal Server Error: {exc}"})


def parse_payload(event):
    if isinstance(event, dict):
        if "url" in event:
            return event
        if "body" in event:
            raw_body = event.get("body") or ""
            if event.get("isBase64Encoded", False):
                raw_body = base64.b64decode(raw_body).decode("utf-8")
            if isinstance(raw_body, (bytes, bytearray)):
                raw_body = raw_body.decode("utf-8")
            if isinstance(raw_body, str) and raw_body.strip():
                return json.loads(raw_body)
            if looks_like_gateway_event(event):
                raise ValueError("Empty Request Body")
        return event

    if isinstance(event, (bytes, bytearray)):
        event = event.decode("utf-8")

    if isinstance(event, str):
        if not event.strip():
            raise ValueError("Empty Request Body")
        return json.loads(event)

    raise ValueError("Unsupported event type")


def looks_like_gateway_event(event):
    return any(
        key in event
        for key in ("version", "rawPath", "requestContext", "headers", "queryParameters")
    )


def mk_proxy_response(status, headers, content_bytes):
    return mk_response(
        200,
        {
            "status_code": status,
            "headers": headers,
            "content": base64.b64encode(content_bytes).decode("utf-8"),
            "is_content_base64": True,
        },
    )


def mk_response(status, body_dict):
    return {
        "isBase64Encoded": False,
        "statusCode": status,
        "headers": {"Content-Type": "application/json"},
        "body": json.dumps(body_dict),
    }


main_handler = handler
