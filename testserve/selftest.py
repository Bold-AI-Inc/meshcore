import base64
import json
import pathlib
import re
import sys
import urllib.request

ROOT = pathlib.Path(__file__).parent
SEED = json.loads((ROOT / "seed" / "providers.json").read_text())
PNG = base64.b64encode((ROOT / "files" / "dummy.png").read_bytes()).decode()
PLACEHOLDER = re.compile(r"\{\{([A-Za-z0-9_]+)\}\}")


def render(template, vars, raw_vars):
    text = json.dumps(template)
    text = PLACEHOLDER.sub(lambda m: json.dumps(vars[m.group(1)])[1:-1] if m.group(1) in vars else m.group(0), text)
    for key, value in raw_vars.items():
        text = text.replace(f'"{{{{{key}}}}}"', json.dumps(value))
    return text.encode()


def navigate(value, path):
    for segment in path.split("."):
        if segment.isdigit():
            if not isinstance(value, list) or int(segment) >= len(value):
                return None
            value = value[int(segment)]
        else:
            if not isinstance(value, dict) or segment not in value:
                return None
            value = value[segment]
    return value


def content_blocks(family):
    if family == "anthropic":
        return [{"type": "text", "text": "hello"},
                {"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": PNG}}]
    if family == "gemini":
        return [{"text": "hello"}, {"inline_data": {"mime_type": "image/png", "data": PNG}}]
    return [{"type": "text", "text": "hello"},
            {"type": "image_url", "image_url": {"url": f"data:image/png;base64,{PNG}"}}]


def call(url, headers, body, stream=False):
    request = urllib.request.Request(url, data=body, headers=dict(headers, **{"Content-Type": "application/json"}))
    with urllib.request.urlopen(request, timeout=60) as response:
        payload = response.read().decode()
    if not stream:
        return json.loads(payload)
    events = []
    for line in payload.splitlines():
        line = line.strip()
        if not line.startswith("data:"):
            continue
        chunk = line[5:].strip()
        if chunk and chunk not in ("[DONE]", "END_OF_STREAM"):
            try:
                events.append(json.loads(chunk))
            except ValueError:
                pass
    return events


def check(label, ok, detail=""):
    print(f"{'PASS' if ok else 'FAIL'}  {label}  {detail}")
    return ok


def chat(provider, key, model):
    family = provider["provider_family"]
    vars = {"api_key": key, "model": model, "query": "hello"}
    url = PLACEHOLDER.sub(lambda m: vars.get(m.group(1), m.group(0)), provider["endpoint_url"])
    headers = json.loads(render(provider["header_template"], vars, {}))
    body = render(provider["request_body_template"], {"model": model, "query": "hello"},
                  {"content_json": content_blocks(family), "max_tokens_json": 512})
    if b"{{" in body or b"testserve-key" in body:
        return check(f"{provider['name']} body-hygiene", False, body[:120].decode())
    events = call(url, headers, body, stream=True)
    text = "".join(str(navigate(e, provider["response_delta_path"]) or "") for e in events)
    usage = {}
    for field in ("usage_input_tokens_path", "usage_output_tokens_path"):
        if provider.get(field):
            values = [navigate(e, provider[field]) for e in events]
            values = [v for v in values if isinstance(v, (int, float))]
            usage[field.split("_")[1]] = values[-1] if values else None
    ok = bool(text) and "image(image/png,70B" in text
    return check(f"{provider['name']} chat[{family}]", ok, f"text={text[:60]!r} usage={usage}")


def embeddings(provider, key, model):
    vars = {"api_key": key, "model": model}
    url = PLACEHOLDER.sub(lambda m: vars.get(m.group(1), m.group(0)), provider["embedding_endpoint_url"])
    headers = json.loads(render(provider["header_template"], vars, {}))
    body = render(provider["embedding_request_body_template"], {"model": model}, {"input_json": "The quick brown fox"})
    vector = navigate(call(url, headers, body), provider["embedding_response_vector_path"])
    ok = isinstance(vector, list) and all(isinstance(v, (int, float)) for v in vector) and vector
    return check(f"{provider['name']} embeddings", bool(ok), f"dim={len(vector) if ok else 0}")


def generation(provider, key, model):
    vars = {"api_key": key, "model": model}
    url = PLACEHOLDER.sub(lambda m: vars.get(m.group(1), m.group(0)), provider["generation_endpoint_url"])
    headers = json.loads(render(provider["header_template"], vars, {}))
    body = render(provider["generation_request_body_template"], {"model": model}, {"prompt_json": "a lighthouse at dusk"})
    value = navigate(call(url, headers, body), provider["generation_response_media_path"])
    kind = "url" if isinstance(value, str) and value.startswith("http") else "base64"
    ok = isinstance(value, str) and bool(value)
    if ok and kind == "base64":
        ok = len(base64.b64decode(value)) > 0
    return check(f"{provider['name']} generation", ok, f"type={kind} len={len(value) if ok else 0}")


def main():
    key = SEED["api_key"]
    results = []
    for provider in SEED["providers"]:
        kinds = {m["kind"]: m["model"] for m in provider["models"]}
        if "chat" in kinds:
            results.append(chat(provider, key, kinds["chat"]))
        if provider.get("embedding_endpoint_url") and "embedding" in kinds:
            results.append(embeddings(provider, key, kinds["embedding"]))
        gen = kinds.get("image_generation") or kinds.get("video_generation")
        if provider.get("generation_endpoint_url") and gen:
            results.append(generation(provider, key, gen))
    last = json.loads(urllib.request.urlopen(f"{SEED['base_url']}/_debug/last", timeout=10).read())
    results.append(check("no api key leaked into any body", not last.get("key_leaked_in_body")))
    results.append(check("no unrendered placeholders", not last.get("placeholders_left")))
    print(f"\n{sum(results)}/{len(results)} passed")
    return 0 if all(results) else 1


if __name__ == "__main__":
    sys.exit(main())
