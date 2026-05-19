import { runAgent } from "../core/agent-loop.js";

const input = process.argv.slice(2).join(" ");

if (!input) {
  console.log("Usage: pnpm dev \"your query\"");
  process.exit(1);
}

const result = await runAgent(input);

console.log("\n=== CONTEXT ===");
console.log(result.context);

console.log("\n=== PROMPT ===");
console.log(result.prompt);

console.log("\n=== RESPONSE ===");
console.log(result.response);