#!/usr/bin/env node
// chrome-devtools-mcp arm for the snapshot head-to-head: serves a testdata
// fixture over localhost, drives chrome-devtools-mcp (navigate_page +
// take_snapshot), and prints the exact tool-result text an agent would
// receive. Count it with benchmarks/counttext.
//
// Usage: CAVEMAN_BROWSE_CHROME=<chrome> node cdm-snapshot.mjs [fixture.html]
import { spawn } from "node:child_process";
import { readFile } from "node:fs/promises";
import { createServer } from "node:http";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const fixture = process.argv[2] ?? "order_dashboard.html";
if (!/^[a-z0-9_-]+\.html$/.test(fixture)) {
  console.error("fixture must be a testdata HTML basename");
  process.exit(2);
}
const html = await readFile(join(here, "..", "..", "testdata", fixture), "utf8");
const server = createServer((_req, res) => {
  res.setHeader("content-type", "text/html; charset=utf-8");
  res.end(html);
});
await new Promise((ok) => server.listen(0, "127.0.0.1", ok));
const url = `http://127.0.0.1:${server.address().port}/`;

const args = ["-y", "chrome-devtools-mcp@latest", "--headless", "--isolated"];
if (process.env.CAVEMAN_BROWSE_CHROME) {
  args.push(`--executable-path=${process.env.CAVEMAN_BROWSE_CHROME}`);
}
const child = spawn("npx", args, { stdio: ["pipe", "pipe", "inherit"] });
const timeout = setTimeout(() => {
  console.error("cdm-snapshot: timed out after 180s");
  cleanup(1);
}, 180_000);

function cleanup(code) {
  clearTimeout(timeout);
  child.kill("SIGKILL");
  server.close();
  process.exit(code);
}

let buffer = "";
let id = 0;
const pending = new Map();
const send = (method, params) => {
  const message = { jsonrpc: "2.0", id: ++id, method, params };
  child.stdin.write(JSON.stringify(message) + "\n");
  return new Promise((ok, bad) => pending.set(message.id, { ok, bad }));
};
const callTool = (name, args) => send("tools/call", { name, arguments: args });

child.stdout.on("data", (chunk) => {
  buffer += chunk.toString();
  let newline;
  while ((newline = buffer.indexOf("\n")) !== -1) {
    const line = buffer.slice(0, newline).trim();
    buffer = buffer.slice(newline + 1);
    if (!line) continue;
    let message;
    try {
      message = JSON.parse(line);
    } catch {
      continue;
    }
    const waiter = pending.get(message.id);
    if (!waiter) continue;
    pending.delete(message.id);
    if (message.error) waiter.bad(new Error(JSON.stringify(message.error)));
    else waiter.ok(message.result);
  }
});

try {
  await send("initialize", {
    protocolVersion: "2025-06-18",
    capabilities: {},
    clientInfo: { name: "caveman-browse-bench", version: "1.0.0" },
  });
  child.stdin.write(JSON.stringify({ jsonrpc: "2.0", method: "notifications/initialized" }) + "\n");
  await callTool("navigate_page", { url });
  if (fixture === "order_dashboard.html") {
    // Same settle condition the other arms use: all 200 rows rendered.
    await callTool("wait_for", { text: "ORD-0200" });
  }
  const snapshot = await callTool("take_snapshot", {});
  const text = (snapshot.content ?? [])
    .filter((part) => part.type === "text")
    .map((part) => part.text)
    .join("\n");
  process.stdout.write(text);
  cleanup(0);
} catch (error) {
  console.error(`cdm-snapshot: ${error.message}`);
  cleanup(1);
}
