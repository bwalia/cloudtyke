#!/usr/bin/env python3
"""Sync Cloudflare DNS for cloudtyke.dev with records.json (desired state).

Creates missing records and updates changed ones. With --prune it also deletes
records of the managed names that are not in the file; otherwise it never
deletes. The token comes from CLOUDFLARE_API_TOKEN or ~/.config/cloudflare/token
and is never printed.

    python3 deploy/dns/sync.py            # dry run: show the plan
    python3 deploy/dns/sync.py --apply
"""
import json, os, sys, urllib.request, urllib.error, pathlib

API = "https://api.cloudflare.com/client/v4"
HERE = pathlib.Path(__file__).parent


def token():
    t = os.environ.get("CLOUDFLARE_API_TOKEN")
    if not t:
        p = pathlib.Path("~/.config/cloudflare/token").expanduser()
        t = p.read_text().strip() if p.exists() else ""
    if not t:
        sys.exit("no Cloudflare token: set CLOUDFLARE_API_TOKEN or create ~/.config/cloudflare/token")
    return t


def call(method, path, body=None):
    req = urllib.request.Request(API + path, method=method, data=json.dumps(body).encode() if body else None,
                                 headers={"Authorization": f"Bearer {TOKEN}", "Content-Type": "application/json",
                                          "User-Agent": "cloudtyke-dns-sync"})
    try:
        with urllib.request.urlopen(req) as r:
            return json.load(r)
    except urllib.error.HTTPError as e:
        sys.exit(f"{method} {path}: {e.code} {e.read().decode()[:400]}")


def main():
    apply, prune = "--apply" in sys.argv, "--prune" in sys.argv
    spec = json.loads((HERE / "records.json").read_text())
    zone = spec["zone"]
    zid = call("GET", f"/zones?name={zone}")["result"][0]["id"]
    have, page = [], 1
    while True:
        r = call("GET", f"/zones/{zid}/dns_records?per_page=500&page={page}")
        have += r["result"]
        if page >= r["result_info"]["total_pages"]:
            break
        page += 1
    fq = lambda n: zone if n == "@" else f"{n}.{zone}"
    want = [dict(r, name=fq(r["name"])) for r in spec["records"]]
    key = lambda r: (r["name"], r["type"], r["content"].rstrip(".").lower())
    by_key = {key(r): r for r in have}
    names_types = {(r["name"], r["type"]) for r in want}
    changes = 0
    for w in want:
        body = {"type": w["type"], "name": w["name"], "content": w["content"], "ttl": w.get("ttl", 1),
                "proxied": w.get("proxied", False), "comment": w.get("comment", "managed by cloudtyke deploy/dns")}
        cur = by_key.get(key(w))
        if cur is None:
            # a CNAME is unique per name: update it in place instead of adding a second one
            same = [h for h in have if h["name"] == w["name"] and h["type"] == w["type"] == "CNAME"]
            if same:
                print(f"~ {w['type']:5} {w['name']} -> {w['content']} (was {same[0]['content']})")
                apply and call("PUT", f"/zones/{zid}/dns_records/{same[0]['id']}", body)
            else:
                print(f"+ {w['type']:5} {w['name']} -> {w['content']}")
                apply and call("POST", f"/zones/{zid}/dns_records", body)
            changes += 1
        elif cur["proxied"] != body["proxied"] or (body["ttl"] != 1 and cur["ttl"] != body["ttl"]):
            print(f"~ {w['type']:5} {w['name']} proxied={body['proxied']} ttl={body['ttl']}")
            apply and call("PUT", f"/zones/{zid}/dns_records/{cur['id']}", body)
            changes += 1
    wanted = {key(w) for w in want}
    for h in have:
        if (h["name"], h["type"]) in names_types and key(h) not in wanted and not (h["type"] == "CNAME"):
            print(f"{'-' if prune else '?'} {h['type']:5} {h['name']} -> {h['content']} (not in records.json)")
            if prune and apply:
                call("DELETE", f"/zones/{zid}/dns_records/{h['id']}")
            changes += 1
    print(f"{changes} change(s){'' if apply else ' (dry run; use --apply)'}")


TOKEN = token()
if __name__ == "__main__":
    main()
