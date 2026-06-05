#!/usr/bin/env bash
# scripts/sync-wiki.sh
# Sync the .wiki/ directory in the main repo to the three wiki targets:
#   - GitHub: OpceanAI/doki-wiki (separate repo, name 'doki-wiki' because
#     GitHub disallows repo names ending in '.wiki')
#   - GitLab: aguitauwu/doki 'wiki' branch (in the main repo)
#   - Codeberg: aguitauwu/Doki.wiki (separate repo, name preserved)
#
# Usage:
#   ./scripts/sync-wiki.sh           # sync all 3
#   ./scripts/sync-wiki.sh github    # only GitHub
#   ./scripts/sync-wiki.sh gitlab    # only GitLab
#   ./scripts/sync-wiki.sh codeberg  # only Codeberg
#
# Requirements:
#   - gh CLI authenticated (for GitHub), or GH_TOKEN env var
#   - GITLAB_TOKEN env var (for GitLab HTTPS)
#   - CODEBERG_TOKEN env var, or ~/.git-credentials, or SSH key (for Codeberg)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
WIKI_DIR="$REPO_ROOT/.wiki"

if [ ! -d "$WIKI_DIR" ]; then
    echo "ERROR: $WIKI_DIR not found" >&2
    exit 1
fi

TARGETS="${1:-all}"

sync_github() {
    echo "==> Syncing to GitHub Wiki (OpceanAI/doki-wiki)"

    local tmpdir
    tmpdir=$(mktemp -d)
    trap "rm -rf '$tmpdir'" EXIT

    # Clone the wiki repo (it may not exist yet)
    if gh repo view "OpceanAI/doki-wiki" >/dev/null 2>&1; then
        echo "    Wiki repo exists, cloning"
        git clone "https://github.com/OpceanAI/doki-wiki.git" "$tmpdir/wiki"
    else
        echo "    Wiki repo does not exist, creating"
        gh repo create "OpceanAI/doki-wiki" --public --source="$WIKI_DIR" --push --description "Doki documentation wiki (synced from .wiki/ in OpceanAI/Doki)"
        return 0
    fi

    cd "$tmpdir/wiki"
    # Remove all existing files except .git
    find . -mindepth 1 -not -path './.git*' -delete
    # Copy the new wiki content
    cp -r "$WIKI_DIR"/* .
    git add -A

    if git diff --cached --quiet; then
        echo "    No changes"
    else
        git commit -m "wiki: sync from main ($(date -u +%Y-%m-%dT%H:%M:%SZ))"
        git push origin main
        echo "    Pushed"
    fi
}

sync_gitlab() {
    echo "==> Syncing to GitLab Wiki (aguitauwu/doki 'wiki' branch)"

    local tmpdir
    tmpdir=$(mktemp -d)
    trap "rm -rf '$tmpdir'" EXIT

    # If GITLAB_TOKEN is set, inject it into the URL
    local remote_url="https://gitlab.com/aguitauwu/doki.git"
    if [ -n "${GITLAB_TOKEN:-}" ]; then
        remote_url="https://oauth2:${GITLAB_TOKEN}@gitlab.com/aguitauwu/doki.git"
    fi

    git clone "$remote_url" "$tmpdir/repo"
    cd "$tmpdir/repo"

    # Create or checkout the wiki branch
    if git ls-remote --heads origin wiki | grep -q wiki; then
        git checkout wiki
    else
        git checkout --orphan wiki
        git rm -rf . 2>/dev/null || true
    fi

    # Remove all existing files except .git
    find . -mindepth 1 -not -path './.git*' -delete
    # Copy the new wiki content
    cp -r "$WIKI_DIR"/* .
    git add -A

    if git diff --cached --quiet; then
        echo "    No changes"
    else
        git commit -m "wiki: sync from main ($(date -u +%Y-%m-%dT%H:%M:%SZ))"
        git push origin wiki --force
        echo "    Pushed"
    fi
}

sync_codeberg() {
    echo "==> Syncing to Codeberg Wiki (aguitauwu/Doki.wiki)"

    local tmpdir
    tmpdir=$(mktemp -d)
    trap "rm -rf '$tmpdir'" EXIT

    # If CODEBERG_TOKEN is set, inject it into the URL
    local remote_url="https://codeberg.org/aguitauwu/Doki.wiki.git"
    if [ -n "${CODEBERG_TOKEN:-}" ]; then
        remote_url="https://oauth2:${CODEBERG_TOKEN}@codeberg.org/aguitauwu/Doki.wiki.git"
    fi

    if git ls-remote "$remote_url" >/dev/null 2>&1; then
        git clone "$remote_url" "$tmpdir/wiki"
    else
        echo "    Wiki repo does not exist on Codeberg. Create it manually:"
        echo "      1. Go to https://codeberg.org/aguitauwu/Doki.wiki"
        echo "      2. Click '+' -> 'New Repository' -> name 'Doki.wiki'"
        echo "      3. Re-run this script"
        return 1
    fi

    cd "$tmpdir/wiki"
    find . -mindepth 1 -not -path './.git*' -delete
    cp -r "$WIKI_DIR"/* .
    git add -A

    if git diff --cached --quiet; then
        echo "    No changes"
    else
        git commit -m "wiki: sync from main ($(date -u +%Y-%m-%dT%H:%M:%SZ))"
        git push origin main --force
        echo "    Pushed"
    fi
}

case "$TARGETS" in
    all)
        sync_github || echo "GitHub sync failed (continuing)"
        sync_gitlab || echo "GitLab sync failed (continuing)"
        sync_codeberg || echo "Codeberg sync failed (continuing)"
        ;;
    github) sync_github ;;
    gitlab) sync_gitlab ;;
    codeberg) sync_codeberg ;;
    *)
        echo "Usage: $0 [all|github|gitlab|codeberg]"
        exit 1
        ;;
esac

echo ""
echo "Done."
