package main

import (
	"bufio"
	"crypto/sha1" // #nosec G505 -- WebSocket opening handshake requires SHA-1 by RFC 6455.
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"omni-bot-go/database"
	"omni-bot-go/engine"
	"omni-bot-go/knowledge"
	"omni-bot-go/mcp"
)

var (
	appName   = "omni-bot-go"
	version   = "dev"
	commit    = "local"
	buildTime = "unknown"
)

func main() {
	dbPath := flag.String("db", filepath.Join("data", "loja.db"), "caminho do SQLite")
	addr := flag.String("addr", "127.0.0.1:8080", "endereco do dashboard")
	commandsPath := flag.String("commands", "", "caminho opcional de YAML operacional para criar contrato e tarefas")
	pluginsDir := flag.String("plugins", "plugins", "diretorio de manifestos MCP/plugins JSON")
	logPath := flag.String("log", filepath.Join("logs", "omni-bot-go.log"), "arquivo de log do runtime")
	showVersion := flag.Bool("version", false, "exibe versao do binario e sai")
	flag.Parse()
	if *showVersion {
		fmt.Printf("%s %s commit=%s build=%s\n", appName, version, commit, buildTime)
		return
	}
	if err := ensureRuntimeDirs(*dbPath, *logPath, *pluginsDir); err != nil {
		log.Fatal(err)
	}
	logFile, err := initRuntimeLog(*logPath)
	if err != nil {
		log.Fatal(err)
	}
	if logFile != nil {
		defer logFile.Close()
	}

	store, err := database.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	eng, err := engine.New()
	if err != nil {
		log.Fatal(err)
	}
	defer eng.Close()

	bus := NewEventBus()
	manager := NewManager(store, eng, knowledge.New(store), bus)
	manager.pluginsDir = *pluginsDir
	if loaded := manager.LoadDiskKnowledgePacks("knowledge"); loaded > 0 {
		log.Printf("%d knowledge pack(s) carregado(s) do disco", loaded)
	}
	manager.Start()
	defer manager.Stop()
	if strings.TrimSpace(*commandsPath) != "" {
		contract, err := manager.LoadBlueprintFile(*commandsPath)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("YAML operacional carregado como contrato #%d", contract.ID)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveIndex)
	mux.HandleFunc("/ws", wsHandler(bus, manager))
	mux.HandleFunc("/api/snapshot", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		if r.Method != http.MethodGet {
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
		return manager.Snapshot(), http.StatusOK, nil
	}))
	mux.HandleFunc("/api/version", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		if r.Method != http.MethodGet {
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
		return buildInfo(), http.StatusOK, nil
	}))
	mux.HandleFunc("/api/health", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		if r.Method != http.MethodGet {
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
		health := runtimeHealth(manager, *dbPath, *pluginsDir, *logPath)
		code := http.StatusOK
		if health["status"] != "ok" {
			code = http.StatusServiceUnavailable
		}
		return health, code, nil
	}))
	mux.HandleFunc("/api/engine", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		if r.Method != http.MethodGet {
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
		return eng.Info(), http.StatusOK, nil
	}))
	mux.HandleFunc("/api/projects", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		if r.Method != http.MethodPost {
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
		var body struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, http.StatusBadRequest, err
		}
		contract, err := manager.CreateProject(body.Input)
		if err != nil {
			return nil, http.StatusBadRequest, err
		}
		return contract, http.StatusCreated, nil
	}))
	mux.HandleFunc("/api/projects/yaml", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		if r.Method != http.MethodPost {
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
		var body struct {
			YAML string `json:"yaml"`
		}
		if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				return nil, http.StatusBadRequest, err
			}
		} else {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, http.StatusBadRequest, err
			}
			body.YAML = string(raw)
		}
		contract, err := manager.CreateProjectFromYAML(body.YAML)
		if err != nil {
			return nil, http.StatusBadRequest, err
		}
		return contract, http.StatusCreated, nil
	}))
	mux.HandleFunc("/api/factory_series", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		if r.Method != http.MethodPost {
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
		var body FactorySeriesRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, http.StatusBadRequest, err
		}
		result, err := manager.CreateFactorySeries(body)
		if err != nil {
			return nil, http.StatusBadRequest, err
		}
		return result, http.StatusCreated, nil
	}))
	mux.HandleFunc("/api/factory_batches/action", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		if r.Method != http.MethodPost {
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
		var body FactoryBatchActionRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, http.StatusBadRequest, err
		}
		result, err := manager.FactoryBatchAction(body)
		if err != nil {
			return nil, http.StatusBadRequest, err
		}
		return result, http.StatusOK, nil
	}))
	mux.HandleFunc("/api/plugins", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		if r.Method != http.MethodGet {
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
		registry, err := manager.Plugins()
		if err != nil {
			return nil, http.StatusBadRequest, err
		}
		return registry, http.StatusOK, nil
	}))
	mux.HandleFunc("/api/plugins/call", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		if r.Method != http.MethodPost {
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
		var body mcp.CallRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, http.StatusBadRequest, err
		}
		result, err := manager.CallPlugin(body)
		if err != nil {
			return result, http.StatusBadRequest, err
		}
		return result, http.StatusOK, nil
	}))
	mux.HandleFunc("/api/plugin_permissions", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		switch r.Method {
		case http.MethodGet:
			items, err := manager.PluginPermissionScopes(queryInt(r, "limit", 120))
			if err != nil {
				return nil, http.StatusInternalServerError, err
			}
			return items, http.StatusOK, nil
		case http.MethodPost:
			var body database.PluginPermissionScope
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				return nil, http.StatusBadRequest, err
			}
			item, err := manager.UpsertPluginPermissionScope(body)
			if err != nil {
				return nil, http.StatusBadRequest, err
			}
			return item, http.StatusOK, nil
		default:
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
	}))
	knowledgePacksHandler := jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		switch r.Method {
		case http.MethodGet:
			limit := queryInt(r, "limit", 80)
			query := strings.TrimSpace(r.URL.Query().Get("q"))
			domain := strings.TrimSpace(r.URL.Query().Get("domain"))
			rule := strings.TrimSpace(r.URL.Query().Get("rule"))
			pattern := strings.TrimSpace(r.URL.Query().Get("pattern"))
			if query != "" || domain != "" || rule != "" || pattern != "" {
				results, err := manager.SearchKnowledgePacksWithOptions(knowledge.SearchOptions{
					Query:   query,
					Domain:  domain,
					Rule:    rule,
					Pattern: pattern,
					Limit:   limit,
				})
				if err != nil {
					return nil, http.StatusBadRequest, err
				}
				return results, http.StatusOK, nil
			}
			items, err := manager.KnowledgePackSummaries(limit)
			if err != nil {
				return nil, http.StatusInternalServerError, err
			}
			return items, http.StatusOK, nil
		case http.MethodPost:
			var body struct {
				Topic  string `json:"topic"`
				Source string `json:"source"`
				Text   string `json:"text"`
				Path   string `json:"path"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				return nil, http.StatusBadRequest, err
			}
			item, err := manager.IngestKnowledgePack(body.Topic, body.Source, body.Text, body.Path)
			if err != nil {
				return nil, http.StatusBadRequest, err
			}
			return item, http.StatusCreated, nil
		default:
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
	})
	mux.HandleFunc("/api/knowledge_packs", knowledgePacksHandler)
	purgeKnowledgePacksHandler := jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		if r.Method != http.MethodPost {
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
		removed, err := manager.knowledgePacks.PurgeExpired()
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
		return map[string]any{"removed": removed}, http.StatusOK, nil
	})
	mux.HandleFunc("/api/knowledge_packs/purge", purgeKnowledgePacksHandler)
	knowledgeCandidatesHandler := jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		switch r.Method {
		case http.MethodGet:
			status := strings.TrimSpace(r.URL.Query().Get("status"))
			limit := queryInt(r, "limit", 80)
			items, err := manager.KnowledgeCandidateSummaries(status, limit)
			if err != nil {
				return nil, http.StatusInternalServerError, err
			}
			return items, http.StatusOK, nil
		case http.MethodPost:
			var body struct {
				ID     int64  `json:"id"`
				Action string `json:"action"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				return nil, http.StatusBadRequest, err
			}
			if body.ID == 0 {
				return nil, http.StatusBadRequest, fmt.Errorf("id da regra candidata ausente")
			}
			switch body.Action {
			case "approve":
				item, err := manager.ApproveKnowledgeCandidate(body.ID)
				if err != nil {
					return nil, http.StatusBadRequest, err
				}
				return item, http.StatusOK, nil
			case "reject":
				item, err := manager.RejectKnowledgeCandidate(body.ID)
				if err != nil {
					return nil, http.StatusBadRequest, err
				}
				return item, http.StatusOK, nil
			default:
				return nil, http.StatusBadRequest, fmt.Errorf("acao de regra candidata desconhecida")
			}
		default:
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
	})
	mux.HandleFunc("/api/knowledge_candidates", knowledgeCandidatesHandler)
	mux.HandleFunc("/api/interrogations", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		if r.Method != http.MethodPost {
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
		var body struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, http.StatusBadRequest, err
		}
		session, err := manager.StartInterrogation(body.Input)
		if err != nil {
			return nil, http.StatusBadRequest, err
		}
		return session, http.StatusCreated, nil
	}))
	mux.HandleFunc("/api/interrogations/preview", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		if r.Method != http.MethodPost {
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
		var body struct {
			ID      int64             `json:"id"`
			Answers map[string]string `json:"answers"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, http.StatusBadRequest, err
		}
		if body.ID == 0 {
			return nil, http.StatusBadRequest, fmt.Errorf("id do interrogatorio ausente")
		}
		draft, err := manager.PreviewInterrogationContract(body.ID, body.Answers)
		if err != nil {
			return nil, http.StatusBadRequest, err
		}
		return draft, http.StatusOK, nil
	}))
	mux.HandleFunc("/api/interrogations/finalize", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		if r.Method != http.MethodPost {
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
		var body struct {
			ID           int64             `json:"id"`
			Answers      map[string]string `json:"answers"`
			NorthStar    string            `json:"north_star"`
			Constraints  string            `json:"constraints"`
			Deliverables string            `json:"deliverables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, http.StatusBadRequest, err
		}
		if body.ID == 0 {
			return nil, http.StatusBadRequest, fmt.Errorf("id do interrogatorio ausente")
		}
		if body.Answers == nil {
			body.Answers = map[string]string{}
		}
		contract, err := manager.FinalizeInterrogationWithDraft(body.ID, body.Answers, ContractDraft{
			NorthStar:    body.NorthStar,
			Constraints:  body.Constraints,
			Deliverables: body.Deliverables,
		})
		if err != nil {
			return nil, http.StatusBadRequest, err
		}
		return contract, http.StatusCreated, nil
	}))
	mux.HandleFunc("/api/panic", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		if r.Method != http.MethodPost {
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
		var body struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := manager.Panic(body.Reason); err != nil {
			return nil, http.StatusInternalServerError, err
		}
		return map[string]string{"status": "bloqueado"}, http.StatusOK, nil
	}))
	mux.HandleFunc("/api/resume", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		if r.Method != http.MethodPost {
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
		if err := manager.Resume(); err != nil {
			return nil, http.StatusInternalServerError, err
		}
		return map[string]string{"status": "ativo"}, http.StatusOK, nil
	}))
	mux.HandleFunc("/api/tasks/action", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		if r.Method != http.MethodPost {
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
		var body struct {
			ID     int64  `json:"id"`
			Action string `json:"action"`
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, http.StatusBadRequest, err
		}
		if body.ID == 0 {
			return nil, http.StatusBadRequest, fmt.Errorf("id da tarefa ausente")
		}
		switch body.Action {
		case "cancel":
			if err := manager.CancelTask(body.ID, body.Reason); err != nil {
				return nil, http.StatusBadRequest, err
			}
		case "retry":
			if err := manager.RetryTask(body.ID, body.Reason); err != nil {
				return nil, http.StatusBadRequest, err
			}
		default:
			return nil, http.StatusBadRequest, fmt.Errorf("acao de tarefa desconhecida")
		}
		return manager.Snapshot(), http.StatusOK, nil
	}))
	mux.HandleFunc("/api/contracts/reaudit", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		if r.Method != http.MethodPost {
			return nil, http.StatusMethodNotAllowed, fmt.Errorf("metodo nao permitido")
		}
		var body struct {
			ID int64 `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, http.StatusBadRequest, err
		}
		if body.ID == 0 {
			return nil, http.StatusBadRequest, fmt.Errorf("id do contrato ausente")
		}
		task, err := manager.ReauditContract(body.ID)
		if err != nil {
			return nil, http.StatusBadRequest, err
		}
		return task, http.StatusCreated, nil
	}))

	server := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	log.Printf("OBG %s (%s) dashboard em http://%s", version, commit, *addr)
	log.Fatal(server.ListenAndServe())
}

func buildInfo() map[string]string {
	return map[string]string{
		"app":        appName,
		"version":    version,
		"commit":     commit,
		"build_time": buildTime,
	}
}

func ensureRuntimeDirs(paths ...string) error {
	required := []string{"data", "logs", "projects"}
	for _, dir := range required {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		dir := path
		if filepath.Ext(path) != "" {
			dir = filepath.Dir(path)
		}
		if dir == "." || dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	return nil
}

func initRuntimeLog(path string) (*os.File, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	if err := rotateRuntimeLog(path, 10*1024*1024, 3); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	log.SetOutput(io.MultiWriter(os.Stdout, file))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	return file, nil
}

func rotateRuntimeLog(path string, maxBytes int64, keep int) error {
	if maxBytes <= 0 || keep <= 0 {
		return nil
	}
	clean := filepath.Clean(path)
	info, err := os.Stat(clean)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Size() < maxBytes {
		return nil
	}
	for i := keep - 1; i >= 1; i-- {
		from := fmt.Sprintf("%s.%d", clean, i)
		to := fmt.Sprintf("%s.%d", clean, i+1)
		if _, err := os.Stat(from); err == nil {
			_ = os.Remove(to)
			if err := os.Rename(from, to); err != nil {
				return err
			}
		}
	}
	first := clean + ".1"
	_ = os.Remove(first)
	return os.Rename(clean, first)
}

func runtimeHealth(manager *Manager, dbPath, pluginsDir, logPath string) map[string]any {
	checks := map[string]any{}
	status := "ok"
	if err := manager.store.Ping(); err != nil {
		status = "degraded"
		checks["database"] = map[string]string{"status": "error", "error": err.Error()}
	} else {
		checks["database"] = map[string]string{"status": "ok", "path": filepath.Clean(dbPath)}
	}
	checks["engine"] = map[string]any{"status": "symbolic", "info": manager.engine.Info()}
	registry, err := manager.Plugins()
	if err != nil {
		status = "degraded"
		checks["plugins"] = map[string]string{"status": "error", "dir": pluginsDir, "error": err.Error()}
	} else {
		checks["plugins"] = map[string]any{"status": "ok", "dir": registry.Dir, "count": len(registry.Plugins)}
	}
	for _, dir := range []string{"data", "logs", "projects"} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			status = "degraded"
			checks["dir:"+dir] = map[string]string{"status": "error"}
		} else {
			checks["dir:"+dir] = map[string]string{"status": "ok"}
		}
	}
	checks["log"] = map[string]string{"status": "ok", "path": filepath.Clean(logPath)}
	return map[string]any{
		"status": status,
		"build":  buildInfo(),
		"checks": checks,
	}
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join("web", "index.html")
	raw, err := os.ReadFile(path) // #nosec G304 -- dashboard path is a fixed local asset joined from constants.
	if err != nil {
		http.Error(w, "web/index.html nao encontrado", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(raw)
}

func jsonHandler(fn func(http.ResponseWriter, *http.Request) (any, int, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		data, code, err := fn(w, r)
		if err != nil {
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(data)
	}
}

func queryInt(r *http.Request, key string, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	var value int
	if _, err := fmt.Sscanf(raw, "%d", &value); err != nil || value <= 0 {
		return fallback
	}
	return value
}

func wsHandler(bus *EventBus, manager *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			http.Error(w, "websocket obrigatorio", http.StatusBadRequest)
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijack indisponivel", http.StatusInternalServerError)
			return
		}
		key := r.Header.Get("Sec-WebSocket-Key")
		if key == "" {
			http.Error(w, "Sec-WebSocket-Key ausente", http.StatusBadRequest)
			return
		}
		conn, rw, err := hijacker.Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		accept := websocketAccept(key)
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
		_, _ = rw.WriteString("Upgrade: websocket\r\n")
		_, _ = rw.WriteString("Connection: Upgrade\r\n")
		_, _ = rw.WriteString("Sec-WebSocket-Accept: " + accept + "\r\n\r\n")
		_ = rw.Flush()

		ch := bus.Subscribe()
		defer bus.Unsubscribe(ch)
		initial, _ := json.Marshal(map[string]any{"kind": "snapshot", "payload": manager.Snapshot()})
		_ = writeWebSocketText(conn, initial)
		for msg := range ch {
			if err := writeWebSocketText(conn, msg); err != nil {
				return
			}
		}
	}
}

func websocketAccept(key string) string {
	// #nosec G401 -- WebSocket opening handshake requires SHA-1 by RFC 6455; this is not used for password hashing or signatures.
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func writeWebSocketText(conn net.Conn, payload []byte) error {
	w := bufio.NewWriter(conn)
	header := []byte{0x81}
	n := len(payload)
	switch {
	case n < 126:
		header = append(header, byte(n))
	case n <= 65535:
		var length [2]byte
		binary.BigEndian.PutUint16(length[:], uint16(n))
		header = append(header, 126)
		header = append(header, length[:]...)
	default:
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(n))
		header = append(header, 127)
		header = append(header, length[:]...)
	}
	if _, err := w.Write(header); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	return w.Flush()
}
