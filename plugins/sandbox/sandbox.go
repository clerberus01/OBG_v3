package sandbox

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Policy struct {
	WorkDir         string   `json:"work_dir,omitempty"`
	AllowCommands   []string `json:"allow_commands,omitempty"`
	ApprovedScripts []string `json:"approved_scripts,omitempty"`
	AllowEnv        []string `json:"allow_env,omitempty"`
	MaxOutputBytes  int      `json:"max_output_bytes,omitempty"`
}

type Request struct {
	PluginID     string
	ManifestPath string
	Command      string
	Args         []string
	Env          map[string]string
	Policy       Policy
	Permissions  Policy
	ContractID   int64
	TaskID       int64
}

type Execution struct {
	Command        string
	Args           []string
	Env            []string
	WorkDir        string
	MaxOutputBytes int
}

type Result struct {
	Output   string
	Duration string
	WorkDir  string
}

func Run(ctx context.Context, req Request, stdin string) (Result, error) {
	execution, err := Prepare(req)
	if err != nil {
		return Result{}, err
	}
	start := time.Now()
	cmd := exec.CommandContext(ctx, execution.Command, execution.Args...) // #nosec G204 -- Prepare validates command, args, allowlist, permissions and workdir before execution.
	cmd.Dir = execution.WorkDir
	cmd.Env = execution.Env
	cmd.Stdin = strings.NewReader(stdin)
	output, err := cmd.CombinedOutput()
	result := Result{
		Output:   TrimOutput(string(output), execution.MaxOutputBytes),
		Duration: time.Since(start).String(),
		WorkDir:  execution.WorkDir,
	}
	return result, err
}

func Prepare(req Request) (Execution, error) {
	basePolicy := normalizePolicy(req.Policy)
	permissions := normalizePolicy(req.Permissions)
	if err := ValidatePolicy(basePolicy, req.Env); err != nil {
		return Execution{}, err
	}
	policy := basePolicy
	if !emptyPolicy(req.Permissions) {
		if err := ValidatePolicy(permissions, nil); err != nil {
			return Execution{}, err
		}
		policy = applyPermissionLimits(policy, permissions)
	}
	policy.WorkDir = scopedWorkDir(policy.WorkDir, req.ContractID, req.TaskID)
	for _, arg := range req.Args {
		if err := ValidateProcessArg(arg); err != nil {
			return Execution{}, err
		}
	}
	if !CommandAllowed(req.Command, req.Args, basePolicy, projectRoot(req.ManifestPath)) {
		return Execution{}, fmt.Errorf("sandbox bloqueou command: %s", req.Command)
	}
	if !emptyPolicy(req.Permissions) && !PermissionAllows(req.Command, req.Args, permissions, projectRoot(req.ManifestPath)) {
		return Execution{}, fmt.Errorf("sandbox bloqueou command por permissao da tarefa/contrato: %s", req.Command)
	}
	workDir, err := WorkDir(req.ManifestPath, req.PluginID, policy)
	if err != nil {
		return Execution{}, err
	}
	env, err := Env(req.Env, policy)
	if err != nil {
		return Execution{}, err
	}
	return Execution{
		Command:        req.Command,
		Args:           req.Args,
		Env:            env,
		WorkDir:        workDir,
		MaxOutputBytes: policy.MaxOutputBytes,
	}, nil
}

func ValidatePolicy(policy Policy, env map[string]string) error {
	policy = normalizePolicy(policy)
	if policy.MaxOutputBytes < 1 || policy.MaxOutputBytes > 65536 {
		return errors.New("sandbox.max_output_bytes deve ficar entre 1 e 65536")
	}
	for _, command := range policy.AllowCommands {
		command = strings.TrimSpace(command)
		if command == "" {
			return errors.New("sandbox.allow_commands contem item vazio")
		}
		if strings.ContainsAny(command, "&|;<>\n\r") {
			return errors.New("sandbox.allow_commands deve conter executaveis diretos")
		}
	}
	for _, script := range policy.ApprovedScripts {
		if err := validateApprovedScript(script); err != nil {
			return err
		}
	}
	for _, key := range policy.AllowEnv {
		if !ValidEnvKey(key) {
			return fmt.Errorf("sandbox.allow_env invalido: %s", key)
		}
	}
	for key := range env {
		if !ValidEnvKey(key) {
			return fmt.Errorf("env invalido: %s", key)
		}
	}
	if policy.WorkDir != "" {
		if filepath.IsAbs(policy.WorkDir) {
			return errors.New("sandbox.work_dir deve ser relativo")
		}
		clean := filepath.Clean(policy.WorkDir)
		if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
			return errors.New("sandbox.work_dir nao pode sair do diretorio controlado")
		}
	}
	return nil
}

func SupportedTransport(transport string) bool {
	switch transport {
	case "stdio-json", "local-service", "web-service":
		return true
	default:
		return false
	}
}

func ValidateServiceEndpoint(transport, endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return errors.New("plugin de servico exige endpoint")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("endpoint invalido: %s", endpoint)
	}
	switch transport {
	case "local-service":
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return errors.New("local-service exige endpoint http/https")
		}
		host := parsed.Hostname()
		if !IsLoopbackHost(host) {
			return errors.New("local-service deve apontar para localhost/loopback")
		}
	case "web-service":
		if parsed.Scheme != "https" {
			return errors.New("web-service exige endpoint https")
		}
		if IsLoopbackHost(parsed.Hostname()) {
			return errors.New("web-service nao pode apontar para localhost/loopback")
		}
	default:
		return fmt.Errorf("transport nao suportado: %s", transport)
	}
	return nil
}

func ValidateHeaders(headers map[string]string) error {
	for key, value := range headers {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "\x00\r\n:") {
			return fmt.Errorf("header invalido: %s", key)
		}
		if strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("valor de header invalido: %s", key)
		}
	}
	return nil
}

func IsLoopbackHost(host string) bool {
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func WorkDir(manifestPath, pluginID string, policy Policy) (string, error) {
	if strings.TrimSpace(manifestPath) == "" {
		return "", errors.New("sandbox exige path do manifest")
	}
	baseDir := filepath.Dir(manifestPath)
	root := filepath.Join(baseDir, ".sandbox", pluginID)
	workDir := root
	if policy.WorkDir != "" {
		clean := filepath.Clean(policy.WorkDir)
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
			return "", errors.New("sandbox.work_dir invalido")
		}
		workDir = filepath.Join(root, clean)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return "", err
	}
	if absWorkDir != absRoot && !strings.HasPrefix(absWorkDir, absRoot+string(os.PathSeparator)) {
		return "", errors.New("sandbox.work_dir fora do diretorio controlado")
	}
	if err := os.MkdirAll(absWorkDir, 0700); err != nil {
		return "", err
	}
	return absWorkDir, nil
}

func Env(pluginEnv map[string]string, policy Policy) ([]string, error) {
	env := baseEnv()
	for key, value := range pluginEnv {
		if !envAllowed(key, policy.AllowEnv) {
			return nil, fmt.Errorf("sandbox bloqueou env %s", key)
		}
		env = UpsertEnv(env, key, value)
	}
	return env, nil
}

func CommandAllowed(command string, args []string, policy Policy, root string) bool {
	if dangerousCommand(command) {
		return false
	}
	if goCommandAllowed(command, args) || gofmtCommandAllowed(command) {
		return true
	}
	if approvedScriptAllowed(command, args, policy, root) {
		return true
	}
	if controlledCommandAllowed(command, args, policy.AllowCommands) {
		return true
	}
	return commandAllowed(command, policy.AllowCommands)
}

func PermissionAllows(command string, args []string, permissions Policy, root string) bool {
	if dangerousCommand(command) {
		return false
	}
	if len(permissions.AllowCommands) == 0 && len(permissions.ApprovedScripts) == 0 {
		return true
	}
	if controlledCommandAllowed(command, args, permissions.AllowCommands) {
		return true
	}
	if allowedCommandPattern(command, args, permissions.AllowCommands) {
		return true
	}
	return approvedScriptAllowed(command, args, permissions, root)
}

func commandAllowed(command string, allowed []string) bool {
	command = cleanCommand(command)
	if restrictedDirectCommand(command) {
		return false
	}
	commandBase := filepath.Base(command)
	for _, item := range allowed {
		item = cleanCommand(item)
		if item == "" {
			continue
		}
		itemBase := filepath.Base(item)
		if equalPath(command, item) || equalPath(commandBase, item) || equalPath(commandBase, itemBase) {
			return true
		}
	}
	return false
}

func restrictedDirectCommand(command string) bool {
	base := strings.ToLower(filepath.Base(cleanCommand(command)))
	base = strings.TrimSuffix(base, ".exe")
	switch base {
	case "curl", "docker", "git", "go", "gofmt", "podman", "powershell", "pwsh":
		return true
	default:
		return false
	}
}

func dangerousCommand(command string) bool {
	base := strings.ToLower(filepath.Base(cleanCommand(command)))
	base = strings.TrimSuffix(base, ".exe")
	dangerous := map[string]struct{}{
		"bash":     {},
		"cmd":      {},
		"del":      {},
		"erase":    {},
		"format":   {},
		"mkfs":     {},
		"mount":    {},
		"mv":       {},
		"rd":       {},
		"reg":      {},
		"ren":      {},
		"rename":   {},
		"rm":       {},
		"rmdir":    {},
		"robocopy": {},
		"rsync":    {},
		"scp":      {},
		"sh":       {},
		"shutdown": {},
		"ssh":      {},
		"takeown":  {},
		"taskkill": {},
		"wsl":      {},
		"wslhost":  {},
	}
	_, ok := dangerous[base]
	return ok
}

func goCommandAllowed(command string, args []string) bool {
	base := strings.ToLower(filepath.Base(cleanCommand(command)))
	if base != "go" && base != "go.exe" {
		return false
	}
	if len(args) == 0 {
		return false
	}
	return args[0] == "test" || args[0] == "build"
}

func gofmtCommandAllowed(command string) bool {
	base := strings.ToLower(filepath.Base(cleanCommand(command)))
	return base == "gofmt" || base == "gofmt.exe"
}

func controlledCommandAllowed(command string, args []string, allowed []string) bool {
	if !controlledCommandPatternAllowed(command, args, allowed) {
		return false
	}
	base := strings.ToLower(filepath.Base(cleanCommand(command)))
	base = strings.TrimSuffix(base, ".exe")
	switch base {
	case "git":
		return controlledGitAllowed(args)
	case "docker", "podman":
		return controlledContainerAllowed(args)
	case "curl":
		return controlledCurlAllowed(args)
	default:
		return false
	}
}

func controlledCommandPatternAllowed(command string, args []string, allowed []string) bool {
	actual := commandTokens(command, args)
	for _, item := range allowed {
		expected := strings.Fields(strings.TrimSpace(item))
		if len(expected) < 2 || len(expected) > len(actual) {
			continue
		}
		matches := true
		for i := range expected {
			if !commandTokenEqual(expected[i], actual[i], i == 0) {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func controlledGitAllowed(args []string) bool {
	if len(args) == 0 || hasBlockedOption(args, "-c", "--exec-path", "--upload-pack", "--receive-pack") {
		return false
	}
	command := args[0]
	if strings.HasPrefix(command, "-") {
		return false
	}
	allowed := map[string]struct{}{
		"branch":    {},
		"diff":      {},
		"log":       {},
		"ls-files":  {},
		"remote":    {},
		"rev-parse": {},
		"show":      {},
		"status":    {},
		"version":   {},
	}
	if _, ok := allowed[command]; !ok {
		return false
	}
	if command == "remote" {
		return len(args) == 1 || (len(args) == 2 && args[1] == "-v")
	}
	return true
}

func controlledContainerAllowed(args []string) bool {
	if len(args) == 0 || hasBlockedOption(args, "-H", "--host", "--context") {
		return false
	}
	if args[0] == "compose" {
		if len(args) < 2 {
			return false
		}
		switch args[1] {
		case "config", "logs", "ps":
			return true
		default:
			return false
		}
	}
	allowed := map[string]struct{}{
		"images":  {},
		"info":    {},
		"inspect": {},
		"logs":    {},
		"ps":      {},
		"version": {},
	}
	_, ok := allowed[args[0]]
	return ok
}

func controlledCurlAllowed(args []string) bool {
	if len(args) == 0 {
		return false
	}
	method := "GET"
	hasURL := false
	blocked := map[string]struct{}{
		"-d": {}, "--data": {}, "--data-raw": {}, "--data-binary": {}, "--form": {}, "-F": {},
		"-o": {}, "-O": {}, "--output": {}, "--remote-name": {}, "-T": {}, "--upload-file": {},
		"-H": {}, "--header": {}, "-K": {}, "--config": {}, "--netrc": {}, "--proxy": {}, "-x": {},
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if curlBlockedArg(arg, blocked) {
			return false
		}
		switch arg {
		case "-X", "--request":
			if i+1 >= len(args) {
				return false
			}
			method = strings.ToUpper(args[i+1])
			i++
			continue
		case "-I", "--head":
			method = "HEAD"
			continue
		}
		if strings.HasPrefix(arg, "--request=") {
			method = strings.ToUpper(strings.TrimPrefix(arg, "--request="))
			continue
		}
		if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://") {
			if err := ValidateCurlURL(arg); err != nil {
				return false
			}
			hasURL = true
		}
	}
	return hasURL && (method == "GET" || method == "HEAD")
}

func curlBlockedArg(arg string, blocked map[string]struct{}) bool {
	if _, ok := blocked[arg]; ok {
		return true
	}
	for item := range blocked {
		if strings.HasPrefix(item, "--") && strings.HasPrefix(arg, item+"=") {
			return true
		}
	}
	for _, prefix := range []string{"-d", "-F", "-o", "-T", "-H", "-x"} {
		if arg != prefix && strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}

func ValidateCurlURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("curl URL invalida: %s", raw)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("curl URL exige http/https: %s", raw)
	}
	return nil
}

func hasBlockedOption(args []string, blocked ...string) bool {
	for _, arg := range args {
		for _, item := range blocked {
			if arg == item || strings.HasPrefix(arg, item+"=") {
				return true
			}
		}
	}
	return false
}

func approvedScriptAllowed(command string, args []string, policy Policy, root string) bool {
	if len(policy.ApprovedScripts) == 0 {
		return false
	}
	if scriptCommandAllowed(command, policy.ApprovedScripts, root) {
		return true
	}
	if !isPowerShellCommand(command) {
		return false
	}
	script, ok := powerShellFileArg(args)
	if !ok {
		return false
	}
	return scriptApproved(script, policy.ApprovedScripts, root)
}

func scriptCommandAllowed(command string, approved []string, root string) bool {
	if isPowerShellCommand(command) {
		return false
	}
	return scriptApproved(command, approved, root)
}

func scriptApproved(script string, approved []string, root string) bool {
	scriptPath, ok := localScriptPath(script, root)
	if !ok {
		return false
	}
	for _, item := range approved {
		approvedPath, ok := localScriptPath(item, root)
		if ok && equalPath(scriptPath, approvedPath) {
			return true
		}
	}
	return false
}

func localScriptPath(script, root string) (string, bool) {
	script = strings.TrimSpace(script)
	if script == "" || strings.ContainsAny(script, "\x00\n\r") {
		return "", false
	}
	if filepath.IsAbs(script) {
		return "", false
	}
	clean := filepath.Clean(script)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	absScript, err := filepath.Abs(filepath.Join(absRoot, clean))
	if err != nil {
		return "", false
	}
	if absScript != absRoot && strings.HasPrefix(absScript, absRoot+string(os.PathSeparator)) {
		return absScript, true
	}
	return "", false
}

func isPowerShellCommand(command string) bool {
	base := strings.ToLower(filepath.Base(cleanCommand(command)))
	return base == "powershell" || base == "powershell.exe" || base == "pwsh" || base == "pwsh.exe"
}

func powerShellFileArg(args []string) (string, bool) {
	var script string
	for i := 0; i < len(args); i++ {
		if strings.EqualFold(args[i], "-File") {
			if i+1 >= len(args) {
				return "", false
			}
			script = args[i+1]
			i++
			continue
		}
		if strings.EqualFold(args[i], "-Command") || strings.EqualFold(args[i], "-EncodedCommand") {
			return "", false
		}
	}
	return script, script != ""
}

func projectRoot(manifestPath string) string {
	if strings.TrimSpace(manifestPath) == "" {
		wd, err := os.Getwd()
		if err == nil {
			return wd
		}
		return "."
	}
	dir := filepath.Dir(manifestPath)
	if strings.EqualFold(filepath.Base(dir), "plugins") {
		return filepath.Dir(dir)
	}
	return dir
}

func validateApprovedScript(script string) error {
	if _, ok := localScriptPath(script, "."); !ok {
		return fmt.Errorf("sandbox.approved_scripts invalido: %s", script)
	}
	return nil
}

func cleanCommand(command string) string {
	return filepath.Clean(strings.TrimSpace(command))
}

func baseEnv() []string {
	keys := []string{"PATH", "HOME", "TMPDIR", "LANG"}
	if runtime.GOOS == "windows" {
		keys = append(keys, "SystemRoot", "WINDIR", "COMSPEC", "PATHEXT", "TEMP", "TMP", "USERPROFILE")
	}
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		if value, ok := lookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func envAllowed(key string, allowed []string) bool {
	for _, item := range allowed {
		if strings.EqualFold(key, item) {
			return true
		}
	}
	return false
}

func applyPermissionLimits(base, permissions Policy) Policy {
	if len(permissions.AllowCommands) > 0 {
		base.AllowCommands = permissions.AllowCommands
	}
	if len(permissions.ApprovedScripts) > 0 {
		base.ApprovedScripts = intersectStrings(base.ApprovedScripts, permissions.ApprovedScripts)
	}
	if len(permissions.AllowEnv) > 0 {
		base.AllowEnv = intersectStrings(base.AllowEnv, permissions.AllowEnv)
	}
	if permissions.MaxOutputBytes > 0 && permissions.MaxOutputBytes < base.MaxOutputBytes {
		base.MaxOutputBytes = permissions.MaxOutputBytes
	}
	if strings.TrimSpace(permissions.WorkDir) != "" {
		base.WorkDir = filepath.Join(base.WorkDir, permissions.WorkDir)
	}
	return base
}

func allowedCommandPattern(command string, args []string, allowed []string) bool {
	if len(allowed) == 0 {
		return false
	}
	actual := commandTokens(command, args)
	for _, item := range allowed {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		expected := strings.Fields(item)
		if len(expected) == 0 {
			continue
		}
		if len(expected) == 1 && commandAllowed(command, []string{expected[0]}) {
			return true
		}
		if len(expected) > len(actual) {
			continue
		}
		matches := true
		for i := range expected {
			if !commandTokenEqual(expected[i], actual[i], i == 0) {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func commandTokens(command string, args []string) []string {
	out := []string{filepath.Base(cleanCommand(command))}
	for _, arg := range args {
		out = append(out, strings.TrimSpace(arg))
	}
	return out
}

func commandTokenEqual(expected, actual string, executable bool) bool {
	expected = cleanCommand(expected)
	actual = cleanCommand(actual)
	if executable {
		expected = strings.TrimSuffix(strings.ToLower(filepath.Base(expected)), ".exe")
		actual = strings.TrimSuffix(strings.ToLower(filepath.Base(actual)), ".exe")
		return expected == actual
	}
	return equalPath(expected, actual)
}

func scopedWorkDir(workDir string, contractID, taskID int64) string {
	var parts []string
	if strings.TrimSpace(workDir) != "" {
		parts = append(parts, workDir)
	}
	if contractID > 0 {
		parts = append(parts, fmt.Sprintf("contract-%d", contractID))
	}
	if taskID > 0 {
		parts = append(parts, fmt.Sprintf("task-%d", taskID))
	}
	if len(parts) == 0 {
		return ""
	}
	return filepath.Join(parts...)
}

func intersectStrings(base, permissions []string) []string {
	if len(base) == 0 || len(permissions) == 0 {
		return nil
	}
	var out []string
	for _, item := range base {
		for _, permission := range permissions {
			if strings.EqualFold(item, permission) {
				out = append(out, item)
				break
			}
		}
	}
	return out
}

func emptyPolicy(policy Policy) bool {
	return strings.TrimSpace(policy.WorkDir) == "" &&
		len(policy.AllowCommands) == 0 &&
		len(policy.ApprovedScripts) == 0 &&
		len(policy.AllowEnv) == 0 &&
		policy.MaxOutputBytes == 0
}

func lookupEnv(key string) (string, bool) {
	if runtime.GOOS != "windows" {
		return os.LookupEnv(key)
	}
	for _, item := range os.Environ() {
		envKey, value, ok := strings.Cut(item, "=")
		if ok && strings.EqualFold(envKey, key) {
			return value, true
		}
	}
	return "", false
}

func equalPath(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func ValidateProcessArg(arg string) error {
	if strings.ContainsAny(arg, "\x00\n\r") {
		return errors.New("args nao podem conter quebras de linha ou byte nulo")
	}
	return nil
}

func ValidEnvKey(key string) bool {
	if strings.TrimSpace(key) == "" || strings.Contains(key, "=") {
		return false
	}
	return !strings.ContainsAny(key, "\x00\n\r")
}

func UpsertEnv(env []string, key, value string) []string {
	prefix := strings.ToUpper(key) + "="
	for i, item := range env {
		if strings.HasPrefix(strings.ToUpper(item), prefix) {
			env[i] = key + "=" + value
			return env
		}
	}
	return append(env, key+"="+value)
}

func TrimOutput(output string, limit int) string {
	output = strings.TrimSpace(output)
	if len(output) <= limit {
		return output
	}
	return output[:limit] + "\n...saida truncada..."
}

func normalizePolicy(policy Policy) Policy {
	if policy.MaxOutputBytes <= 0 {
		policy.MaxOutputBytes = 16000
	}
	return policy
}
