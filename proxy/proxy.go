package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	listen  = flag.String("listen", ":11435", "listen address")
	target  = flag.String("target", "http://localhost:11434", "Ollama target URL") 
	logFile *os.File
) 

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Stream   bool      `json:"stream"`
	Tools    []struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	} `json:"tools,omitempty"`
}

type generateRequest struct {
	Model   string `json:"model"`
	Prompt  string `json:"prompt"`
	System  string `json:"system"`
	Stream  bool   `json:"stream"`
	Options struct {
		Temperature float64 `json:"temperature,omitempty"`
	} `json:"options,omitempty"`
}

type logEntry struct {
	TS                 string    `json:"ts"`
	Method             string    `json:"method"`
	Path               string    `json:"path"`
	AgentHeader        string    `json:"agent_header"`
	Model              string    `json:"model"`
	Stream             bool      `json:"stream"`
	Messages           []message `json:"messages,omitempty"`
	Tools              []string  `json:"tools,omitempty"`
	PromptTokensEst    int       `json:"prompt_tokens_est"`
	ResponseStatus     int       `json:"response_status"`
	LatencyMs          int64     `json:"latency_ms"`
	FinishReason       string    `json:"finish_reason,omitempty"`
	CompletionTokens   int       `json:"completion_tokens_est,omitempty"`
}

func main() {
	flag.Parse()
	var err error
	logFile, err = os.OpenFile("proxy.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening proxy.log: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	http.HandleFunc("/", handler)

	fmt.Printf("\033[36mOllama Proxy\033[0m \033[33m%s\033[0m \u2192 \033[33m%s\033[0m\n", *listen, *target)
	fmt.Printf("Log: \033[33mproxy.log\033[0m\n\n")
	log.Fatal(http.ListenAndServe(*listen, nil))
}

// handler is the root HTTP handler. It adds CORS headers, reads the body,
// then dispatches based on the path and stream flag.
func handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	if r.Method == "OPTIONS" {
		w.WriteHeader(204)
		return
	}

	body, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		http.Error(w, "bad request", 400)
		return
	}

	path := r.URL.Path
	targetURL := *target + path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	if !isChatPath(path) {
		simpleProxy(w, r, body, targetURL, path)
		return
	}

		// Ollama native generate endpoint
	if strings.HasSuffix(path, "/api/generate") || path == "/api/generate" {
		var genReq generateRequest

		if err := json.Unmarshal(body, &genReq); err != nil {
			stdout("\033[33m%s JSON parse error, proxying without log\033[0m\n", path)
			simpleProxy(w, r, body, targetURL, path)
			return
		}

		chatReq := chatRequest{
			Model:  genReq.Model,
			Stream: genReq.Stream,
			Messages: []message{
				{
					Role:    "system",
					Content: genReq.System,
				},
				{
					Role:    "user",
					Content: genReq.Prompt,
				},
			},
		}

		if chatReq.Stream {
			chatStreamHandler(w, r, body, targetURL, path, &chatReq)
		} else {
			chatHandler(w, r, body, targetURL, path, &chatReq)
		}

		return
	}

	var chatReq chatRequest
	if err := json.Unmarshal(body, &chatReq); err != nil {
		stdout("\033[33m%s JSON parse error, proxying without log\033[0m\n", path)
		simpleProxy(w, r, body, targetURL, path)
		return
	}

	if chatReq.Stream {
		chatStreamHandler(w, r, body, targetURL, path, &chatReq)
	} else {
		chatHandler(w, r, body, targetURL, path, &chatReq)
	}
}

func isChatPath(path string) bool {
	return strings.HasSuffix(path, "/chat/completions") ||
		path == "/api/chat" ||
		strings.HasSuffix(path, "/api/chat") ||
		path == "/api/generate" ||
		strings.HasSuffix(path, "/api/generate")
}

// simpleProxy forwards non-chat requests with minimal logging.
func simpleProxy(w http.ResponseWriter, r *http.Request, body []byte, targetURL, path string) {
	outReq, err := http.NewRequest(r.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	copyHeaders(outReq.Header, r.Header)
	outReq.Header.Set("Accept-Encoding", "identity")

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(outReq)
	if err != nil {
		stdout("\033[31mERROR\033[0m %s %s: %v\n", r.Method, path, err)
		http.Error(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

	line := fmt.Sprintf("[%s %s \u2192 %d]", r.Method, path, resp.StatusCode)
	stdout("\033[36m%s\033[0m\n", line)
}

// chatHandler handles non-streaming chat completion requests.
func chatHandler(w http.ResponseWriter, r *http.Request, body []byte, targetURL, path string, chatReq *chatRequest) {
	agent := detectAgent(r)
	start := time.Now()

	outReq, err := http.NewRequest(r.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	copyHeaders(outReq.Header, r.Header)
	outReq.Header.Set("Accept-Encoding", "identity")

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(outReq)
	if err != nil {
		stdout("\033[31mERROR\033[0m %s %s: %v\n", r.Method, path, err)
		http.Error(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)

	entry := makeLogEntry(chatReq, agent, start, resp.StatusCode, respBody, path)
	writeLog(entry)
}

// chatStreamHandler handles streaming chat completions. It forwards SSE
// chunks to the client in real-time while buffering them for logging.
func chatStreamHandler(w http.ResponseWriter, r *http.Request, body []byte, targetURL, path string, chatReq *chatRequest) {
	agent := detectAgent(r)
	start := time.Now()

	outReq, err := http.NewRequest(r.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	copyHeaders(outReq.Header, r.Header)
	outReq.Header.Set("Accept-Encoding", "identity")

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(outReq)
	if err != nil {
		stdout("\033[31mERROR\033[0m %s %s: %v\n", r.Method, path, err)
		http.Error(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	flusher, canFlush := w.(http.Flusher)
	var buf bytes.Buffer

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 65536), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		line = append(line, '\n')
		w.Write(line)
		buf.Write(line)
		if canFlush {
			flusher.Flush()
		}
	}

	entry := makeStreamLogEntry(chatReq, agent, start, resp.StatusCode, buf.Bytes(), path)
	writeLog(entry)
}

// detectAgent checks known headers to identify the coding agent.
func detectAgent(r *http.Request) string {
	if a := r.Header.Get("X-Agent-Name"); a != "" {
		return a
	}

	ua := strings.ToLower(r.UserAgent())
	pairs := []struct {
		keyword string
		name    string
	}{
		{"opencode", "opencode"},
		{"claude", "claude-code"},
		{"aider", "aider"},
		{"crush", "crush"},
		{"continue", "continue"},
		{"cursor", "cursor"},
		{"github-copilot", "github-copilot"},
		{"jetbrains", "jetbrains"},
	}
	for _, p := range pairs {
		if strings.Contains(ua, p.keyword) {
			return p.name
		}
	}

	if ua != "" {
		return ua
	}
	return "unknown"
}

// copyHeaders copies all headers from src to dst.
func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// parseStreamedResponse extracts content and finish_reason from a buffered
// SSE/stream response, handling both OpenAI and Ollama wire formats.
func parseStreamedResponse(data string) (content string, finishReason string) {
	var sb strings.Builder
	lines := strings.Split(data, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// OpenAI SSE: data: {...} or data: [DONE]
		if strings.HasPrefix(line, "data: ") {
			payload := strings.TrimPrefix(line, "data: ")
			if payload == "[DONE]" {
				continue
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(payload), &chunk); err == nil && len(chunk.Choices) > 0 {
				sb.WriteString(chunk.Choices[0].Delta.Content)
				if chunk.Choices[0].FinishReason != "" {
					finishReason = chunk.Choices[0].FinishReason
				}
				continue
			}
		}

		// Ollama native: bare JSON lines
		var chunk struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Done         bool   `json:"done"`
			DoneReason   string `json:"done_reason"`
			FinishReason string `json:"finish_reason"`
		}
		if err := json.Unmarshal([]byte(line), &chunk); err == nil {
			sb.WriteString(chunk.Message.Content)
			if chunk.FinishReason != "" {
				finishReason = chunk.FinishReason
			}
			if chunk.DoneReason != "" {
				finishReason = chunk.DoneReason
			}
			if chunk.Done && finishReason == "" {
				finishReason = "stop"
			}
		}
	}

	return sb.String(), finishReason
}

// parseResponse extracts content and finish_reason from a non-streaming
// JSON response body (OpenAI or Ollama format).
func parseResponse(data []byte) (content string, finishReason string) {
	var openAI struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &openAI); err == nil && len(openAI.Choices) > 0 {
		return openAI.Choices[0].Message.Content, openAI.Choices[0].FinishReason
	}

	var ollama struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		DoneReason string `json:"done_reason"`
	}
	if err := json.Unmarshal(data, &ollama); err == nil {
		fr := ollama.DoneReason
		if fr == "" {
			fr = "stop"
		}
		return ollama.Message.Content, fr
	}

	return "", ""
}

// makeLogEntry builds a logEntry for non-streaming requests.
func makeLogEntry(cr *chatRequest, agent string, start time.Time, status int, respBody []byte, path string) *logEntry {
	content, finishReason := parseResponse(respBody)
	return &logEntry{
		TS:               start.UTC().Format(time.RFC3339),
		Method:           "POST",
		Path:             path,
		AgentHeader:      agent,
		Model:            cr.Model,
		Stream:           false,
		Messages:         cr.Messages,
		Tools:            extractToolNames(cr.Tools),
		PromptTokensEst:  estimateTokens(cr),
		ResponseStatus:   status,
		LatencyMs:        time.Since(start).Milliseconds(),
		FinishReason:     finishReason,
		CompletionTokens: len(content) / 4,
	}
}

// makeStreamLogEntry builds a logEntry for streaming requests using the
// buffered SSE response.
func makeStreamLogEntry(cr *chatRequest, agent string, start time.Time, status int, respBody []byte, path string) *logEntry {
	content, finishReason := parseStreamedResponse(string(respBody))
	return &logEntry{
		TS:               start.UTC().Format(time.RFC3339),
		Method:           "POST",
		Path:             path,
		AgentHeader:      agent,
		Model:            cr.Model,
		Stream:           true,
		Messages:         cr.Messages,
		Tools:            extractToolNames(cr.Tools),
		PromptTokensEst:  estimateTokens(cr),
		ResponseStatus:   status,
		LatencyMs:        time.Since(start).Milliseconds(),
		FinishReason:     finishReason,
		CompletionTokens: len(content) / 4,
	}
}

func extractToolNames(tools []struct {
	Type     string `json:"type"`
	Function struct {
		Name string `json:"name"`
	} `json:"function"`
}) []string {
	if len(tools) == 0 {
		return nil
	}
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		n := t.Function.Name
		if n == "" {
			n = t.Type
		}
		if n != "" {
			names = append(names, n)
		}
	}
	return names
}

func estimateTokens(cr *chatRequest) int {
	total := 0
	for _, m := range cr.Messages {
		total += len(m.Content) / 4
	}
	for _, t := range cr.Tools {
		if t.Function.Name != "" {
			total += len(t.Function.Name)/4 + 5
		}
	}
	return total
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}

// writeLog writes the log entry to both stdout (colorized) and proxy.log (JSONL).
func writeLog(entry *logEntry) {
	writeStdout(entry)
	writeJSONL(entry)
}

func writeStdout(entry *logEntry) {
	// Divider
	div := strings.Repeat("\u2500", 60)
	stdout("\033[33m%s\033[0m\n", div)

	// Request line
	t, _ := time.Parse(time.RFC3339, entry.TS)
	stdout("\033[36m[%s]\033[0m %s %s\n",
		t.Format("2006-01-02 15:04:05"),
		entry.Method,
		entry.Path,
	)

	// Model
	stdout("\033[1;37mMODEL:\033[0m %s\n", entry.Model)

	// Agent
	stdout("\033[1;37mAGENT:\033[0m %s\n", entry.AgentHeader)

	// Estimated tokens
	stdout("\033[1;37mTOKENS estimados:\033[0m \033[35m~%d prompt\033[0m\n", entry.PromptTokensEst)

	// Messages
	stdout("\033[36mMESSAGES (%d):\033[0m\n", len(entry.Messages))
	for _, m := range entry.Messages {
	label := strings.ToUpper(m.Role)

	switch m.Role {
	case "system":
		stdout("  \033[35m[%s]\033[0m  %s\n", label, truncate(m.Content, 300))
	case "user":
		stdout("  \033[36m[%s]\033[0m  %s\n", label, truncate(m.Content, 300))
	default:
		stdout("  \033[1;37m[%s]\033[0m  %s\n", label, truncate(m.Content, 300))
		}
	}

	// Tools
	if len(entry.Tools) > 0 {
		stdout("\033[36mTOOLS declaradas:\033[0m %s\n", strings.Join(entry.Tools, ", "))
	}

	// Stream flag
	stdout("\033[1;37mSTREAM:\033[0m %t\n", entry.Stream)

	// Second divider
	stdout("\033[33m%s\033[0m\n", div)

	// Response line
	latency := float64(entry.LatencyMs) / 1000
	stdout("\033[32mRESPONSE em %.1fs | finish_reason: %s | ~%d tokens gerados\033[0m\n",
		latency,
		entry.FinishReason,
		entry.CompletionTokens,
	)

	// Final divider
	stdout("\033[33m%s\033[0m\n\n", div)
}

func writeJSONL(entry *logEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	data = append(data, '\n')
	logFile.Write(data)
}

func stdout(format string, a ...interface{}) {
	fmt.Fprintf(os.Stdout, format, a...)
}

func init() {
	log.SetFlags(0)
}
