const http = require('http');

const PORT = 8080;

const server = http.createServer((req, res) => {
  res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
  res.end(`
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Doki Container Engine</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      font-family: system-ui, -apple-system, sans-serif;
      background: #0a0a0a;
      color: #fff;
      display: flex; justify-content: center; align-items: center;
      min-height: 100vh;
    }
    .card {
      background: #1a1a2e;
      border: 1px solid #16213e;
      border-radius: 16px;
      padding: 60px 80px;
      text-align: center;
      box-shadow: 0 0 60px rgba(0, 180, 255, 0.15);
    }
    h1 {
      font-size: 3rem;
      background: linear-gradient(135deg, #00b4ff, #7b2fbe);
      -webkit-background-clip: text;
      -webkit-text-fill-color: transparent;
      margin-bottom: 12px;
    }
    .subtitle {
      font-size: 1.2rem;
      color: #888;
      margin-bottom: 32px;
    }
    .badge {
      display: inline-block;
      background: #16213e;
      padding: 8px 20px;
      border-radius: 20px;
      font-size: 0.85rem;
      color: #00b4ff;
      margin: 4px;
    }
    .footer {
      margin-top: 40px;
      font-size: 0.8rem;
      color: #555;
    }
  </style>
</head>
<body>
  <div class="card">
    <h1>Hello World by Doki</h1>
    <p class="subtitle">Container Engine for Android</p>
    <div>
      <span class="badge">Rootless</span>
      <span class="badge">Docker Compatible</span>
      <span class="badge">OCI Runtime</span>
      <span class="badge">Termux</span>
      <span class="badge">ARM64</span>
    </div>
    <p class="footer">
      Powered by <strong>Doki v0.1.0</strong> &mdash;
      Running on Node.js ${process.version}
    </p>
  </div>
</body>
</html>
  `);
});

server.listen(PORT, '0.0.0.0', () => {
  console.log(`🚀 Doki Node.js server running on http://0.0.0.0:${PORT}`);
  console.log(`   Hello World by Doki`);
});
