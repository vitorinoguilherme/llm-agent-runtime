export function buildPrompt(context: any[], userInput: string) {
  const ctx = context
    .map(
      (c) => `FILE: ${c.path}\n${c.content.substring(0, 2000)}`
    )
    .join("\n\n");

  return `
You are a senior software engineer.

CONTEXT:
${ctx}

USER:
${userInput}

ANSWER:
`;
}