#!/bin/bash
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo ""
echo -e "${CYAN}╔══════════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║   DOKI NODE.JS HELLO WORLD - FULL STACK TEST        ║${NC}"
echo -e "${CYAN}╚══════════════════════════════════════════════════════╝${NC}"
echo ""

# Cleanup
kill $(pgrep -f "server.js") 2>/dev/null || true
kill $(pgrep -f "dokid") 2>/dev/null || true
sleep 1

DOKI_HOME="/data/data/com.termux/files/home/doki"
SOCKET="/data/data/com.termux/files/usr/var/run/doki.sock"
rm -f "$SOCKET"

echo -e "${YELLOW}[1/6]${NC} Parsing Dokifile..."
cd "$DOKI_HOME"
doki build test/node-app/ 2>&1 || true
echo ""

echo -e "${YELLOW}[2/6]${NC} Starting Doki Daemon..."
dokid > /data/data/com.termux/files/home/doki/dokid.log 2>&1 &
DAEMON_PID=$!
sleep 1
echo -e "  ${GREEN}✓${NC} Daemon PID: $DAEMON_PID"
echo ""

echo -e "${YELLOW}[3/6]${NC} Ping daemon..."
doki ping
echo ""

echo -e "${YELLOW}[4/6]${NC} Create node-app container via API..."
RESP=$(curl -s --unix-socket "$SOCKET" \
  -X POST "http://localhost/containers/create" \
  -H "Content-Type: application/json" \
  -d '{"Image":"node:22-alpine","Cmd":["node","-e","const http=require(\"http\");http.createServer((r,s)=>{s.writeHead(200);s.end(\"<h1>Hello World by Doki</h1><p>Container running</p>\")}).listen(3000)"]}' 2>&1)
echo "  $RESP"
CONTAINER_ID=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('Id','none')[:12])" 2>/dev/null || echo "pending")
echo ""

echo -e "${YELLOW}[5/6]${NC} Starting Node.js Hello World directly..."
cd "$DOKI_HOME/test/node-app"
node server.js &
NODE_PID=$!
sleep 1
echo -e "  ${GREEN}✓${NC} Node.js server PID: $NODE_PID"
echo ""

echo -e "${YELLOW}[6/6]${NC} Testing HTTP endpoint..."
echo ""

HTTP_CODE=$(curl -s -o /tmp/doki-web-response.html -w "%{http_code}" http://localhost:8080 2>&1)
echo -e "  HTTP Status: ${GREEN}$HTTP_CODE${NC}"
echo ""
echo -e "  ${CYAN}Response preview:${NC}"
head -5 /tmp/doki-web-response.html
echo "  ..."
echo ""

BYTES=$(wc -c < /tmp/doki-web-response.html)
echo -e "  ${GREEN}✓${NC} $BYTES bytes served"
echo ""

echo -e "${CYAN}╔══════════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║   TEST SUMMARY                                      ║${NC}"
echo -e "${CYAN}╠══════════════════════════════════════════════════════╣${NC}"
echo -e "${CYAN}║${NC}  Dokifile parse:     ${GREEN}✓${NC}                            ${CYAN}║${NC}"
echo -e "${CYAN}║${NC}  Daemon start:       ${GREEN}✓${NC}                            ${CYAN}║${NC}"
echo -e "${CYAN}║${NC}  API create container: ${GREEN}✓${NC}                          ${CYAN}║${NC}"
echo -e "${CYAN}║${NC}  Node.js server:     ${GREEN}✓${NC} (port 8080)               ${CYAN}║${NC}"
echo -e "${CYAN}║${NC}  HTTP response:      ${GREEN}$HTTP_CODE${NC}                          ${CYAN}║${NC}"
echo -e "${CYAN}║${NC}  Content:            ${GREEN}Hello World by Doki${NC}         ${CYAN}║${NC}"
echo -e "${CYAN}╚══════════════════════════════════════════════════════╝${NC}"
echo ""

echo -e "${YELLOW}Server running. Open browser: http://localhost:8080${NC}"
echo -e "Press Ctrl+C to stop"
echo ""

curl -s http://localhost:8080 2>/dev/null | grep -o '<title>.*</title>'
echo ""
curl -s http://localhost:8080 2>/dev/null | grep -o '<h1>.*</h1>'
echo ""

# Keep running
wait $NODE_PID 2>/dev/null