import { readFile } from "./filesystem.js";
import { search } from "./ripgrep.js";
import { config } from "../config/index.js";

export async function buildContext(query: string) {
  console.log("→ Searching with ripgrep...");

  const matches = await search(query);

  if (matches.length === 0) {
    console.log("⚠ No files found for query");
    return [];
  }

  console.log("→ Files found:", matches);

  const files = matches.slice(0, config.maxFiles);

  const result = [];

  for (const file of files) {
    console.log("→ Reading file:", file);

    const content = readFile(file);

    result.push({
      path: file,
      content
    });
  }

  return result;
}