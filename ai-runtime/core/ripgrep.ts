import { execa } from "execa";

export async function search(query: string): Promise<string[]> {
  try {
    console.log("→ Running ripgrep with query:", query);

    const terms = query
      .split(" ")
      .map((t) => t.trim())
      .filter(Boolean);

    const results = new Set<string>();

    for (const term of terms) {
      const { stdout } = await execa(
        "rg",
        [
          "-e", term,
          "--files-with-matches",
          "--no-messages",
          "--hidden",
          "--glob", "!node_modules",
          "--glob", "!.git",
          "--max-count", "1",
          "."
        ],
        {
          timeout: 5000,
          reject: false
        }
      );

      stdout
        .split("\n")
        .filter(Boolean)
        .forEach((f) => results.add(f));
    }

    return Array.from(results);
  } catch (err) {
    console.error("ripgrep error:", err);
    return [];
  }
}