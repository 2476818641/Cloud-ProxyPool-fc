# -*- coding: utf-8 -*-
import base64
import json
import os
import sys
import time
import zipfile

import requests
import toml
from alibabacloud_fc20230330 import models as fc_models
from alibabacloud_fc20230330.client import Client as FcClient
from alibabacloud_tea_openapi import models as open_api_models

CONFIG_FILE = "deploy.toml"


def load_config():
    if not os.path.exists(CONFIG_FILE):
        print(f"[error] missing config file: {CONFIG_FILE}")
        print(f"[hint] copy deploy.toml.example to {CONFIG_FILE} and fill in AccessKey values")
        sys.exit(1)

    try:
        with open(CONFIG_FILE, "r", encoding="utf-8") as handle:
            return toml.load(handle)
    except Exception as exc:
        print(f"[error] failed to parse {CONFIG_FILE}: {exc}")
        sys.exit(1)


def create_zip(source_dir, output_filename):
    print(f"[+] packaging server code: {output_filename}")
    with zipfile.ZipFile(output_filename, "w", zipfile.ZIP_DEFLATED) as zipf:
        for root, dirs, files in os.walk(source_dir):
            dirs[:] = [item for item in dirs if item != "__pycache__"]
            for filename in files:
                if filename.endswith((".pyc", ".pyo")):
                    continue
                file_path = os.path.join(root, filename)
                arcname = os.path.relpath(file_path, source_dir)
                zipf.write(file_path, arcname)


def get_client(region, access_key_id, access_key_secret):
    config = open_api_models.Config(
        access_key_id=access_key_id,
        access_key_secret=access_key_secret,
        region_id=region,
        endpoint=f"fcv3.{region}.aliyuncs.com",
        connect_timeout=10000,
        read_timeout=60000,
    )
    return FcClient(config)


def function_exists(client, function_name):
    try:
        client.get_function(function_name, fc_models.GetFunctionRequest())
        return True
    except Exception as exc:
        message = str(exc)
        if "FunctionNotFound" in message or "404" in message:
            return False
        raise


def build_create_function_input(conf, code_content):
    deployment = conf["deployment"]
    return fc_models.CreateFunctionInput(
        function_name=deployment["function_name"],
        description=deployment.get(
            "description",
            "Cloud ProxyPool over Alibaba Cloud Function Compute",
        ),
        handler=deployment.get("handler", "index.handler"),
        runtime=deployment.get("runtime", "python3.10"),
        timeout=deployment.get("timeout", 60),
        memory_size=deployment.get("memory_size", 512),
        internet_access=deployment.get("internet_access", True),
        code=fc_models.InputCodeLocation(
            zip_file=base64.b64encode(code_content).decode("utf-8")
        ),
    )


def build_update_function_input(conf, code_content):
    deployment = conf["deployment"]
    return fc_models.UpdateFunctionInput(
        description=deployment.get(
            "description",
            "Cloud ProxyPool over Alibaba Cloud Function Compute",
        ),
        handler=deployment.get("handler", "index.handler"),
        runtime=deployment.get("runtime", "python3.10"),
        timeout=deployment.get("timeout", 60),
        memory_size=deployment.get("memory_size", 512),
        internet_access=deployment.get("internet_access", True),
        code=fc_models.InputCodeLocation(
            zip_file=base64.b64encode(code_content).decode("utf-8")
        ),
    )


def deploy_function(client, region, code_content, conf):
    function_name = conf["deployment"]["function_name"]

    exists = function_exists(client, function_name)

    if exists:
        print(f"[*] [{region}] function exists, updating code and config")
        request = fc_models.UpdateFunctionRequest(
            body=build_update_function_input(conf, code_content)
        )
        client.update_function(function_name, request)
    else:
        print(f"[*] [{region}] function not found, creating")
        request = fc_models.CreateFunctionRequest(
            body=build_create_function_input(conf, code_content)
        )
        client.create_function(request)

    wait_for_function_ready(client, function_name, region, conf)
    return True


def wait_for_function_ready(client, function_name, region, conf):
    delay = conf["deployment"].get("post_update_delay", 2)
    retries = conf["deployment"].get("status_check_retries", 3)
    for attempt in range(1, retries + 1):
        try:
            response = client.get_function(function_name, fc_models.GetFunctionRequest())
            state = (response.body.state or "").lower()
            update_status = (response.body.last_update_status or "").lower()
            if not state and not update_status:
                return
            if state in ("", "active") and update_status in ("", "successful"):
                return
        except Exception:
            pass
        print(f"[*] [{region}] waiting for function to become active ({attempt}/{retries})")
        time.sleep(delay)
    print(f"[!] [{region}] function status is still pending, continuing to trigger setup")


def build_http_trigger_config(conf):
    methods = conf["deployment"].get(
        "methods",
        ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"],
    )
    http_config = fc_models.HTTPTriggerConfig(
        auth_type=conf["deployment"].get("auth_type", "anonymous"),
        methods=methods,
        disable_urlinternet=False,
    )
    return json.dumps(http_config.to_map(), ensure_ascii=False)


def ensure_http_trigger(client, region, conf):
    function_name = conf["deployment"]["function_name"]
    trigger_name = conf["deployment"].get("trigger_name", "http_trigger")
    trigger_config = build_http_trigger_config(conf)

    try:
        client.get_trigger(function_name, trigger_name)
        print(f"[*] [{region}] trigger exists, updating")
        request = fc_models.UpdateTriggerRequest(
            body=fc_models.UpdateTriggerInput(
                trigger_config=trigger_config,
                description="Public HTTP trigger for Cloud ProxyPool",
            )
        )
        response = client.update_trigger(function_name, trigger_name, request)
    except Exception as exc:
        message = str(exc)
        if "TriggerNotFound" not in message and "404" not in message:
            raise
        print(f"[*] [{region}] trigger not found, creating")
        request = fc_models.CreateTriggerRequest(
            body=fc_models.CreateTriggerInput(
                trigger_name=trigger_name,
                trigger_type="http",
                trigger_config=trigger_config,
                description="Public HTTP trigger for Cloud ProxyPool",
            )
        )
        response = client.create_trigger(function_name, request)

    url = ""
    if response and response.body and response.body.http_trigger:
        url = response.body.http_trigger.url_internet or ""

    if url:
        return url

    get_response = client.get_trigger(function_name, trigger_name)
    if get_response.body and get_response.body.http_trigger:
        return get_response.body.http_trigger.url_internet
    return None


def check_health(url, conf):
    test_url = conf["health_check"]["test_url"]
    expected_status = conf["health_check"]["expected_status"]

    print(f"[*] [health] testing proxy via {url} -> {test_url}")

    payload = {
        "method": "GET",
        "url": test_url,
        "headers": {"User-Agent": "Deploy-HealthCheck"},
        "body": "",
    }

    try:
        response = requests.post(url, json=payload, timeout=15)
        if response.status_code != 200:
            print(f"[-] [health] function invoke failed: {response.status_code}")
            print(f"    response: {response.text[:200]}")
            return False

        data = response.json()
        if data.get("status_code") != expected_status:
            print(f"[-] [health] unexpected upstream status: {data.get('status_code')}")
            if "error" in data:
                print(f"    error: {data['error']}")
            return False

        content = base64.b64decode(data.get("content", "")).decode(
            "utf-8", errors="ignore"
        )
        preview = content.strip()[:50].replace("\n", " ")
        print(f"[+] [health] passed, upstream={data['status_code']}, preview={preview}...")
        return True
    except Exception as exc:
        print(f"[-] [health] exception: {exc}")
        return False


def generate_client_config(urls):
    config_content = f"""[client]
listen_addr = "127.0.0.1:10800"
socks_addr = ":10801"
dashboard_addr = ":8081"
dump = false
debug = false

# Optional:
# user = "admin"
# password = "your_password"
# dump_file = "traffic.log"

[cloud]
# Generated by deploy.py
function_urls = {json.dumps(urls)}
region = "multi-region"
"""

    base_dir = os.path.dirname(os.path.abspath(__file__))
    client_dir = os.path.join(os.path.dirname(base_dir), "client")
    config_path = os.path.join(client_dir, "config.toml")

    with open(config_path, "w", encoding="utf-8") as handle:
        handle.write(config_content)
    print(f"\n[+] wrote client config: {config_path} ({len(urls)} available nodes)")


def main():
    print("=== Cloud ProxyPool deploy tool for Alibaba Cloud FC ===")
    conf = load_config()

    access_key_id = conf["aliyun"]["access_key_id"]
    access_key_secret = conf["aliyun"]["access_key_secret"]

    if "YOUR_ACCESS_KEY_ID" in access_key_id or "YOUR_ACCESS_KEY_SECRET" in access_key_secret:
        print("[hint] fill real AccessKey values in deploy.toml before running")
        sys.exit(1)

    base_dir = os.path.dirname(os.path.abspath(__file__))
    server_dir = os.path.join(os.path.dirname(base_dir), "server")
    zip_path = os.path.join(base_dir, "deploy_package.zip")
    create_zip(server_dir, zip_path)
    with open(zip_path, "rb") as handle:
        code_content = handle.read()

    success_urls = []
    regions = conf["deployment"]["regions"]
    print(f"\n[+] deploying to {len(regions)} regions: {regions}")

    for region in regions:
        print(f"\n>>> region: {region}")
        try:
            client = get_client(region, access_key_id, access_key_secret)
            if deploy_function(client, region, code_content, conf):
                url = ensure_http_trigger(client, region, conf)
                if not url:
                    print(f"[-] [{region}] failed to obtain HTTP trigger URL")
                    continue

                print(f"[+] [{region}] trigger URL: {url}")
                if conf["health_check"]["enable"]:
                    if check_health(url, conf):
                        success_urls.append(url)
                    else:
                        print(f"[!] [{region}] deployment ok but health check failed")
                else:
                    success_urls.append(url)
        except Exception as exc:
            print(f"[-] [{region}] deployment failed: {exc}")

    if os.path.exists(zip_path):
        os.remove(zip_path)

    if success_urls:
        generate_client_config(success_urls)
        print("\n=== deployment finished ===")
        print("Run the Go client with ./cloud-proxy.exe -C config.toml")
    else:
        print("\n[!] no healthy FC nodes were deployed")


if __name__ == "__main__":
    main()
