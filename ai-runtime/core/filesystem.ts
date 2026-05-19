import fg from "fast-glob";
import fs from "fs";

export async function getProjectFiles() {
  return fg(["**/*.ts", "!node_modules"], { dot: false });
}

export function readFile(file: string) {
  return fs.readFileSync(file, "utf-8");
}