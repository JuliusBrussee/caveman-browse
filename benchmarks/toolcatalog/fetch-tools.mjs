#!/usr/bin/env node
// Fetches an MCP server's tool catalog over stdio (newline-delimited JSON-RPC)
// and prints a JSON object: { server: <serverInfo>, tools: [{name, description,
// inputSchema}] }. The tools array uses the exact field set every MCP host
// receives, so token counts are comparable across servers.
//
// Usage: node fetch-tools.mjs <command> [args...]
import { spawn } from "node:child_process";

const [cmd, ...args] = process.argv.slice(2);
if (!cmd) {
  console.error("usage: fetch-tools.mjs <command> [args...]");
  process.exit(2);
}

const child = spawn(cmd, args, { stdio: ["pipe", "pipe", "inherit"] });
const timeout = setTimeout(() => {
  console.error("fetch-tools: timed out after 120s");
  child.kill("SIGKILL");
  process.exit(1);
}, 120_000);

let buffer = "";
let serverInfo = null;
const send = (message) => child.stdin.write(JSON.stringify(message) + "\n");

send({
  jsonrpc: "2.0",
  id: 1,
  method: "initialize",
  params: {
    protocolVersion: "2025-06-18",
    capabilities: {},
    clientInfo: { name: "caveman-browse-bench", version: "1.0.0" },
  },
});

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
      continue; // non-protocol noise on stdout
    }
    if (message.id === 1 && message.result) {
      serverInfo = message.result.serverInfo ?? null;
      send({ jsonrpc: "2.0", method: "notifications/initialized" });
      send({ jsonrpc: "2.0", id: 2, method: "tools/list", params: {} });
    } else if (message.id === 2) {
      clearTimeout(timeout);
      if (message.error) {
        console.error(`tools/list error: ${JSON.stringify(message.error)}`);
        child.kill("SIGKILL");
        process.exit(1);
      }
      const tools = (message.result.tools ?? []).map((tool) => ({
        name: tool.name,
        description: tool.description ?? "",
        inputSchema: tool.inputSchema ?? {},
      }));
      process.stdout.write(JSON.stringify({ server: serverInfo, tools }) + "\n");
      child.kill("SIGKILL");
      process.exit(0);
    }
  }
});

child.on("exit", (code) => {
  clearTimeout(timeout);
  if (code !== null && code !== 0) process.exit(1);
});
