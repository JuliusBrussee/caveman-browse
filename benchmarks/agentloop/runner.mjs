#!/usr/bin/env node
// Tier 3 head-to-head: identical agent, identical task, one browser MCP server
// swapped per arm. Serves a testdata fixture on localhost, runs headless
// `claude -p` with a strict single-server MCP config, and records
// provider-reported usage (observed-local), turns, wall time, and an
// exact-answer gate. No token figure here is ever 'verified'.
//
// Usage:
//   CAVEMAN_BROWSE_CHROME=<chrome> node runner.mjs [--arms browse,playwright,cdm] [--tasks id,id] [--model sonnet]
import { spawn } from "node:child_process";
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, "..", "..");
const chrome = process.env.CAVEMAN_BROWSE_CHROME;
if (!chrome) {
  console.error("CAVEMAN_BROWSE_CHROME must point at Chrome");
  process.exit(2);
}

const argv = process.argv.slice(2);
const flag = (name, fallback) => {
  const index = argv.indexOf(`--${name}`);
  return index === -1 ? fallback : argv[index + 1];
};
const model = flag("model", "sonnet");
const armFilter = flag("arms", "browse,playwright,cdm").split(",");
const taskFilter = flag("tasks", "").split(",").filter(Boolean);

const ARMS = {
  browse: () => ({
    command: join(repo, "bin", "caveman-browse"),
    args: [],
    env: { CAVEMAN_BROWSE_CHROME: chrome, CAVEMAN_HOME: mkdtempSync(join(tmpdir(), "cb-home-")) },
  }),
  playwright: () => ({
    command: "npx",
    args: ["-y", "@playwright/mcp@0.0.79", "--browser", "chrome", "--headless", "--isolated"],
  }),
  cdm: () => ({
    command: "npx",
    args: ["-y", "chrome-devtools-mcp@1.7.0", "--headless", "--isolated", `--executable-path=${chrome}`],
  }),
};

const tasks = JSON.parse(readFileSync(join(here, "tasks.json"), "utf8"))
  .filter((task) => taskFilter.length === 0 || taskFilter.includes(task.id));

const serve = (fixture) => {
  const html = readFileSync(join(repo, "testdata", fixture), "utf8");
  const server = createServer((_req, res) => {
    res.setHeader("content-type", "text/html; charset=utf-8");
    res.end(html);
  });
  return new Promise((ok) =>
    server.listen(0, "127.0.0.1", () => ok({ server, url: `http://127.0.0.1:${server.address().port}/` })),
  );
};

const results = [];
for (const task of tasks) {
  for (const armName of armFilter) {
    const arm = ARMS[armName]();
    const { server, url } = await serve(task.fixture);
    const configPath = join(mkdtempSync(join(tmpdir(), "cb-arm-")), "mcp.json");
    writeFileSync(configPath, JSON.stringify({ mcpServers: { browser: arm } }));
    const prompt = task.prompt.replace("{URL}", url);
    console.error(`>> ${task.id} / ${armName}`);
    const started = Date.now();
    // Async spawn so the in-process fixture server keeps serving while the
    // agent runs (spawnSync starves the event loop and the page never loads).
    const run = await new Promise((done) => {
      const child = spawn(
        "claude",
        [
          "-p", prompt,
          "--model", model,
          "--mcp-config", configPath,
          "--strict-mcp-config",
          "--dangerously-skip-permissions",
          "--output-format", "json",
        ],
        { env: { ...process.env } },
      );
      let stdout = "";
      let stderr = "";
      child.stdout.on("data", (chunk) => (stdout += chunk));
      child.stderr.on("data", (chunk) => (stderr += chunk));
      const killer = setTimeout(() => child.kill("SIGKILL"), 420_000);
      child.on("close", () => {
        clearTimeout(killer);
        done({ stdout, stderr });
      });
    });
    const wallSeconds = (Date.now() - started) / 1000;
    server.close();
    let record = { task: task.id, arm: armName, model, wallSeconds: Number(wallSeconds.toFixed(1)) };
    try {
      const payload = JSON.parse(run.stdout);
      const passed = task.expect.every((needle) => (payload.result ?? "").includes(needle));
      record = {
        ...record,
        passed,
        subtype: payload.subtype,
        turns: payload.num_turns,
        usage: payload.usage,
        costUSD: payload.total_cost_usd,
        answer: (payload.result ?? "").split("\n").filter((line) => line.includes("ANSWER:")).pop() ?? null,
        resultTail: (payload.result ?? "").slice(-400),
      };
    } catch {
      record = { ...record, passed: false, error: (run.stderr || run.stdout || "no output").slice(0, 400) };
    }
    results.push(record);
    console.error(JSON.stringify(record));
  }
}

writeFileSync(join(here, "results.json"), JSON.stringify(results, null, 2) + "\n");
console.error(`wrote ${results.length} records to benchmarks/agentloop/results.json`);
