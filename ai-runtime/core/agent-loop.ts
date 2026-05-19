import { buildContext } from "./context-builder.js";
import { buildPrompt } from "./prompt-builder.js";
import { generate } from "../providers/ollama.js";

export async function runAgent(input: string) {
  console.log("→ Building context...");

  const context = await buildContext(input);

  console.log("→ Building prompt...");

  const prompt = buildPrompt(context, input);

  console.log("→ Calling Ollama...");

  const response = await generate(prompt);

  console.log("→ Done");

  return {
    context,
    prompt,
    response
  };
}