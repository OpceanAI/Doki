#!/bin/bash
# Script to push Doki to GitLab
# Usage: ./push-to-gitlab.sh <GITLAB_TOKEN>

set -e

if [ -z "$1" ]; then
    echo "Usage: $0 <GITLAB_PERSONAL_ACCESS_TOKEN>"
    echo ""
    echo "Create a token at: https://gitlab.com/-/user_settings/personal_access_tokens"
    echo "Required scopes: api, read_repository, write_repository"
    exit 1
fi

GITLAB_TOKEN="$1"
REPO_URL="https://oauth2:${GITLAB_TOKEN}@gitlab.com/aguitauwu/doki.git"
WIKI_URL="https://oauth2:${GITLAB_TOKEN}@gitlab.com/aguitauwu/doki.wiki.git"

echo "=== Pushing main repository to GitLab ==="
cd /data/data/com.termux/files/home/doki
git remote set-url gitlab "$REPO_URL" 2>/dev/null || git remote add gitlab "$REPO_URL"
git push gitlab main --force
echo "Main repository pushed successfully."

echo ""
echo "=== Pushing wiki to GitLab ==="
cd /data/data/com.termux/files/usr/tmp/doki-gitlab-wiki
git init 2>/dev/null || true
git add .
git commit -m "Doki wiki v0.8.0" 2>/dev/null || echo "No changes to commit"
git remote set-url origin "$WIKI_URL" 2>/dev/null || git remote add origin "$WIKI_URL"
git push origin master --force
echo "Wiki pushed successfully."

echo ""
echo "=== Done ==="
echo "Main repo: https://gitlab.com/aguitauwu/doki"
echo "Wiki: https://gitlab.com/aguitauwu/doki/-/wikis"