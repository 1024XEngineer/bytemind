package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	reviewMetadataFiles = map[string]struct{}{
		"transcript.md": {},
		"user_notes.md": {},
		"metrics.json":  {},
	}
	reviewTextExts = map[string]struct{}{
		".txt": {}, ".md": {}, ".json": {}, ".csv": {}, ".js": {}, ".ts": {}, ".tsx": {}, ".jsx": {},
		".yaml": {}, ".yml": {}, ".xml": {}, ".html": {}, ".css": {}, ".sh": {}, ".rb": {}, ".go": {}, ".rs": {},
		".java": {}, ".c": {}, ".cpp": {}, ".h": {}, ".hpp": {}, ".sql": {}, ".r": {}, ".toml": {},
	}
	reviewImageExts = map[string]struct{}{
		".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".svg": {}, ".webp": {},
	}
	reviewMimeOverrides = map[string]string{
		".svg":  "image/svg+xml",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	}
)

type reviewRun struct {
	ID      string           `json:"id"`
	Prompt  string           `json:"prompt"`
	EvalID  any              `json:"eval_id"`
	Outputs []map[string]any `json:"outputs"`
	Grading map[string]any   `json:"grading,omitempty"`
}

type reviewServer struct {
	workspace     string
	skillName     string
	feedbackPath  string
	previous      map[string]map[string]any
	benchmarkPath string
}

func runGenerateReview(args []string) error {
	fs := flag.NewFlagSet("generate-review", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	port := fs.Int("port", 3117, "Server port")
	fs.IntVar(port, "p", 3117, "Server port")
	skillName := fs.String("skill-name", "", "Skill name for header")
	fs.StringVar(skillName, "n", "", "Skill name for header")
	previousWorkspace := fs.String("previous-workspace", "", "Previous iteration workspace")
	benchmarkPath := fs.String("benchmark", "", "benchmark.json path")
	staticOut := fs.String("static", "", "Write static HTML output instead of serving")
	fs.StringVar(staticOut, "s", "", "Write static HTML output instead of serving")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: go run ./internal/skills/skill-creator/tools generate-review <workspace-path> [--port 3117] [--skill-name name] [--previous-workspace dir] [--benchmark benchmark.json] [--static out.html]")
	}

	workspace := mustAbs(fs.Arg(0))
	if info, err := os.Stat(workspace); err != nil || !info.IsDir() {
		return fmt.Errorf("%s is not a directory", workspace)
	}

	runs := findReviewRuns(workspace)
	if len(runs) == 0 {
		return fmt.Errorf("no runs found in %s", workspace)
	}

	name := strings.TrimSpace(*skillName)
	if name == "" {
		name = strings.ReplaceAll(filepath.Base(workspace), "-workspace", "")
	}
	feedbackPath := filepath.Join(workspace, "feedback.json")

	previous := map[string]map[string]any{}
	if strings.TrimSpace(*previousWorkspace) != "" {
		prev, err := loadPreviousIteration(mustAbs(*previousWorkspace))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed loading previous workspace: %v\n", err)
		} else {
			previous = prev
		}
	}

	benchmarkObj := map[string]any(nil)
	if strings.TrimSpace(*benchmarkPath) != "" {
		_ = readJSONFile(*benchmarkPath, &benchmarkObj)
	}

	if strings.TrimSpace(*staticOut) != "" {
		html, err := generateReviewHTML(runs, name, previous, benchmarkObj)
		if err != nil {
			return err
		}
		outPath := mustAbs(*staticOut)
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(outPath, []byte(html), 0o644); err != nil {
			return err
		}
		fmt.Printf("\nStatic viewer written to: %s\n", outPath)
		return nil
	}

	srv := &reviewServer{
		workspace:     workspace,
		skillName:     name,
		feedbackPath:  feedbackPath,
		previous:      previous,
		benchmarkPath: strings.TrimSpace(*benchmarkPath),
	}

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return err
		}
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.handleIndex)
	mux.HandleFunc("/index.html", srv.handleIndex)
	mux.HandleFunc("/api/feedback", srv.handleFeedback)

	url := fmt.Sprintf("http://localhost:%d", actualPort)
	fmt.Println("\n  Eval Viewer")
	fmt.Println("  ---------------------------------")
	fmt.Printf("  URL:       %s\n", url)
	fmt.Printf("  Workspace: %s\n", workspace)
	fmt.Printf("  Feedback:  %s\n", feedbackPath)
	if len(previous) > 0 {
		fmt.Printf("  Previous:  %s (%d runs)\n", *previousWorkspace, len(previous))
	}
	if strings.TrimSpace(*benchmarkPath) != "" {
		fmt.Printf("  Benchmark: %s\n", *benchmarkPath)
	}
	fmt.Println()
	fmt.Println("  Press Ctrl+C to stop.")
	fmt.Println()
	openBrowser(url)

	httpServer := &http.Server{Handler: mux}
	return httpServer.Serve(listener)
}

func (s *reviewServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	runs := findReviewRuns(s.workspace)
	benchmarkObj := map[string]any(nil)
	if strings.TrimSpace(s.benchmarkPath) != "" {
		_ = readJSONFile(s.benchmarkPath, &benchmarkObj)
	}
	html, err := generateReviewHTML(runs, s.skillName, s.previous, benchmarkObj)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

func (s *reviewServer) handleFeedback(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		data := []byte("{}")
		if raw, err := os.ReadFile(s.feedbackPath); err == nil {
			data = raw
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	case http.MethodPost:
		defer r.Body.Close()
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		if _, ok := payload["reviews"]; !ok {
			http.Error(w, `{"error":"Expected JSON object with 'reviews' key"}`, http.StatusBadRequest)
			return
		}
		if err := writeJSONFile(s.feedbackPath, payload); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func findReviewRuns(workspace string) []reviewRun {
	runs := make([]reviewRun, 0)
	findRunsRecursive(workspace, workspace, &runs)
	sort.Slice(runs, func(i, j int) bool {
		iEval := asFloat(runs[i].EvalID)
		jEval := asFloat(runs[j].EvalID)
		if iEval == jEval {
			return runs[i].ID < runs[j].ID
		}
		return iEval < jEval
	})
	return runs
}

func findRunsRecursive(root, current string, runs *[]reviewRun) {
	entries, err := os.ReadDir(current)
	if err != nil {
		return
	}
	outputsDir := filepath.Join(current, "outputs")
	if info, err := os.Stat(outputsDir); err == nil && info.IsDir() {
		if run := buildReviewRun(root, current); run != nil {
			*runs = append(*runs, *run)
		}
		return
	}

	skip := map[string]struct{}{"node_modules": {}, ".git": {}, "skill": {}, "inputs": {}}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, blocked := skip[entry.Name()]; blocked {
			continue
		}
		findRunsRecursive(root, filepath.Join(current, entry.Name()), runs)
	}
}

func buildReviewRun(root, runDir string) *reviewRun {
	prompt := ""
	var evalID any = nil
	metaCandidates := []string{filepath.Join(runDir, "eval_metadata.json"), filepath.Join(filepath.Dir(runDir), "eval_metadata.json")}
	for _, path := range metaCandidates {
		var meta map[string]any
		if err := readJSONFile(path, &meta); err == nil {
			prompt = asString(meta["prompt"])
			evalID = meta["eval_id"]
			if prompt != "" {
				break
			}
		}
	}

	if prompt == "" {
		transcriptCandidates := []string{filepath.Join(runDir, "transcript.md"), filepath.Join(runDir, "outputs", "transcript.md")}
		re := regexp.MustCompile(`(?s)## Eval Prompt\n\n(.*?)(?:\n##|$)`)
		for _, path := range transcriptCandidates {
			if data, err := os.ReadFile(path); err == nil {
				match := re.FindStringSubmatch(string(data))
				if len(match) >= 2 {
					prompt = strings.TrimSpace(match[1])
					break
				}
			}
		}
	}
	if prompt == "" {
		prompt = "(No prompt found)"
	}

	rel, err := filepath.Rel(root, runDir)
	if err != nil {
		rel = runDir
	}
	runID := strings.ReplaceAll(filepath.ToSlash(rel), "/", "-")

	outputs := make([]map[string]any, 0)
	outputDir := filepath.Join(runDir, "outputs")
	if entries, err := os.ReadDir(outputDir); err == nil {
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if _, skip := reviewMetadataFiles[entry.Name()]; skip {
				continue
			}
			outputs = append(outputs, embedReviewFile(filepath.Join(outputDir, entry.Name())))
		}
	}

	var grading map[string]any
	for _, p := range []string{filepath.Join(runDir, "grading.json"), filepath.Join(filepath.Dir(runDir), "grading.json")} {
		if err := readJSONFile(p, &grading); err == nil && len(grading) > 0 {
			break
		}
	}

	return &reviewRun{ID: runID, Prompt: prompt, EvalID: evalID, Outputs: outputs, Grading: grading}
}

func embedReviewFile(path string) map[string]any {
	ext := strings.ToLower(filepath.Ext(path))
	mimeType := reviewMIME(path)
	name := filepath.Base(path)

	if _, ok := reviewTextExts[ext]; ok {
		data, err := os.ReadFile(path)
		if err != nil {
			return map[string]any{"name": name, "type": "error", "content": "(Error reading file)"}
		}
		return map[string]any{"name": name, "type": "text", "content": string(data)}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{"name": name, "type": "error", "content": "(Error reading file)"}
	}
	b64 := base64.StdEncoding.EncodeToString(raw)

	if _, ok := reviewImageExts[ext]; ok {
		return map[string]any{"name": name, "type": "image", "mime": mimeType, "data_uri": "data:" + mimeType + ";base64," + b64}
	}
	if ext == ".pdf" {
		return map[string]any{"name": name, "type": "pdf", "data_uri": "data:" + mimeType + ";base64," + b64}
	}
	if ext == ".xlsx" {
		return map[string]any{"name": name, "type": "xlsx", "data_b64": b64}
	}
	return map[string]any{"name": name, "type": "binary", "mime": mimeType, "data_uri": "data:" + mimeType + ";base64," + b64}
}

func reviewMIME(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if v, ok := reviewMimeOverrides[ext]; ok {
		return v
	}
	if guessed := mime.TypeByExtension(ext); guessed != "" {
		return guessed
	}
	return "application/octet-stream"
}

func loadPreviousIteration(workspace string) (map[string]map[string]any, error) {
	result := make(map[string]map[string]any)
	feedbackMap := map[string]string{}
	feedbackPath := filepath.Join(workspace, "feedback.json")
	var feedback map[string]any
	if err := readJSONFile(feedbackPath, &feedback); err == nil {
		for _, item := range asMapSlice(feedback["reviews"]) {
			runID := asString(item["run_id"])
			text := strings.TrimSpace(asString(item["feedback"]))
			if runID != "" && text != "" {
				feedbackMap[runID] = text
			}
		}
	}
	for _, run := range findReviewRuns(workspace) {
		outputs := make([]any, 0, len(run.Outputs))
		for _, out := range run.Outputs {
			outputs = append(outputs, out)
		}
		result[run.ID] = map[string]any{
			"feedback": feedbackMap[run.ID],
			"outputs":  outputs,
		}
	}
	for runID, text := range feedbackMap {
		if _, ok := result[runID]; !ok {
			result[runID] = map[string]any{"feedback": text, "outputs": []any{}}
		}
	}
	return result, nil
}

func generateReviewHTML(runs []reviewRun, skillName string, previous map[string]map[string]any, benchmark map[string]any) (string, error) {
	previousFeedback := map[string]string{}
	previousOutputs := map[string]any{}
	for runID, payload := range previous {
		if text := strings.TrimSpace(asString(payload["feedback"])); text != "" {
			previousFeedback[runID] = text
		}
		if outs, ok := payload["outputs"]; ok {
			previousOutputs[runID] = outs
		}
	}

	runsAny := make([]any, 0, len(runs))
	for _, run := range runs {
		runsAny = append(runsAny, run)
	}
	embedded := map[string]any{
		"skill_name":        skillName,
		"runs":              runsAny,
		"previous_feedback": previousFeedback,
		"previous_outputs":  previousOutputs,
	}
	if benchmark != nil {
		embedded["benchmark"] = benchmark
	}

	jsonData, err := json.Marshal(embedded)
	if err != nil {
		return "", err
	}
	return strings.Replace(reviewInlinePageTemplate, "__EMBEDDED_JSON__", string(jsonData), 1), nil
}

const reviewInlinePageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Eval Review</title>
  <script src="https://cdn.sheetjs.com/xlsx-0.20.3/package/dist/xlsx.full.min.js" integrity="sha384-EnyY0/GSHQGSxSgMwaIPzSESbqoOLSexfnSMN2AP+39Ckmn92stwABZynq1JyzdT" crossorigin="anonymous"></script>
  <style>
    :root {
      --bg: #f7f7f5;
      --surface: #fff;
      --border: #e0e0dc;
      --text: #141413;
      --muted: #6b6b63;
      --accent: #c4613f;
      --green: #2f7a38;
      --red: #b23a3a;
    }
    * { box-sizing: border-box; }
    body { margin: 0; font-family: -apple-system, BlinkMacSystemFont, Segoe UI, Roboto, Helvetica, Arial, sans-serif; background: var(--bg); color: var(--text); }
    .wrap { max-width: 1200px; margin: 0 auto; padding: 16px; }
    .header { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: 12px; }
    .title { font-size: 20px; font-weight: 700; }
    .progress { color: var(--muted); font-size: 13px; }
    .card { background: var(--surface); border: 1px solid var(--border); border-radius: 8px; margin-bottom: 12px; overflow: hidden; }
    .card h3 { margin: 0; padding: 10px 12px; border-bottom: 1px solid var(--border); font-size: 13px; text-transform: uppercase; letter-spacing: 0.03em; color: var(--muted); }
    .card .body { padding: 12px; }
    .prompt { white-space: pre-wrap; line-height: 1.6; font-size: 14px; }
    .output { border: 1px solid var(--border); border-radius: 6px; margin-bottom: 10px; overflow: hidden; }
    .output-header { background: #fafaf8; border-bottom: 1px solid var(--border); padding: 8px 10px; display: flex; justify-content: space-between; align-items: center; font-size: 12px; color: var(--muted); font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
    .output-body { padding: 10px; overflow-x: auto; }
    .output-body pre { margin: 0; white-space: pre-wrap; word-break: break-word; font-size: 12px; line-height: 1.5; font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
    .output-body img { max-width: 100%; height: auto; display: block; border-radius: 4px; }
    .output-body iframe { width: 100%; height: 560px; border: none; border-radius: 4px; }
    .hint { color: var(--muted); font-size: 12px; }
    textarea { width: 100%; min-height: 120px; resize: vertical; border: 1px solid var(--border); border-radius: 6px; padding: 10px; font: inherit; font-size: 14px; line-height: 1.5; }
    .status { margin-top: 6px; color: var(--muted); font-size: 12px; min-height: 16px; }
    .prev-feedback { margin-top: 8px; border: 1px solid var(--border); border-radius: 6px; background: #fafaf8; padding: 8px 10px; font-size: 12px; color: var(--muted); display: none; }
    .nav { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
    button { border: 1px solid var(--border); border-radius: 6px; background: #fff; color: var(--text); cursor: pointer; padding: 8px 12px; font-size: 13px; }
    button:hover { background: #f2f2ee; }
    .submit { border-color: transparent; background: var(--accent); color: #fff; font-weight: 600; }
    .submit:hover { filter: brightness(0.95); }
    .badge { display: inline-block; margin-left: 8px; border-radius: 999px; padding: 2px 8px; font-size: 11px; background: #eee; color: #444; vertical-align: middle; }
    .grade-ok { color: var(--green); font-weight: 600; }
    .grade-fail { color: var(--red); font-weight: 600; }
    .benchmark pre { margin: 0; white-space: pre-wrap; word-break: break-word; font-size: 12px; line-height: 1.5; font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="header">
      <div class="title">Eval Review: <span id="skill-name"></span></div>
      <div class="progress" id="progress"></div>
    </div>

    <div class="card">
      <h3>Prompt <span class="badge" id="config-badge" style="display:none"></span></h3>
      <div class="body"><div class="prompt" id="prompt"></div></div>
    </div>

    <div class="card">
      <h3>Outputs</h3>
      <div class="body" id="outputs"></div>
    </div>

    <div class="card">
      <h3>Feedback</h3>
      <div class="body">
        <textarea id="feedback" placeholder="Describe issues, suggestions, or what looks good."></textarea>
        <div class="status" id="status"></div>
        <div class="prev-feedback" id="prev-feedback"></div>
      </div>
    </div>

    <div class="card benchmark" id="benchmark-card" style="display:none">
      <h3>Benchmark</h3>
      <div class="body"><pre id="benchmark"></pre></div>
    </div>

    <div class="nav">
      <button id="prev-btn" onclick="navigate(-1)">← Previous</button>
      <button class="submit" onclick="submitAll()">Submit All Reviews</button>
      <button id="next-btn" onclick="navigate(1)">Next →</button>
    </div>
  </div>

  <script>
    const EMBEDDED_DATA = __EMBEDDED_JSON__;
    let feedbackMap = {};
    let currentIndex = 0;
    let saveTimer = null;

    function byId(id) { return document.getElementById(id); }
    function esc(s) { return String(s || "").replace(/[&<>"']/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[m])); }

    async function init() {
      byId("skill-name").textContent = EMBEDDED_DATA.skill_name || "unknown";
      try {
        const resp = await fetch("/api/feedback");
        if (resp.ok) {
          const data = await resp.json();
          if (Array.isArray(data.reviews)) {
            for (const r of data.reviews) {
              if (typeof r.run_id === "string") feedbackMap[r.run_id] = String(r.feedback || "");
            }
          }
        }
      } catch (_) {}

      byId("feedback").addEventListener("input", () => {
        clearTimeout(saveTimer);
        byId("status").textContent = "";
        saveTimer = setTimeout(saveCurrent, 700);
      });

      if (EMBEDDED_DATA.benchmark) {
        byId("benchmark-card").style.display = "block";
        byId("benchmark").textContent = JSON.stringify(EMBEDDED_DATA.benchmark, null, 2);
      }

      render();
    }

    function getRuns() { return Array.isArray(EMBEDDED_DATA.runs) ? EMBEDDED_DATA.runs : []; }
    function currentRun() { const runs = getRuns(); return runs[currentIndex] || null; }

    function render() {
      const runs = getRuns();
      if (!runs.length) {
        byId("prompt").textContent = "No runs found.";
        byId("outputs").innerHTML = "<div class=\"hint\">No output files.</div>";
        byId("progress").textContent = "0 / 0";
        return;
      }
      const run = currentRun();
      byId("progress").textContent = String(currentIndex + 1) + " / " + String(runs.length);
      byId("prompt").textContent = run.prompt || "(No prompt found)";
      byId("prev-btn").disabled = currentIndex <= 0;
      byId("next-btn").disabled = currentIndex >= runs.length - 1;

      const configBadge = byId("config-badge");
      const parts = String(run.id || "").split("-");
      if (parts.length >= 2) {
        const cfg = parts[parts.length - 2];
        if (cfg && cfg !== "eval") {
          configBadge.style.display = "inline-block";
          configBadge.textContent = cfg;
        } else {
          configBadge.style.display = "none";
        }
      } else {
        configBadge.style.display = "none";
      }

      renderOutputs(run.outputs || []);
      byId("feedback").value = feedbackMap[run.id] || "";
      byId("status").textContent = "";

      const prev = (EMBEDDED_DATA.previous_feedback || {})[run.id];
      const prevEl = byId("prev-feedback");
      if (prev && String(prev).trim()) {
        prevEl.style.display = "block";
        prevEl.innerHTML = "<strong>Previous feedback:</strong><br>" + esc(prev);
      } else {
        prevEl.style.display = "none";
        prevEl.textContent = "";
      }
    }

    function renderOutputs(files) {
      const root = byId("outputs");
      root.innerHTML = "";
      if (!Array.isArray(files) || files.length === 0) {
        root.innerHTML = "<div class=\"hint\">No output files found.</div>";
        return;
      }
      for (const file of files) {
        const wrapper = document.createElement("div");
        wrapper.className = "output";
        const header = document.createElement("div");
        header.className = "output-header";
        const name = document.createElement("span");
        name.textContent = file.name || "output";
        header.appendChild(name);
        const dl = document.createElement("a");
        dl.textContent = "Download";
        dl.href = downloadUri(file);
        dl.download = file.name || "output";
        dl.style.color = "#c4613f";
        dl.style.textDecoration = "none";
        dl.style.fontFamily = "inherit";
        header.appendChild(dl);
        wrapper.appendChild(header);

        const body = document.createElement("div");
        body.className = "output-body";

        if (file.type === "text") {
          const pre = document.createElement("pre");
          pre.textContent = file.content || "";
          body.appendChild(pre);
        } else if (file.type === "image") {
          const img = document.createElement("img");
          img.src = file.data_uri || "";
          img.alt = file.name || "";
          body.appendChild(img);
        } else if (file.type === "pdf") {
          const iframe = document.createElement("iframe");
          iframe.src = file.data_uri || "";
          body.appendChild(iframe);
        } else if (file.type === "xlsx") {
          renderXlsx(body, file.data_b64 || "");
        } else {
          const link = document.createElement("a");
          link.href = downloadUri(file);
          link.download = file.name || "file";
          link.textContent = "Download " + (file.name || "file");
          link.style.color = "#c4613f";
          body.appendChild(link);
        }
        wrapper.appendChild(body);
        root.appendChild(wrapper);
      }
    }

    function renderXlsx(container, b64) {
      if (!b64) {
        container.textContent = "No spreadsheet content.";
        return;
      }
      if (typeof XLSX === "undefined") {
        const msg = document.createElement("div");
        msg.className = "hint";
        msg.textContent = "SheetJS is unavailable. Please download the file to inspect.";
        container.appendChild(msg);
        return;
      }
      try {
        const binary = atob(b64);
        const bytes = Uint8Array.from(binary, c => c.charCodeAt(0));
        const wb = XLSX.read(bytes, { type: "array" });
        wb.SheetNames.forEach((sheetName, idx) => {
          if (wb.SheetNames.length > 1) {
            const label = document.createElement("div");
            label.className = "hint";
            label.style.marginBottom = "6px";
            label.textContent = "Sheet: " + sheetName;
            container.appendChild(label);
          }
          const ws = wb.Sheets[sheetName];
          const htmlTable = XLSX.utils.sheet_to_html(ws, { editable: false });
          const box = document.createElement("div");
          box.innerHTML = htmlTable;
          container.appendChild(box);
        });
      } catch (err) {
        container.textContent = "Error rendering spreadsheet: " + String(err);
      }
    }

    function downloadUri(file) {
      if (file.data_uri) return file.data_uri;
      if (file.data_b64) return "data:application/octet-stream;base64," + file.data_b64;
      if (file.type === "text") return "data:text/plain;charset=utf-8," + encodeURIComponent(file.content || "");
      return "#";
    }

    function navigate(step) {
      saveCurrent();
      const runs = getRuns();
      currentIndex = Math.max(0, Math.min(runs.length - 1, currentIndex + step));
      render();
    }

    function saveCurrent() {
      const run = currentRun();
      if (!run) return;
      const value = byId("feedback").value || "";
      if (value.trim()) feedbackMap[run.id] = value;
      else delete feedbackMap[run.id];

      const reviews = Object.entries(feedbackMap).map(([run_id, feedback]) => ({
        run_id, feedback, timestamp: new Date().toISOString()
      }));
      fetch("/api/feedback", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ reviews, status: "in_progress" })
      }).then(() => {
        byId("status").textContent = "Saved";
      }).catch(() => {
        byId("status").textContent = "Will download on submit";
      });
    }

    function submitAll() {
      saveCurrent();
      const runs = getRuns();
      const ts = new Date().toISOString();
      const reviews = runs.map(r => ({
        run_id: r.id,
        feedback: feedbackMap[r.id] || "",
        timestamp: ts
      }));
      const payload = JSON.stringify({ reviews, status: "complete" }, null, 2);
      fetch("/api/feedback", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: payload
      }).then(() => {
        alert("Review complete. Return to your Codex session.");
      }).catch(() => {
        const blob = new Blob([payload], { type: "application/json" });
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = "feedback.json";
        a.click();
        URL.revokeObjectURL(url);
        alert("Saved as feedback.json. Return to your Codex session.");
      });
    }

    document.addEventListener("keydown", (e) => {
      if (e.target && e.target.tagName === "TEXTAREA") return;
      if (e.key === "ArrowLeft" || e.key === "ArrowUp") navigate(-1);
      if (e.key === "ArrowRight" || e.key === "ArrowDown") navigate(1);
    });

    init();
  </script>
</body>
</html>
`
