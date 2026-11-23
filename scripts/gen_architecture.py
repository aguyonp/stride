#!/usr/bin/env python3
# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""Generator for docs/architecture.svg — the single source of truth for the diagram.

The editable text lives in the TEXT dict below (one key per label, grouped by
box). Geometry, colors and fonts live in render(). The checked-in SVG must be
byte-identical to this script's output (guarantees the exact same rendering):
CI runs ``--check`` and fails on any drift.

To update the diagram: change the strings in TEXT, then regenerate:

    python3 scripts/gen_architecture.py            # rewrites docs/architecture.svg
    python3 scripts/gen_architecture.py --check    # exits 1 if the checked-in file is stale
"""

from __future__ import annotations

import sys
from pathlib import Path

FONT = "Inter, 'Segoe UI', Helvetica, Arial, sans-serif"

TEXT = {
    "aria": "Stride 4.4.0 architecture",
    "title": "Stride 4.4.0 — Architecture",
    "subtitle": "compose.yml: stride (Go API + in-process jobs + embedded SPA) · valkey (TTL cache + SSE Pub/Sub) · SQLite WAL (Goose) · S3 backups",
    "pwa_title": "React 19 + TS PWA",
    "pwa": [
        "Dashboard · Cycles Gantt (/plan)",
        "Body Map (/body)",
        "Command palette (Cmd+K)",
        "Typed client · passkeys",
    ],
    "pwa_footer": "Zero polling · realtime SSE",
    "portal_title": "Physio & Coach Portal",
    "portal": [
        "/share/:token (per-link permissions)",
        "Instructions · notes · prescriptions",
    ],
    "obs_title": "Observability",
    "obs": "GET /version · /metrics (dedicated port)",
    "go_title": "Go Server (HTTP + Worker)",
    "go": [
        "WebAuthn passkeys · recovery codes",
        "AES-256-GCM · SHA-256 · SPEC.md + types.ts",
        "SSE stream (LLM tokens) · per-user MCP",
        "Jobs: sync · llm · webhook · backup",
        "Macro / Meso / Micro cycles & planned/done",
        "Body Map & correlations (drop, wear, D-, GCT)",
        "Biomechanics · EF · Impact · Readiness",
        "AI coach · session morphing · .FIT export",
    ],
    "go_footer": "Prometheus-instrumented (P50/P95/P99)",
    "jobs_title": "Async jobs + backoff retries",
    "jobs": "Sync · FIT/GPX/TCX parsing · LLM · webhooks",
    "sqlite_title": "SQLite (WAL)",
    "sqlite": [
        "Embedded Goose migrations",
        "Cycles · Body Map · Share",
        "1 writer · read-only pool",
    ],
    "valkey_title": "Valkey 9",
    "valkey": [
        "Pools per job group",
        "TTL cache · SSE Pub/Sub",
        "(in-memory fallback in dev)",
    ],
    "providers": "Outbound APIs (meter · quota · egress guard)",
    "garmin_title": "Garmin Connect",
    "garmin": "Personal API · .FIT archives · health",
    "strava_title": "Strava API v3",
    "strava": "OAuth2 · streams · shoes · webhooks",
    "llm_title": "LLM Streaming",
    "llm": "Gemini · Claude · OpenAI · Ollama",
    "meteo_title": "Open-Meteo",
    "meteo": "Hourly weather & session archive",
    "webhooks_title": "Outbound webhooks",
    "webhooks": "Discord · Telegram · JSON (retry 429/5xx)",
    "edge_rest": "REST JSON",
    "edge_sse": "SSE stream",
    "edge_portal": "public portal",
    "edge_metrics": "/metrics",
    "edge_sql": "SQL WAL",
    "edge_pubsub": "Pub/Sub · Cache",
    "edge_snapshot": "snapshot",
    "edge_sigv4": "SigV4",
    "backup_title": "Atomic backups",
    "backup": [
        "VACUUM INTO · gzip · SHA-256 · daily + pre-migration",
        "data/backups (retention 7 + 3 pre-migration)",
    ],
    "s3_title": "S3-compatible storage",
    "s3": "AWS · MinIO · R2 · Scaleway · Garage · B2",
}


def esc(s: str) -> str:
    return s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def num(n: int | float) -> str:
    if isinstance(n, float) and n.is_integer():
        n = int(n)
    return str(n)


def text(x: float, y: float, s: str, size: float, weight: int | None = None,
         fill: str = "#2b2b2b", anchor: bool = True) -> str:
    a = ' text-anchor="middle"' if anchor else ""
    w = "" if weight is None else f' font-weight="{weight}"'  # None = attribute omitted, as in the SVG
    return (
        f'  <text x="{num(x)}" y="{num(y)}" font-family="{FONT}" font-size="{num(size)}"'
        f'{w} fill="{fill}"{a}>{esc(s)}</text>'
    )


def rect(x: float, y: float, w: float, h: float, fill: str, sw: float = 1.8,
         dash: str | None = None, stroke: str = "#1e1e1e", rx: float = 14) -> str:
    d = f' stroke-dasharray="{dash}"' if dash else ""
    return (
        f'  <rect x="{num(x)}" y="{num(y)}" width="{num(w)}" height="{num(h)}" rx="{num(rx)}" '
        f'fill="{fill}" stroke="{stroke}" stroke-width="{num(sw)}"{d}/>'
    )


def path(d: str, sw: float, stroke: str = "#1e1e1e", dash: str | None = None, start: bool = False) -> str:
    da = f' stroke-dasharray="{dash}"' if dash else ""
    ms = ' marker-start="url(#ahs)"' if start else ""
    return f'  <path d="{d}" fill="none" stroke="{stroke}" stroke-width="{num(sw)}"{da} marker-end="url(#ah)"{ms}/>'


def render() -> str:
    L: list[str] = []
    a = L.append
    a(f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1080 720" width="1080" height="720" role="img" aria-label="{esc(TEXT["aria"])}">')
    a("  <defs>")
    a('    <marker id="ah" viewBox="0 0 12 12" refX="10" refY="6" markerWidth="10" markerHeight="10" orient="auto">')
    a('      <path d="M1.5,1.5 L10,6 L1.5,10.5" fill="none" stroke="#1e1e1e" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/>')
    a("    </marker>")
    a('    <marker id="ahs" viewBox="0 0 12 12" refX="2" refY="6" markerWidth="10" markerHeight="10" orient="auto">')
    a('      <path d="M10.5,1.5 L2,6 L10.5,10.5" fill="none" stroke="#1e1e1e" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/>')
    a("    </marker>")
    a("  </defs>")
    a("")
    a('  <rect width="1080" height="720" rx="18" fill="#ffffff"/>')
    a("")
    a("  <!-- Header -->")
    a(text(40, 52, TEXT["title"], 22, 700, "#1e1e1e", anchor=False))
    a(text(40, 76, TEXT["subtitle"], 13, 400, "#495057", anchor=False))  # original keeps the explicit weight here
    a("")
    a("  <!-- React PWA -->")
    a(rect(40, 135, 225, 150, fill="#a5d8ff"))
    a(text(152.5, 165, TEXT["pwa_title"], 15.5, 700, fill="#1e1e1e"))
    for y, s in zip((188, 207, 226, 245), TEXT["pwa"]):
        a(text(152.5, y, s, 12, fill="#2b2b2b"))
    a(text(152.5, 266, TEXT["pwa_footer"], 11.5, 600, fill="#1864ab"))
    a("")
    a("  <!-- Public Physio / Coach Share View -->")
    a(rect(40, 315, 225, 96, "#e7f5ff", sw=1.6, dash="5,3"))
    a(text(152.5, 344, TEXT["portal_title"], 14.5, 700, fill="#1e1e1e"))
    a(text(152.5, 365, TEXT["portal"][0], 12, fill="#3b3b3b"))
    a(text(152.5, 384, TEXT["portal"][1], 11.5, fill="#495057"))
    a("")
    a("  <!-- Prometheus Scraper -->")
    a(rect(40, 440, 225, 74, "#f1f3f5", sw=1.5))
    a(text(152.5, 468, TEXT["obs_title"], 14.5, 700, fill="#1e1e1e"))
    a(text(152.5, 489, TEXT["obs"], 12, fill="#3b3b3b"))
    a("")
    a("  <!-- Go API + Worker Core -->")
    a(rect(365, 125, 300, 295, fill="#b2f2bb"))
    a(text(515, 156, TEXT["go_title"], 16, 700, fill="#1e1e1e"))
    for y, s in zip((180, 200, 220, 240, 260, 280, 300, 320), TEXT["go"]):
        a(text(515, y, s, 12, fill="#2b2b2b"))
    a(text(515, 342, TEXT["go_footer"], 11.5, 600, fill="#2b8a3e"))
    a(rect(387, 358, 256, 44, "#d3f9d8", sw=1.2, stroke="#2b8a3e", rx=9))
    a(text(515, 377, TEXT["jobs_title"], 12, 600, fill="#1e1e1e"))
    a(text(515, 394, TEXT["jobs"], 11, fill="#3b3b3b"))
    a("")
    a("  <!-- SQLite WAL -->")
    a(rect(335, 485, 175, 96, fill="#ffec99"))
    a(text(422.5, 514, TEXT["sqlite_title"], 15.5, 700, fill="#1e1e1e"))
    a(text(422.5, 535, TEXT["sqlite"][0], 12, fill="#3b3b3b"))
    a(text(422.5, 553, TEXT["sqlite"][1], 11.5, fill="#495057"))
    a(text(422.5, 570, TEXT["sqlite"][2], 11, fill="#495057"))
    a("")
    a("  <!-- Valkey -->")
    a(rect(530, 485, 185, 96, fill="#ffc9c9"))
    a(text(622.5, 514, TEXT["valkey_title"], 15.5, 700, fill="#1e1e1e"))
    a(text(622.5, 535, TEXT["valkey"][0], 12, fill="#3b3b3b"))
    a(text(622.5, 553, TEXT["valkey"][1], 11.5, fill="#495057"))
    a(text(622.5, 570, TEXT["valkey"][2], 11, fill="#495057"))
    a("")
    a("  <!-- External Providers -->")
    a(text(905, 108, TEXT["providers"], 12, 600, fill="#495057"))
    a("")
    a(rect(780, 122, 255, 74, fill="#d0bfff"))
    a(text(907.5, 152, TEXT["garmin_title"], 15.5, 700, fill="#1e1e1e"))
    a(text(907.5, 174, TEXT["garmin"], 12, fill="#3b3b3b"))
    a("")
    a(rect(780, 214, 255, 74, fill="#ffd8a8"))
    a(text(907.5, 244, TEXT["strava_title"], 15.5, 700, fill="#1e1e1e"))
    a(text(907.5, 266, TEXT["strava"], 12, fill="#3b3b3b"))
    a("")
    a(rect(780, 306, 255, 74, fill="#eebefa"))
    a(text(907.5, 336, TEXT["llm_title"], 15.5, 700, fill="#1e1e1e"))
    a(text(907.5, 358, TEXT["llm"], 12, fill="#3b3b3b"))
    a("")
    a(rect(780, 398, 255, 74, fill="#c5f6fa"))
    a(text(907.5, 428, TEXT["meteo_title"], 15.5, 700, fill="#1e1e1e"))
    a(text(907.5, 450, TEXT["meteo"], 12, fill="#3b3b3b"))
    a("")
    a(rect(780, 490, 255, 74, fill="#e9ecef"))
    a(text(907.5, 520, TEXT["webhooks_title"], 15.5, 700, fill="#1e1e1e"))
    a(text(907.5, 542, TEXT["webhooks"], 12, fill="#3b3b3b"))
    a("")
    a("  <!-- Connections: Left -> Go Core -->")
    a(path("M268,190 L361,190", 1.7, start=True))
    a(text(315, 181, TEXT["edge_rest"], 11, 600, fill="#495057"))
    a("")
    a(path("M361,226 L268,226", 1.8, stroke="#1864ab"))
    a(text(315, 242, TEXT["edge_sse"], 11, 600, fill="#1864ab"))
    a("")
    a(path("M268,360 L361,330", 1.6))
    a(text(315, 335, TEXT["edge_portal"], 11, fill="#495057"))
    a("")
    a(path("M268,476 L361,400", 1.5, dash="4,3"))
    a(text(306, 454, TEXT["edge_metrics"], 11, fill="#495057"))
    a("")
    a("  <!-- Connections: Go Core -> SQLite & Valkey -->")
    a(path("M460,422 L432,481", 1.7, start=True))
    a(text(418, 456, TEXT["edge_sql"], 11.5, 600, fill="#495057"))
    a("")
    a(path("M572,422 L605,481", 1.7, start=True))
    a(text(626, 456, TEXT["edge_pubsub"], 11.5, 600, fill="#495057"))
    a("")
    a("  <!-- Atomic backups -->")
    a(rect(335, 612, 380, 74, fill="#fff9db"))
    a(text(525, 640, TEXT["backup_title"], 15, 700, fill="#1e1e1e"))
    a(text(525, 661, TEXT["backup"][0], 12, fill="#3b3b3b"))
    a(text(525, 677, TEXT["backup"][1], 11, fill="#495057"))
    a("")
    a(rect(780, 602, 255, 74, fill="#e6fcf5"))
    a(text(907.5, 632, TEXT["s3_title"], 15.5, 700, fill="#1e1e1e"))
    a(text(907.5, 654, TEXT["s3"], 12, fill="#3b3b3b"))
    a("")
    a(path("M422.5,583 L422.5,608", 1.6))
    a(text(380, 600, TEXT["edge_snapshot"], 11, fill="#495057"))
    a("")
    a(path("M717,649 L776,641", 1.6, dash="4,3"))
    a(text(746, 634, TEXT["edge_sigv4"], 11, fill="#495057"))
    a("")
    a("  <!-- Connections: Go Worker -> External Providers -->")
    for d in (
        "M667,255 C715,255 730,159 776,159",
        "M667,270 C715,270 730,251 776,251",
        "M667,285 C715,285 730,343 776,343",
        "M667,300 C715,300 730,435 776,435",
        "M667,315 C715,315 730,527 776,527",
    ):
        a(path(d, 1.6))
    a("</svg>")
    return "\n".join(L) + "\n"


def main() -> int:
    out = Path(__file__).resolve().parent.parent / "docs" / "architecture.svg"
    svg = render()
    if "--check" in sys.argv[1:]:
        current = out.read_text(encoding="utf-8")
        if current != svg:
            import difflib

            sys.stderr.writelines(difflib.unified_diff(
                current.splitlines(keepends=True), svg.splitlines(keepends=True),
                fromfile=str(out), tofile="generated"))
            sys.stderr.write("\ndocs/architecture.svg is stale — run: python3 scripts/gen_architecture.py\n")
            return 1
        print("docs/architecture.svg is up to date.")
        return 0
    out.write_text(svg, encoding="utf-8")
    print(f"wrote {out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
