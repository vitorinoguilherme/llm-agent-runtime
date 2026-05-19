import { spawn } from "child_process";

export class LSPClient {
  private proc;
  private id = 0;

  constructor() {
    this.proc = spawn("node", [
      "./node_modules/typescript/lib/tsserver.js"
    ], {
      stdio: ["pipe", "pipe", "ignore"]
    });

    this.proc.stdout.on("data", (data) => {
      // SILENCIADO para não travar output
    });
  }

  dispose() {
    this.proc.kill();
  }

  send(method: string, params: any) {
    const msg = JSON.stringify({
      seq: this.id++,
      type: "request",
      command: method,
      arguments: params
    });

    this.proc.stdin.write(msg + "\n");
  }

  openFile(file: string, content: string) {
    this.send("open", { file, fileContent: content });
  }
}