#!/usr/bin/env bash
# Markdown changelog of the commits between the previous X.Y.Z tag and <ref> (default HEAD), grouped by
# conventional-commit type. Usage: scripts/changelog.sh [ref]
set -euo pipefail

to="${1:-HEAD}"
prev="$(git describe --tags --abbrev=0 --match '[0-9]*.[0-9]*.[0-9]*' "${to}^" 2>/dev/null || true)"
range="${prev:+${prev}..}${to}"

git log --no-merges --format='%s%x09%h' "$range" | awk -F '\t' '
  {
    subject = $1; hash = $2
    type = subject; sub(/[(:!].*/, "", type)
    group = "Other"
    if (type == "feat") group = "Features"
    else if (type == "fix") group = "Fixes"
    else if (type == "perf") group = "Performance"
    else if (type == "refactor") group = "Refactoring"
    else if (type == "docs") group = "Documentation"
    else if (type == "ci" || type == "build") group = "CI & build"
    else if (type == "chore" || type == "style" || type == "test") group = "Maintenance"
    lines[group] = lines[group] "- " subject " (" hash ")\n"
  }
  END {
    n = split("Features Fixes Performance Refactoring Documentation CI&build Maintenance Other", order, " ")
    for (i = 1; i <= n; i++) {
      g = order[i]; if (g == "CI&build") g = "CI & build"
      if (g in lines) printf "### %s\n%s\n", g, lines[g]
    }
  }'

if [[ -n "$prev" && -n "${GITHUB_REPOSITORY:-}" ]]; then
  echo "**Full diff:** ${GITHUB_SERVER_URL:-https://github.com}/${GITHUB_REPOSITORY}/compare/${prev}...${to}"
fi
