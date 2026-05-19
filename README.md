# llm-agent-runtime

Experimentos e análises técnicas de code agents usando modelos locais via Ollama.

Inclui:
- logs brutos de execução
- proxy para captura/interceptação
- runtime mínimo baseado em ripgrep
- comparações entre Aider, OpenCode, Crush e Claude Code

## Estrutura

- `/logs` → logs crus dos agents
- `/proxy` → proxy Go usado para interceptar requests
- `/ai-runtime` → runtime mínimo usado nos experimentos
- `/docs` → análises técnicas e comparações

## Hardware

- RX 6600 8GB
- Ollama
- Vulkan/DirectML
