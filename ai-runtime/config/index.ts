import dotenv from "dotenv";

dotenv.config();

export const config = {
  ollamaUrl: process.env.OLLAMA_URL!,
  model: process.env.OLLAMA_MODEL!,
  maxFiles: Number(process.env.MAX_CONTEXT_FILES || 5)
};