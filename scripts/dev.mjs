#!/usr/bin/env node
/**
 * Local dev orchestrator: starts the Go API and all four Next.js apps.
 * Usage: pnpm dev            (everything)
 *        pnpm dev --no-api   (frontends only)
 */
import { spawn } from "node:child_process";
import { existsSync } from "node:fs";

const args = new Set(process.argv.slice(2));
const runApi = !args.has("--no-api");

const procs = [];

function run(name, cmd, cmdArgs, opts = {}) {
  const color = opts.color ?? "36";
  const child = spawn(cmd, cmdArgs, {
    stdio: ["ignore", "pipe", "pipe"],
    env: process.env,
    shell: false,
  });
  const tag = `\x1b[${color}m[${name}]\x1b[0m`;
  const forward = (buf) => {
    for (const line of buf.toString().split("\n")) {
      if (line.trim()) console.log(`${tag} ${line}`);
    }
  };
  child.stdout.on("data", forward);
  child.stderr.on("data", forward);
  child.on("exit", (code) => {
    if (code !== null && code !== 0) console.log(`${tag} exited with code ${code}`);
  });
  procs.push(child);
}

if (runApi && !existsSync("services/api/go.mod")) {
  console.error("services/api not found — run from the repo root.");
  process.exit(1);
}

if (runApi) {
  // Ensure infra is up, then run the API with live reload via `go run`.
  run("setup", "docker", ["compose", "up", "-d", "--quiet-pull"], { color: "90" });
  run("api", "bash", ["scripts/api-dev.sh"], { color: "35" });
}

const apps = [
  ["customer", "apps/customer-web", "32"],
  ["restaurant", "apps/restaurant-web", "34"],
  ["courier", "apps/courier-web", "33"],
  ["admin", "apps/admin-web", "31"],
];

for (const [name, cwd, color] of apps) {
  run(name, "pnpm", ["--filter", `./${cwd}`, "dev"], { color });
}

const shutdown = () => {
  for (const p of procs) {
    try {
      p.kill("SIGTERM");
    } catch {
      /* already gone */
    }
  }
  process.exit(0);
};
process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);

// Keep the orchestrator alive while children run.
setInterval(() => {}, 1 << 30);
