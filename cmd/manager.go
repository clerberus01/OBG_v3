package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"omni-bot-go/database"
	"omni-bot-go/engine"
	"omni-bot-go/knowledge"
	"omni-bot-go/mcp"
	"omni-bot-go/plugins/sandbox"
)

type EventBus struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func NewEventBus() *EventBus {
	return &EventBus{clients: map[chan []byte]struct{}{}}
}

func (b *EventBus) Subscribe() chan []byte {
	ch := make(chan []byte, 16)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *EventBus) Unsubscribe(ch chan []byte) {
	b.mu.Lock()
	delete(b.clients, ch)
	close(ch)
	b.mu.Unlock()
}

func (b *EventBus) Publish(kind string, payload any) {
	raw, _ := json.Marshal(map[string]any{"kind": kind, "payload": payload, "at": time.Now()})
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- raw:
		default:
		}
	}
}

type Manager struct {
	store          *database.Store
	engine         *engine.Engine
	knowledgePacks *knowledge.Library
	pluginsDir     string
	bus            *EventBus
	workerQueues   map[int]chan database.Task
	planningMu     sync.Mutex
	ctx            context.Context
	cancel         context.CancelFunc
}

type jitKnowledgePackItem struct {
	Topic    string            `json:"topic"`
	Source   string            `json:"source,omitempty"`
	Summary  string            `json:"summary,omitempty"`
	Keywords []string          `json:"keywords,omitempty"`
	Rules    map[string]string `json:"rules,omitempty"`
	Score    int               `json:"score"`
}

type watchdogEvent struct {
	TaskID     int64  `json:"task_id"`
	ContractID int64  `json:"contract_id"`
	Title      string `json:"title"`
	Role       string `json:"role"`
	Attempt    int    `json:"attempt"`
	Limit      int    `json:"limit"`
	Reason     string `json:"reason"`
	Status     string `json:"status"`
	Blocked    bool   `json:"blocked"`
}

type assertivenessMetrics struct {
	Approved            int               `json:"approved"`
	Redone              int               `json:"redone"`
	Blocked             int               `json:"blocked"`
	AverageSeconds      float64           `json:"average_seconds"`
	SuccessRateByRole   []successRateItem `json:"success_rate_by_role"`
	SuccessRateByDomain []successRateItem `json:"success_rate_by_domain"`
}

type contractDashboardSummary struct {
	ContractID     int64     `json:"contract_id"`
	Hash           string    `json:"hash"`
	NorthStar      string    `json:"north_star"`
	Tasks          int       `json:"tasks"`
	Pending        int       `json:"pending"`
	Running        int       `json:"running"`
	Approved       int       `json:"approved"`
	Blocked        int       `json:"blocked"`
	ReadOnly       int       `json:"read_only"`
	Domains        []string  `json:"domains"`
	Roles          []string  `json:"roles"`
	FactoryBatchID string    `json:"factory_batch_id,omitempty"`
	FactoryIndex   int       `json:"factory_index,omitempty"`
	FactoryTotal   int       `json:"factory_total,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type taskDependencyEdge struct {
	ContractID    int64  `json:"contract_id"`
	FromTaskID    int64  `json:"from_task_id"`
	ToTaskID      int64  `json:"to_task_id"`
	ToStatus      string `json:"to_status"`
	FromStatus    string `json:"from_status,omitempty"`
	Blocked       bool   `json:"blocked"`
	CrossContract bool   `json:"cross_contract"`
}

type watchdogSnapshotEvent struct {
	TaskID     int64  `json:"task_id"`
	ContractID int64  `json:"contract_id"`
	Title      string `json:"title"`
	Role       string `json:"role"`
	Attempt    int    `json:"attempt"`
	Limit      int    `json:"limit"`
	Reason     string `json:"reason"`
	Status     string `json:"status"`
	Blocked    bool   `json:"blocked"`
	FailedAt   string `json:"failed_at,omitempty"`
}

type factorySeriesSummary struct {
	BatchID      string `json:"batch_id"`
	Mode         string `json:"mode"`
	Contracts    int    `json:"contracts"`
	Tasks        int    `json:"tasks"`
	Approved     int    `json:"approved"`
	Blocked      int    `json:"blocked"`
	CurrentIndex int    `json:"current_index"`
	Total        int    `json:"total"`
}

type successRateItem struct {
	Key            string  `json:"key"`
	Approved       int     `json:"approved"`
	Failed         int     `json:"failed"`
	Total          int     `json:"total"`
	SuccessRate    float64 `json:"success_rate"`
	AverageSeconds float64 `json:"average_seconds"`
}

type ContractDraft struct {
	NorthStar    string `json:"north_star"`
	Constraints  string `json:"constraints"`
	Deliverables string `json:"deliverables"`
	Hash         string `json:"hash"`
}

type FactorySeriesRequest struct {
	Template     string   `json:"template"`
	Items        []string `json:"items"`
	Constraints  string   `json:"constraints"`
	Deliverables string   `json:"deliverables"`
	ForceNew     bool     `json:"force_new"`
}

type FactorySeriesResult struct {
	BatchID   string              `json:"batch_id"`
	Mode      string              `json:"mode"`
	Count     int                 `json:"count"`
	Contracts []database.Contract `json:"contracts"`
}

type FactoryBatchActionRequest struct {
	BatchID string `json:"batch_id"`
	Action  string `json:"action"`
	Index   int    `json:"index,omitempty"`
	Reason  string `json:"reason"`
}

type FactoryBatchExport struct {
	Kind        string                `json:"kind"`
	Version     string                `json:"version"`
	BatchID     string                `json:"batch_id"`
	GeneratedAt time.Time             `json:"generated_at"`
	ExportHash  string                `json:"export_hash"`
	Summary     FactoryExportSummary  `json:"summary"`
	Batch       database.FactoryBatch `json:"batch"`
	Items       []FactoryExportItem   `json:"items"`
	Logs        []database.LogEntry   `json:"logs"`
}

type FactoryExportSummary struct {
	Contracts int `json:"contracts"`
	Tasks     int `json:"tasks"`
	Artifacts int `json:"artifacts"`
	Handoffs  int `json:"handoffs"`
	Logs      int `json:"logs"`
}

type FactoryExportItem struct {
	Item      database.FactoryItem `json:"item"`
	Contract  database.Contract    `json:"contract"`
	Tasks     []database.Task      `json:"tasks"`
	Artifacts []database.Artifact  `json:"artifacts"`
	Handoffs  []database.Handoff   `json:"handoffs"`
}

func NewManager(store *database.Store, eng *engine.Engine, knowledgePacks *knowledge.Library, bus *EventBus) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	manager := &Manager{
		store:          store,
		engine:         eng,
		knowledgePacks: knowledgePacks,
		pluginsDir:     "plugins",
		bus:            bus,
		workerQueues:   newWorkerQueues(16),
		ctx:            ctx,
		cancel:         cancel,
	}
	manager.initializeWorkerRoles()
	manager.loadPersistedKnowledgePacks()
	return manager
}

func (m *Manager) initializeWorkerRoles() {
	for i := 1; i <= 4; i++ {
		_ = m.store.SetAgent(i, workerIdleRole(i), 0, database.StatusPending)
	}
}

func (m *Manager) Start() {
	for i := 1; i <= 4; i++ {
		go m.worker(i)
	}
	go m.managerLoop()
}

func (m *Manager) Stop() {
	m.cancel()
}

func (m *Manager) StartInterrogation(input string) (database.Interrogation, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return database.Interrogation{}, errors.New("pedido vazio")
	}
	questions := interrogationQuestions(input)
	northStar := "Entregar: " + input
	constraints := strings.Join([]string{
		"SQLite e a fonte de verdade.",
		"Tarefas aprovadas sao read-only.",
		"Saidas devem conter relatorio JSON auditavel.",
		"Falha tripla bloqueia a tarefa pelo watchdog.",
	}, " ")
	deliverables := "Contrato selado, checklist de tarefas, relatorios de estado JSON, artefatos aprovados e logs tecnicos."
	session, err := m.store.CreateInterrogation(input, questions, northStar, constraints, deliverables)
	if err != nil {
		return database.Interrogation{}, err
	}
	m.store.Log("INFO", "interrogatorio", fmt.Sprintf("sessao de balcao criada: %d", session.ID))
	m.bus.Publish("snapshot", m.Snapshot())
	return session, nil
}

func (m *Manager) FinalizeInterrogation(id int64, answers map[string]string) (database.Contract, error) {
	return m.FinalizeInterrogationWithDraft(id, answers, ContractDraft{})
}

func (m *Manager) PreviewInterrogationContract(id int64, answers map[string]string) (ContractDraft, error) {
	session, err := m.store.GetInterrogation(id)
	if err != nil {
		return ContractDraft{}, err
	}
	if session.Status == "Contrato Selado" {
		return ContractDraft{}, errors.New("interrogatorio ja selado")
	}
	answers = completeInterrogationAnswers(session, answers)
	northStar, constraints, deliverables := buildContractFromAnswers(session, answers)
	return buildContractDraft(northStar, constraints, deliverables), nil
}

func (m *Manager) FinalizeInterrogationWithDraft(id int64, answers map[string]string, draft ContractDraft) (database.Contract, error) {
	session, err := m.store.GetInterrogation(id)
	if err != nil {
		return database.Contract{}, err
	}
	if session.Status == "Contrato Selado" {
		return database.Contract{}, errors.New("interrogatorio ja selado")
	}
	answers = completeInterrogationAnswers(session, answers)
	northStar, constraints, deliverables := buildContractFromAnswers(session, answers)
	if strings.TrimSpace(draft.NorthStar) != "" || strings.TrimSpace(draft.Constraints) != "" || strings.TrimSpace(draft.Deliverables) != "" {
		northStar = strings.TrimSpace(draft.NorthStar)
		constraints = strings.TrimSpace(draft.Constraints)
		deliverables = strings.TrimSpace(draft.Deliverables)
	}
	if strings.TrimSpace(northStar) == "" || strings.TrimSpace(constraints) == "" || strings.TrimSpace(deliverables) == "" {
		return database.Contract{}, errors.New("contrato revisado incompleto")
	}
	contract, err := m.createContractWithTasks(northStar, constraints, deliverables)
	if err != nil {
		return database.Contract{}, err
	}
	if _, err := m.store.SealInterrogation(session.ID, contract.ID, answers, northStar, constraints, deliverables); err != nil {
		return database.Contract{}, err
	}
	m.store.Log("INFO", "manager", "contrato selado apos interrogatorio: "+contract.Hash)
	m.bus.Publish("snapshot", m.Snapshot())
	return contract, nil
}

func (m *Manager) CreateProject(input string) (database.Contract, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return database.Contract{}, errors.New("pedido vazio")
	}
	return m.createContractWithTasks(input, "Sem sobrescrever tarefas aprovadas; validar sintaxe; usar SQLite como fonte de verdade.", "Contrato, tarefas, relatorios JSON e logs tecnicos.")
}

func (m *Manager) CreateFactorySeries(req FactorySeriesRequest) (FactorySeriesResult, error) {
	req.Template = strings.TrimSpace(req.Template)
	if req.Template == "" {
		return FactorySeriesResult{}, errors.New("template da fabrica vazio")
	}
	items := normalizeFactoryItems(req.Items)
	if len(items) == 0 {
		return FactorySeriesResult{}, errors.New("fabrica sem itens")
	}
	if len(items) > 50 {
		return FactorySeriesResult{}, errors.New("fabrica limitada a 50 itens por lote")
	}
	batchID := factoryBatchID(req.Template, items)
	if req.ForceNew {
		batchID = factoryBatchIDWithSalt(req.Template, items, time.Now().UTC().Format(time.RFC3339Nano))
	}
	constraints := strings.TrimSpace(req.Constraints)
	if constraints == "" {
		constraints = "Executar em serie; preservar contrato imutavel por item; SQLite como fonte de verdade; sem modelos externos em producao."
	}
	deliverables := strings.TrimSpace(req.Deliverables)
	if deliverables == "" {
		deliverables = "Artefatos auditaveis por item, handoffs encadeados, logs tecnicos e crivo final por contrato."
	}

	m.planningMu.Lock()
	defer m.planningMu.Unlock()

	if !req.ForceNew {
		existing, err := m.store.GetFactoryBatch(batchID)
		if err == nil {
			result, err := m.factorySeriesResultFromBatch(existing)
			if err != nil {
				return FactorySeriesResult{}, err
			}
			m.store.Log("INFO", "factory-series", fmt.Sprintf("lote %s ja existia; retornando sem duplicar contratos", batchID))
			return result, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return FactorySeriesResult{}, err
		}
	}

	if _, err := m.store.UpsertFactoryBatch(batchID, req.Template, items, constraints, deliverables, "active"); err != nil {
		return FactorySeriesResult{}, err
	}

	result := FactorySeriesResult{BatchID: batchID, Mode: "factory-series", Count: len(items)}
	var previousFinalTaskID int64
	var previousContractID int64
	for index, item := range items {
		northStar := renderFactoryTemplate(req.Template, item, index+1, len(items), batchID)
		itemConstraints := fmt.Sprintf("%s Lote factory-series %s; item %d/%d; item=%s.", constraints, batchID, index+1, len(items), item)
		contract, err := m.store.CreateContract(northStar, itemConstraints, deliverables)
		if err != nil {
			return FactorySeriesResult{}, err
		}
		if _, err := m.store.UpsertFactoryItem(batchID, index+1, item, contract.ID, "created"); err != nil {
			return FactorySeriesResult{}, err
		}
		factoryPayload := map[string]any{
			"batch_id":             batchID,
			"mode":                 "factory-series",
			"item":                 item,
			"index":                index + 1,
			"total":                len(items),
			"previous_contract_id": previousContractID,
		}
		var baseDeps []int64
		if previousFinalTaskID > 0 {
			baseDeps = []int64{previousFinalTaskID}
		}
		created, err := m.createTasksFromSymbolicPlanWithContext(contract, baseDeps, map[string]any{
			"source":  "factory_series",
			"factory": factoryPayload,
		})
		if err != nil {
			return FactorySeriesResult{}, err
		}
		if len(created) > 0 {
			previousFinalTaskID = created[len(created)-1].ID
		}
		previousContractID = contract.ID
		result.Contracts = append(result.Contracts, contract)
	}
	m.store.Log("INFO", "factory-series", fmt.Sprintf("lote %s criado com %d contrato(s)", batchID, len(result.Contracts)))
	m.bus.Publish("snapshot", m.Snapshot())
	return result, nil
}

func (m *Manager) createContractWithTasks(northStar, constraints, deliverables string) (database.Contract, error) {
	m.planningMu.Lock()
	defer m.planningMu.Unlock()

	contract, err := m.store.CreateContract(northStar, constraints, deliverables)
	if err != nil {
		return database.Contract{}, err
	}
	if _, err := m.createTasksFromSymbolicPlanWithContext(contract, nil, nil); err != nil {
		return database.Contract{}, err
	}
	m.store.Log("INFO", "manager", "contrato criado: "+contract.Hash)
	m.bus.Publish("snapshot", m.Snapshot())
	return contract, nil
}

func (m *Manager) createTasksFromSymbolicPlan(contract database.Contract) error {
	_, err := m.createTasksFromSymbolicPlanWithContext(contract, nil, nil)
	return err
}

func (m *Manager) createTasksFromSymbolicPlanWithContext(contract database.Contract, firstTaskDeps []int64, extraPayload map[string]any) ([]database.Task, error) {
	plan, err := m.engine.Plan(contract)
	if err != nil {
		return nil, err
	}
	taskIDs := map[string]int64{}
	var createdTasks []database.Task
	for index, planned := range plan {
		var deps []int64
		for _, key := range planned.Dependencies {
			id, ok := taskIDs[key]
			if !ok {
				return nil, fmt.Errorf("tarefa planejada %q depende de %q antes da declaracao", planned.ID, key)
			}
			deps = append(deps, id)
		}
		if index == 0 && len(firstTaskDeps) > 0 {
			deps = append(deps, firstTaskDeps...)
		}
		payload := map[string]any{
			"source": "symbolic_planner",
			"domain": planned.Domain,
			"key":    planned.ID,
		}
		for key, value := range extraPayload {
			payload[key] = value
		}
		if len(planned.Payload) > 0 {
			var plannedPayload map[string]any
			if err := json.Unmarshal(planned.Payload, &plannedPayload); err == nil {
				for key, value := range plannedPayload {
					payload[key] = value
				}
			}
		}
		created, err := m.store.AddTaskWithPayload(contract.ID, planned.Description, planned.Role, deps, payload)
		if err != nil {
			return nil, err
		}
		createdTasks = append(createdTasks, created)
		key := planned.ID
		if key == "" {
			key = fmt.Sprintf("task_%d", index+1)
		}
		taskIDs[key] = created.ID
	}
	return createdTasks, nil
}

func normalizeFactoryItems(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func factoryBatchID(template string, items []string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(template) + "\n" + strings.Join(items, "\n")))
	return hex.EncodeToString(sum[:])[:12]
}

func factoryBatchIDWithSalt(template string, items []string, salt string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(template) + "\n" + strings.Join(items, "\n") + "\n" + strings.TrimSpace(salt)))
	return hex.EncodeToString(sum[:])[:12]
}

func (m *Manager) factorySeriesResultFromBatch(batch database.FactoryBatch) (FactorySeriesResult, error) {
	items, err := m.store.ListFactoryItems(batch.BatchID)
	if err != nil {
		return FactorySeriesResult{}, err
	}
	result := FactorySeriesResult{BatchID: batch.BatchID, Mode: "factory-series", Count: batch.Total}
	for _, item := range items {
		if item.ContractID == 0 {
			continue
		}
		contract, err := m.store.GetContract(item.ContractID)
		if err != nil {
			return FactorySeriesResult{}, err
		}
		result.Contracts = append(result.Contracts, contract)
	}
	return result, nil
}

func factoryExportLogs(logs []database.LogEntry, batchID string, contractIDs map[int64]struct{}) []database.LogEntry {
	var out []database.LogEntry
	for _, item := range logs {
		if strings.Contains(item.Message, batchID) {
			out = append(out, item)
			continue
		}
		for contractID := range contractIDs {
			if strings.Contains(item.Message, fmt.Sprintf("contrato %d", contractID)) || strings.Contains(item.Message, fmt.Sprintf("contract=%d", contractID)) {
				out = append(out, item)
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func factoryExportHash(export FactoryBatchExport) string {
	export.ExportHash = ""
	raw, _ := json.Marshal(export)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func renderFactoryTemplate(template, item string, index, total int, batchID string) string {
	out := strings.TrimSpace(template)
	replacer := strings.NewReplacer(
		"{{item}}", item,
		"{{index}}", fmt.Sprint(index),
		"{{total}}", fmt.Sprint(total),
		"{{batch}}", batchID,
	)
	out = replacer.Replace(out)
	if !strings.Contains(out, item) {
		out = fmt.Sprintf("%s: %s", out, item)
	}
	return out
}

func (m *Manager) Panic(reason string) error {
	if strings.TrimSpace(reason) == "" {
		reason = "interrupcao manual sem motivo informado"
	}
	if err := m.store.CancelRunning(reason); err != nil {
		return err
	}
	if err := m.store.SetBlocked(true, reason); err != nil {
		return err
	}
	m.bus.Publish("panic", map[string]string{"reason": reason})
	return nil
}

func (m *Manager) Resume() error {
	if err := m.store.SetBlocked(false, ""); err != nil {
		return err
	}
	m.bus.Publish("resume", m.Snapshot())
	return nil
}

func (m *Manager) CancelTask(taskID int64, reason string) error {
	if strings.TrimSpace(reason) == "" {
		reason = "parada manual da tarefa"
	}
	if err := m.store.CancelTask(taskID, reason); err != nil {
		return err
	}
	m.store.Log("WARN", "task-control", fmt.Sprintf("tarefa %d cancelada: %s", taskID, reason))
	m.bus.Publish("snapshot", m.Snapshot())
	return nil
}

func (m *Manager) RetryTask(taskID int64, reason string) error {
	if strings.TrimSpace(reason) == "" {
		reason = "refacao manual solicitada"
	}
	if err := m.store.RetryTask(taskID, reason); err != nil {
		return err
	}
	m.store.Log("INFO", "task-control", fmt.Sprintf("tarefa %d recolocada na fila: %s", taskID, reason))
	m.bus.Publish("snapshot", m.Snapshot())
	return nil
}

func (m *Manager) FactoryBatchAction(req FactoryBatchActionRequest) (map[string]any, error) {
	req.BatchID = strings.TrimSpace(req.BatchID)
	req.Action = strings.TrimSpace(strings.ToLower(req.Action))
	if req.BatchID == "" {
		return nil, errors.New("batch_id ausente")
	}
	if strings.TrimSpace(req.Reason) == "" {
		req.Reason = "acao operacional da fabrica"
	}
	switch req.Action {
	case "pause":
		batch, err := m.store.SetFactoryBatchStatus(req.BatchID, "paused")
		if err != nil {
			return nil, err
		}
		m.store.Log("WARN", "factory-series", fmt.Sprintf("lote %s pausado: %s", req.BatchID, req.Reason))
		m.bus.Publish("snapshot", m.Snapshot())
		return map[string]any{"batch": batch, "action": req.Action}, nil
	case "resume":
		batch, err := m.store.SetFactoryBatchStatus(req.BatchID, "active")
		if err != nil {
			return nil, err
		}
		m.store.Log("INFO", "factory-series", fmt.Sprintf("lote %s retomado: %s", req.BatchID, req.Reason))
		m.bus.Publish("snapshot", m.Snapshot())
		return map[string]any{"batch": batch, "action": req.Action}, nil
	case "cancel":
		batch, affected, err := m.cancelFactoryBatch(req.BatchID, req.Reason)
		if err != nil {
			return nil, err
		}
		m.bus.Publish("snapshot", m.Snapshot())
		return map[string]any{"batch": batch, "action": req.Action, "cancelled_tasks": affected}, nil
	case "skip":
		item, affected, err := m.skipFactoryItem(req.BatchID, req.Index, req.Reason)
		if err != nil {
			return nil, err
		}
		m.bus.Publish("snapshot", m.Snapshot())
		return map[string]any{"item": item, "action": req.Action, "cancelled_tasks": affected}, nil
	case "reprocess":
		item, task, err := m.reprocessFactoryItem(req.BatchID, req.Index, req.Reason)
		if err != nil {
			return nil, err
		}
		m.bus.Publish("snapshot", m.Snapshot())
		return map[string]any{"item": item, "reaudit_task": task, "action": req.Action}, nil
	default:
		return nil, errors.New("acao de fabrica desconhecida")
	}
}

func (m *Manager) ExportFactoryBatch(batchID string) (FactoryBatchExport, error) {
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		return FactoryBatchExport{}, errors.New("batch_id ausente")
	}
	batch, err := m.store.GetFactoryBatch(batchID)
	if err != nil {
		return FactoryBatchExport{}, err
	}
	items, err := m.store.ListFactoryItems(batchID)
	if err != nil {
		return FactoryBatchExport{}, err
	}
	tasks, err := m.store.ListTasks()
	if err != nil {
		return FactoryBatchExport{}, err
	}
	artifacts, err := m.store.ListArtifacts()
	if err != nil {
		return FactoryBatchExport{}, err
	}
	handoffs, err := m.store.ListHandoffs(1000)
	if err != nil {
		return FactoryBatchExport{}, err
	}
	logs, err := m.store.ListLogs(1000)
	if err != nil {
		return FactoryBatchExport{}, err
	}

	tasksByContract := map[int64][]database.Task{}
	for _, task := range tasks {
		tasksByContract[task.ContractID] = append(tasksByContract[task.ContractID], task)
	}
	artifactsByContract := map[int64][]database.Artifact{}
	for _, artifact := range artifacts {
		artifactsByContract[artifact.ContractID] = append(artifactsByContract[artifact.ContractID], artifact)
	}
	handoffsByContract := map[int64][]database.Handoff{}
	for _, handoff := range handoffs {
		handoffsByContract[handoff.ContractID] = append(handoffsByContract[handoff.ContractID], handoff)
	}

	export := FactoryBatchExport{
		Kind:        "factory-batch-export",
		Version:     "2.0.0",
		BatchID:     batch.BatchID,
		GeneratedAt: time.Now().UTC(),
		Batch:       batch,
	}
	contractIDs := map[int64]struct{}{}
	for _, item := range items {
		contract, err := m.store.GetContract(item.ContractID)
		if err != nil {
			return FactoryBatchExport{}, err
		}
		itemTasks := tasksByContract[item.ContractID]
		itemArtifacts := artifactsByContract[item.ContractID]
		itemHandoffs := handoffsByContract[item.ContractID]
		export.Items = append(export.Items, FactoryExportItem{
			Item:      item,
			Contract:  contract,
			Tasks:     itemTasks,
			Artifacts: itemArtifacts,
			Handoffs:  itemHandoffs,
		})
		contractIDs[item.ContractID] = struct{}{}
		export.Summary.Contracts++
		export.Summary.Tasks += len(itemTasks)
		export.Summary.Artifacts += len(itemArtifacts)
		export.Summary.Handoffs += len(itemHandoffs)
	}
	export.Logs = factoryExportLogs(logs, batchID, contractIDs)
	export.Summary.Logs = len(export.Logs)
	export.ExportHash = factoryExportHash(export)
	m.store.Log("INFO", "factory-series", fmt.Sprintf("lote %s exportado hash=%s", batchID, export.ExportHash))
	return export, nil
}

func (m *Manager) cancelFactoryBatch(batchID, reason string) (database.FactoryBatch, int64, error) {
	batch, err := m.store.SetFactoryBatchStatus(batchID, "cancelled")
	if err != nil {
		return database.FactoryBatch{}, 0, err
	}
	items, err := m.store.ListAllFactoryItems(1000)
	if err != nil {
		return database.FactoryBatch{}, 0, err
	}
	var affected int64
	for _, item := range items {
		if item.BatchID != batchID {
			continue
		}
		_, _ = m.store.SetFactoryItemStatus(batchID, item.Index, "cancelled")
		if item.ContractID == 0 {
			continue
		}
		count, err := m.store.CancelFactoryItemTasks(item.ContractID, reason)
		if err != nil {
			return database.FactoryBatch{}, 0, err
		}
		affected += count
	}
	m.store.Log("WARN", "factory-series", fmt.Sprintf("lote %s cancelado: %s", batchID, reason))
	return batch, affected, nil
}

func (m *Manager) skipFactoryItem(batchID string, index int, reason string) (database.FactoryItem, int64, error) {
	item, err := m.store.SetFactoryItemStatus(batchID, index, "skipped")
	if err != nil {
		return database.FactoryItem{}, 0, err
	}
	var affected int64
	if item.ContractID != 0 {
		affected, err = m.store.CancelFactoryItemTasks(item.ContractID, reason)
		if err != nil {
			return database.FactoryItem{}, 0, err
		}
	}
	m.store.Log("WARN", "factory-series", fmt.Sprintf("item %d do lote %s pulado: %s", index, batchID, reason))
	return item, affected, nil
}

func (m *Manager) reprocessFactoryItem(batchID string, index int, reason string) (database.FactoryItem, database.Task, error) {
	item, err := m.store.SetFactoryItemStatus(batchID, index, "reprocess_requested")
	if err != nil {
		return database.FactoryItem{}, database.Task{}, err
	}
	if item.ContractID == 0 {
		return database.FactoryItem{}, database.Task{}, errors.New("item da fabrica sem contrato para reprocessar")
	}
	task, err := m.ReauditContract(item.ContractID)
	if err != nil {
		return database.FactoryItem{}, database.Task{}, err
	}
	m.store.Log("INFO", "factory-series", fmt.Sprintf("item %d do lote %s marcado para reprocessamento: %s", index, batchID, reason))
	return item, task, nil
}

func (m *Manager) ReauditContract(contractID int64) (database.Task, error) {
	if _, err := m.store.GetContract(contractID); err != nil {
		return database.Task{}, err
	}
	task, err := m.store.AddAuditTask(contractID)
	if err != nil {
		return database.Task{}, err
	}
	m.store.Log("INFO", "task-control", fmt.Sprintf("reauditoria criada para contrato %d como tarefa %d", contractID, task.ID))
	m.bus.Publish("snapshot", m.Snapshot())
	return task, nil
}

func (m *Manager) Snapshot() map[string]any {
	contracts, _ := m.store.ListContracts()
	interrogations, _ := m.store.ListInterrogations()
	tasks, _ := m.store.ListTasks()
	artifacts, _ := m.store.ListArtifacts()
	handoffs, _ := m.store.ListHandoffs(80)
	plugins, _ := m.Plugins()
	pluginCalls, _ := m.store.ListPluginCalls(80)
	pluginCommandRegistry, _ := m.store.ListPluginCommandRegistry(120)
	pluginPermissionScopes, _ := m.store.ListPluginPermissionScopes(120)
	knowledgePacks, _ := m.knowledgePacks.Summaries(80)
	knowledgeCandidates, _ := m.store.ListKnowledgeCandidates("", 80)
	agents, _ := m.store.ListAgents()
	logs, _ := m.store.ListLogs(80)
	factoryBatches, _ := m.store.ListFactoryBatches(80)
	factoryItems, _ := m.store.ListAllFactoryItems(500)
	return map[string]any{
		"blocked":                  m.store.SystemBlocked(),
		"build":                    buildInfo(),
		"engine":                   m.engine.Info(),
		"interrogations":           interrogations,
		"contracts":                contracts,
		"contract_summaries":       buildContractDashboardSummaries(contracts, tasks),
		"tasks":                    tasks,
		"task_dependencies":        buildTaskDependencyEdges(tasks),
		"watchdog_events":          buildWatchdogSnapshotEvents(tasks),
		"factory_series":           buildFactorySeriesSummaries(tasks),
		"factory_batches":          factoryBatches,
		"factory_items":            factoryItems,
		"assertiveness":            buildAssertivenessMetrics(tasks),
		"artifacts":                artifacts,
		"handoffs":                 handoffs,
		"plugins":                  plugins.Plugins,
		"plugin_calls":             pluginCalls,
		"plugin_command_registry":  pluginCommandRegistry,
		"plugin_permission_scopes": pluginPermissionScopes,
		"knowledge_packs":          knowledgePacks,
		"knowledge_candidates":     knowledgeCandidates,
		"agents":                   agents,
		"logs":                     logs,
	}
}

func buildContractDashboardSummaries(contracts []database.Contract, tasks []database.Task) []contractDashboardSummary {
	summaries := make([]contractDashboardSummary, len(contracts))
	byContract := map[int64]*contractDashboardSummary{}
	domainsByContract := map[int64]map[string]struct{}{}
	rolesByContract := map[int64]map[string]struct{}{}
	for i, contract := range contracts {
		summaries[i] = contractDashboardSummary{
			ContractID: contract.ID,
			Hash:       contract.Hash,
			NorthStar:  contract.NorthStar,
			CreatedAt:  contract.CreatedAt,
			UpdatedAt:  contract.CreatedAt,
		}
		byContract[contract.ID] = &summaries[i]
		domainsByContract[contract.ID] = map[string]struct{}{}
		rolesByContract[contract.ID] = map[string]struct{}{}
	}
	for _, task := range tasks {
		item := byContract[task.ContractID]
		if item == nil {
			continue
		}
		item.Tasks++
		if task.ReadOnly {
			item.ReadOnly++
		}
		switch task.Status {
		case database.StatusApproved:
			item.Approved++
		case database.StatusRunning:
			item.Running++
		case database.StatusBlocked, database.StatusRejected, database.StatusCancelled:
			item.Blocked++
		default:
			item.Pending++
		}
		if task.UpdatedAt.After(item.UpdatedAt) {
			item.UpdatedAt = task.UpdatedAt
		}
		if domain := strings.TrimSpace(fmt.Sprint(task.Payload["domain"])); domain != "" && domain != "<nil>" {
			domainsByContract[task.ContractID][domain] = struct{}{}
		}
		if role := strings.TrimSpace(task.Role); role != "" {
			rolesByContract[task.ContractID][role] = struct{}{}
		}
		if factory, ok := task.Payload["factory"].(map[string]any); ok {
			if item.FactoryBatchID == "" {
				item.FactoryBatchID = strings.TrimSpace(fmt.Sprint(factory["batch_id"]))
				item.FactoryIndex = intFromFactoryValue(factory["index"])
				item.FactoryTotal = intFromFactoryValue(factory["total"])
			}
		}
	}
	for i := range summaries {
		summaries[i].Domains = sortedKeys(domainsByContract[summaries[i].ContractID])
		summaries[i].Roles = sortedKeys(rolesByContract[summaries[i].ContractID])
	}
	return summaries
}

func buildTaskDependencyEdges(tasks []database.Task) []taskDependencyEdge {
	byID := map[int64]database.Task{}
	for _, task := range tasks {
		byID[task.ID] = task
	}
	var edges []taskDependencyEdge
	for _, task := range tasks {
		for _, depID := range task.Dependencies {
			dep := byID[depID]
			edges = append(edges, taskDependencyEdge{
				ContractID:    task.ContractID,
				FromTaskID:    depID,
				ToTaskID:      task.ID,
				ToStatus:      task.Status,
				FromStatus:    dep.Status,
				Blocked:       task.Status == database.StatusPending && dep.Status != database.StatusApproved,
				CrossContract: dep.ID != 0 && dep.ContractID != task.ContractID,
			})
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].ToTaskID == edges[j].ToTaskID {
			return edges[i].FromTaskID < edges[j].FromTaskID
		}
		return edges[i].ToTaskID < edges[j].ToTaskID
	})
	return edges
}

func buildWatchdogSnapshotEvents(tasks []database.Task) []watchdogSnapshotEvent {
	var events []watchdogSnapshotEvent
	for _, task := range tasks {
		watchdog, _ := task.Payload["watchdog"].(map[string]any)
		if task.Attempts == 0 && len(watchdog) == 0 {
			continue
		}
		limit := intFromFactoryValue(watchdog["limit"])
		if limit == 0 {
			limit = 3
		}
		attempt := intFromFactoryValue(watchdog["attempt"])
		if attempt == 0 {
			attempt = task.Attempts
		}
		reason := strings.TrimSpace(fmt.Sprint(watchdog["reason"]))
		if reason == "" || reason == "<nil>" {
			reason = strings.TrimSpace(fmt.Sprint(task.Payload["reason"]))
		}
		if reason == "" || reason == "<nil>" {
			reason = strings.TrimSpace(fmt.Sprint(task.Payload["retry_reason"]))
		}
		if reason == "" || reason == "<nil>" {
			reason = "falha registrada pelo watchdog"
		}
		events = append(events, watchdogSnapshotEvent{
			TaskID:     task.ID,
			ContractID: task.ContractID,
			Title:      task.Title,
			Role:       task.Role,
			Attempt:    attempt,
			Limit:      limit,
			Reason:     reason,
			Status:     task.Status,
			Blocked:    task.Status == database.StatusBlocked || boolFromMapValue(watchdog["blocked"]),
			FailedAt:   strings.TrimSpace(fmt.Sprint(watchdog["failed_at"])),
		})
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].Blocked != events[j].Blocked {
			return events[i].Blocked
		}
		if events[i].Attempt == events[j].Attempt {
			return events[i].TaskID > events[j].TaskID
		}
		return events[i].Attempt > events[j].Attempt
	})
	return events
}

func buildFactorySeriesSummaries(tasks []database.Task) []factorySeriesSummary {
	batches := map[string]*factorySeriesSummary{}
	contractSeen := map[string]map[int64]struct{}{}
	for _, task := range tasks {
		factory, ok := task.Payload["factory"].(map[string]any)
		if !ok {
			continue
		}
		batchID := strings.TrimSpace(fmt.Sprint(factory["batch_id"]))
		if batchID == "" {
			continue
		}
		item := batches[batchID]
		if item == nil {
			item = &factorySeriesSummary{BatchID: batchID, Mode: "factory-series"}
			batches[batchID] = item
			contractSeen[batchID] = map[int64]struct{}{}
		}
		item.Tasks++
		if _, ok := contractSeen[batchID][task.ContractID]; !ok {
			contractSeen[batchID][task.ContractID] = struct{}{}
			item.Contracts++
		}
		if task.Status == database.StatusApproved {
			item.Approved++
		}
		if task.Status == database.StatusBlocked {
			item.Blocked++
		}
		if idx := intFromFactoryValue(factory["index"]); idx > 0 && task.Status != database.StatusApproved {
			if item.CurrentIndex == 0 || idx < item.CurrentIndex {
				item.CurrentIndex = idx
			}
		}
		if total := intFromFactoryValue(factory["total"]); total > item.Total {
			item.Total = total
		}
	}
	out := make([]factorySeriesSummary, 0, len(batches))
	for _, item := range batches {
		if item.CurrentIndex == 0 && item.Total > 0 {
			item.CurrentIndex = item.Total
		}
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].BatchID < out[j].BatchID
	})
	return out
}

func intFromFactoryValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		var out int
		_, _ = fmt.Sscanf(fmt.Sprint(value), "%d", &out)
		return out
	}
}

func boolFromMapValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func sortedKeys(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (m *Manager) Plugins() (mcp.Registry, error) {
	registry, err := mcp.LoadRegistry(m.pluginsDir)
	if err != nil {
		return registry, err
	}
	_ = m.store.UpsertPluginCommandRegistry(pluginCommandRegistrations(registry))
	return registry, nil
}

func (m *Manager) PluginPermissionScopes(limit int) ([]database.PluginPermissionScope, error) {
	return m.store.ListPluginPermissionScopes(limit)
}

func (m *Manager) UpsertPluginPermissionScope(scope database.PluginPermissionScope) (database.PluginPermissionScope, error) {
	if scope.Scope == "contract" {
		if _, err := m.store.GetContract(scope.ContractID); err != nil {
			return database.PluginPermissionScope{}, fmt.Errorf("contrato da permissao nao encontrado: %w", err)
		}
	}
	if scope.Scope == "task" {
		task, err := m.store.GetTask(scope.TaskID)
		if err != nil {
			return database.PluginPermissionScope{}, fmt.Errorf("tarefa da permissao nao encontrada: %w", err)
		}
		if scope.ContractID == 0 {
			scope.ContractID = task.ContractID
		}
		if scope.ContractID != task.ContractID {
			return database.PluginPermissionScope{}, fmt.Errorf("tarefa %d nao pertence ao contrato %d", task.ID, scope.ContractID)
		}
	}
	var policy sandbox.Policy
	if strings.TrimSpace(scope.Permissions) == "" {
		scope.Permissions = "{}"
	}
	if err := json.Unmarshal([]byte(scope.Permissions), &policy); err != nil {
		return database.PluginPermissionScope{}, fmt.Errorf("permissions invalido: %w", err)
	}
	if err := sandbox.ValidatePolicy(policy, nil); err != nil {
		return database.PluginPermissionScope{}, err
	}
	if err := m.store.UpsertPluginPermissionScope(scope); err != nil {
		return database.PluginPermissionScope{}, err
	}
	items, err := m.store.ListPluginPermissionScopes(200)
	if err != nil {
		return database.PluginPermissionScope{}, err
	}
	for _, item := range items {
		if item.Scope == scope.Scope && item.ContractID == scope.ContractID && item.TaskID == scope.TaskID && item.PluginID == nonEmptyScopeValue(scope.PluginID) && item.Tool == nonEmptyScopeValue(scope.Tool) {
			m.bus.Publish("snapshot", m.Snapshot())
			return item, nil
		}
	}
	m.bus.Publish("snapshot", m.Snapshot())
	return scope, nil
}

func nonEmptyScopeValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "*"
	}
	return value
}

func (m *Manager) CallPlugin(req mcp.CallRequest) (mcp.CallResult, error) {
	normalizedReq, err := m.normalizePluginCallScope(req)
	if err != nil {
		return mcp.CallResult{}, err
	}
	req = normalizedReq
	registry, err := m.Plugins()
	if err != nil {
		return mcp.CallResult{}, err
	}
	result, err := registry.Call(m.ctx, req)
	ok := err == nil
	errorMessage := ""
	if err != nil {
		errorMessage = err.Error()
	}
	storedResult := map[string]any{
		"ok":          ok,
		"output":      result.Output,
		"duration":    result.Duration,
		"sandboxed":   result.Sandboxed,
		"work_dir":    result.WorkDir,
		"transport":   result.Transport,
		"contract_id": result.ContractID,
		"task_id":     result.TaskID,
	}
	if err != nil {
		storedResult["error"] = errorMessage
		if result.Output == "" {
			storedResult["output"] = errorMessage
		}
	}
	rawResult, marshalErr := json.Marshal(storedResult)
	output := string(rawResult)
	if marshalErr != nil {
		output = result.Output
		if err != nil && output == "" {
			output = err.Error()
		}
	}
	_ = m.store.AddPluginCallRecord(database.PluginCallRecord{
		PluginID:   req.PluginID,
		Tool:       req.Tool,
		Transport:  result.Transport,
		ContractID: req.ContractID,
		TaskID:     req.TaskID,
		Input:      pluginCallInput(req),
		Output:     output,
		OK:         ok,
		Duration:   result.Duration,
		Sandboxed:  result.Sandboxed,
		WorkDir:    result.WorkDir,
		Error:      errorMessage,
	})
	if err != nil {
		m.store.Log("ERROR", "plugin", fmt.Sprintf("%s/%s: %s", req.PluginID, req.Tool, err.Error()))
		return result, err
	}
	m.store.Log("INFO", "plugin", fmt.Sprintf("%s/%s executado em %s", req.PluginID, req.Tool, result.Duration))
	m.bus.Publish("snapshot", m.Snapshot())
	return result, nil
}

func (m *Manager) normalizePluginCallScope(req mcp.CallRequest) (mcp.CallRequest, error) {
	if req.TaskID > 0 {
		task, err := m.store.GetTask(req.TaskID)
		if err != nil {
			return req, fmt.Errorf("tarefa da chamada de plugin nao encontrada: %w", err)
		}
		if task.ReadOnly {
			return req, errors.New("tarefa read-only nao pode executar plugin")
		}
		if req.ContractID > 0 && req.ContractID != task.ContractID {
			return req, fmt.Errorf("tarefa %d nao pertence ao contrato %d", task.ID, req.ContractID)
		}
		req.ContractID = task.ContractID
	}
	if req.ContractID > 0 {
		if _, err := m.store.GetContract(req.ContractID); err != nil {
			return req, fmt.Errorf("contrato da chamada de plugin nao encontrado: %w", err)
		}
	}
	req, err := m.applyStoredPluginPermissions(req)
	if err != nil {
		return req, err
	}
	return req, nil
}

func (m *Manager) applyStoredPluginPermissions(req mcp.CallRequest) (mcp.CallRequest, error) {
	if req.ContractID <= 0 && req.TaskID <= 0 {
		return req, nil
	}
	scope, ok, err := m.store.PluginPermissionsForCall(req.ContractID, req.TaskID, req.PluginID, req.Tool)
	if err != nil || !ok {
		return req, err
	}
	var stored mcp.SandboxPolicy
	if err := json.Unmarshal([]byte(scope.Permissions), &stored); err != nil {
		return req, fmt.Errorf("permissao armazenada invalida para %s/%s: %w", req.PluginID, req.Tool, err)
	}
	req.Permissions = combinePluginPermissions(req.Permissions, stored)
	return req, nil
}

func pluginCallInput(req mcp.CallRequest) map[string]any {
	return map[string]any{
		"input":       req.Input,
		"contract_id": req.ContractID,
		"task_id":     req.TaskID,
		"permissions": req.Permissions,
	}
}

func pluginCommandRegistrations(registry mcp.Registry) []database.PluginCommandRegistration {
	commands := registry.CommandRegistrations()
	out := make([]database.PluginCommandRegistration, 0, len(commands))
	for _, command := range commands {
		rawSandbox, _ := json.Marshal(command.Sandbox)
		out = append(out, database.PluginCommandRegistration{
			PluginID:     command.PluginID,
			Tool:         command.Tool,
			Kind:         command.Kind,
			Transport:    command.Transport,
			Target:       command.Target,
			Status:       command.Status,
			Enabled:      command.Enabled,
			ManifestPath: command.ManifestPath,
			Sandbox:      string(rawSandbox),
		})
	}
	return out
}

func combinePluginPermissions(left, right mcp.SandboxPolicy) mcp.SandboxPolicy {
	return mcp.SandboxPolicy{
		WorkDir:         combinePermissionWorkDir(left.WorkDir, right.WorkDir),
		AllowCommands:   restrictiveStrings(left.AllowCommands, right.AllowCommands),
		ApprovedScripts: restrictiveStrings(left.ApprovedScripts, right.ApprovedScripts),
		AllowEnv:        restrictiveStrings(left.AllowEnv, right.AllowEnv),
		MaxOutputBytes:  restrictiveMaxOutput(left.MaxOutputBytes, right.MaxOutputBytes),
	}
}

func combinePermissionWorkDir(left, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	return filepath.Join(left, right)
}

func restrictiveStrings(left, right []string) []string {
	if len(left) == 0 {
		return right
	}
	if len(right) == 0 {
		return left
	}
	var out []string
	for _, a := range left {
		for _, b := range right {
			if strings.EqualFold(a, b) {
				out = append(out, a)
				break
			}
		}
	}
	return out
}

func restrictiveMaxOutput(left, right int) int {
	if left <= 0 {
		return right
	}
	if right <= 0 || left < right {
		return left
	}
	return right
}

func (m *Manager) IngestKnowledgePack(topic, source, text, path string) (knowledge.RuleMap, error) {
	var item knowledge.RuleMap
	var err error
	if strings.TrimSpace(path) != "" {
		item, err = m.knowledgePacks.IngestFile(topic, path)
	} else {
		item, err = m.knowledgePacks.IngestText(topic, source, text)
	}
	if err != nil {
		return knowledge.RuleMap{}, err
	}
	m.engine.LoadKnowledgePack(knowledge.RuleMapToPack(item))
	m.store.Log("INFO", "knowledge_packs", fmt.Sprintf("knowledge pack JIT atualizado: %s", item.Topic))
	m.bus.Publish("snapshot", m.Snapshot())
	return item, nil
}

func (m *Manager) SearchKnowledgePacks(query string, limit int) ([]knowledge.SearchResult, error) {
	return m.knowledgePacks.Search(query, limit)
}

func (m *Manager) SearchKnowledgePacksWithOptions(options knowledge.SearchOptions) ([]knowledge.SearchResult, error) {
	return m.knowledgePacks.SearchWithOptions(options)
}

func (m *Manager) KnowledgePackSummaries(limit int) ([]knowledge.Summary, error) {
	return m.knowledgePacks.Summaries(limit)
}

func (m *Manager) KnowledgeCandidateSummaries(status string, limit int) ([]database.KnowledgeCandidate, error) {
	return m.store.ListKnowledgeCandidates(status, limit)
}

func (m *Manager) ApproveKnowledgeCandidate(id int64) (database.KnowledgeCandidate, error) {
	candidate, err := m.store.GetKnowledgeCandidate(id)
	if err != nil {
		return database.KnowledgeCandidate{}, err
	}
	if candidate.Status != "pending" {
		return database.KnowledgeCandidate{}, errors.New("regra candidata ja decidida")
	}
	domain := normalizeCandidateDomain(candidate.Domain)
	topic := "learned-" + domain
	pack := &knowledge.KnowledgePack{
		Domain:  topic,
		Version: "learned",
		Rules: []knowledge.Rule{
			{Pattern: candidate.Pattern, Action: candidate.Action, Template: candidate.Template},
		},
	}
	if existing, err := m.knowledgePacks.Load(topic); err == nil {
		pack = knowledge.RuleMapToPack(existing)
		if !packHasAction(pack, candidate.Action) {
			pack.Rules = append(pack.Rules, knowledge.Rule{Pattern: candidate.Pattern, Action: candidate.Action, Template: candidate.Template})
		}
	}
	if err := m.knowledgePacks.SavePermanentKnowledgePack(pack, "self-evolving"); err != nil {
		return database.KnowledgeCandidate{}, err
	}
	m.engine.LoadKnowledgePack(pack)
	approved, err := m.store.SetKnowledgeCandidateStatus(id, "approved")
	if err != nil {
		return database.KnowledgeCandidate{}, err
	}
	m.store.Log("INFO", "knowledge_packs", fmt.Sprintf("regra candidata %d aprovada em %s", id, topic))
	m.bus.Publish("snapshot", m.Snapshot())
	return approved, nil
}

func (m *Manager) RejectKnowledgeCandidate(id int64) (database.KnowledgeCandidate, error) {
	rejected, err := m.store.SetKnowledgeCandidateStatus(id, "rejected")
	if err != nil {
		return database.KnowledgeCandidate{}, err
	}
	m.store.Log("INFO", "knowledge_packs", fmt.Sprintf("regra candidata %d rejeitada", id))
	m.bus.Publish("snapshot", m.Snapshot())
	return rejected, nil
}

func (m *Manager) LoadDiskKnowledgePacks(dir string) int {
	pattern := filepath.Join(strings.TrimSpace(dir), "*.pack.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		m.store.Log("WARN", "knowledge_packs", "padrao invalido para packs em disco: "+err.Error())
		return 0
	}
	loaded := 0
	for _, path := range files {
		pack, err := knowledge.LoadFromFile(path)
		if err != nil {
			m.store.Log("WARN", "knowledge_packs", fmt.Sprintf("falha ao carregar pack %s: %s", path, err.Error()))
			continue
		}
		m.engine.LoadKnowledgePack(pack)
		if err := m.knowledgePacks.SaveKnowledgePack(pack, path); err != nil {
			m.store.Log("WARN", "knowledge_packs", fmt.Sprintf("pack em disco carregado mas nao persistido %s: %s", path, err.Error()))
		}
		loaded++
		m.store.Log("INFO", "knowledge_packs", fmt.Sprintf("pack em disco carregado: %s@%s", pack.Domain, pack.Version))
	}
	return loaded
}

func (m *Manager) loadPersistedKnowledgePacks() {
	items, err := m.knowledgePacks.List(500)
	if err != nil {
		return
	}
	for _, item := range items {
		m.engine.LoadKnowledgePack(knowledge.RuleMapToPack(item))
	}
}

func (m *Manager) applicationGoroutineBudget() int {
	return 1 + len(m.workerQueues)
}

func (m *Manager) managerLoop() {
	schedulerTicker := time.NewTicker(700 * time.Millisecond)
	janitorTicker := time.NewTicker(time.Hour)
	defer schedulerTicker.Stop()
	defer janitorTicker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-schedulerTicker.C:
			m.scheduleReadyTasks()
		case <-janitorTicker.C:
			m.purgeExpiredKnowledgePacks()
		}
	}
}

func (m *Manager) scheduleReadyTasks() {
	m.planningMu.Lock()
	defer m.planningMu.Unlock()

	if m.store.SystemBlocked() {
		return
	}
	tasks, err := m.store.ReadyTasks(4)
	if err != nil {
		m.store.Log("ERROR", "scheduler", err.Error())
		return
	}
	for _, task := range tasks {
		if m.taskFactoryBatchBlocked(task) {
			continue
		}
		if err := m.store.MarkRunning(task.ID); err != nil {
			continue
		}
		workerID := workerIDForTaskRole(task.Role)
		select {
		case m.workerQueues[workerID] <- task:
		default:
			m.watchdogFailure(task, fmt.Sprintf("fila do worker %d cheia para papel %s", workerID, task.Role))
		}
	}
	m.bus.Publish("snapshot", m.Snapshot())
}

func (m *Manager) taskFactoryBatchBlocked(task database.Task) bool {
	factory, ok := task.Payload["factory"].(map[string]any)
	if !ok {
		return false
	}
	batchID := strings.TrimSpace(fmt.Sprint(factory["batch_id"]))
	if batchID == "" {
		return false
	}
	batch, err := m.store.GetFactoryBatch(batchID)
	if err != nil {
		return false
	}
	return batch.Status == "paused" || batch.Status == "cancelled"
}

func (m *Manager) worker(workerID int) {
	queue := m.workerQueues[workerID]
	for {
		select {
		case <-m.ctx.Done():
			return
		case task := <-queue:
			_ = m.store.SetAgent(workerID, task.Role, task.ID, database.StatusRunning)
			result, err := m.execute(task)
			if err != nil {
				m.watchdogFailure(task, err.Error())
				_ = m.store.SetAgent(workerID, workerIdleRole(workerID), 0, database.StatusPending)
				m.bus.Publish("snapshot", m.Snapshot())
				continue
			}
			if err := m.store.ApproveTask(task.ID, result); err != nil {
				m.store.Log("ERROR", "auditor", err.Error())
			} else if err := m.createHandoffs(task, result); err != nil {
				m.store.Log("ERROR", "handoff", err.Error())
			} else if err := m.createKnowledgeCandidate(task, result); err != nil {
				m.store.Log("WARN", "knowledge_packs", err.Error())
			}
			_ = m.store.SetAgent(workerID, workerIdleRole(workerID), 0, database.StatusPending)
			m.bus.Publish("snapshot", m.Snapshot())
		}
	}
}

func (m *Manager) watchdogFailure(task database.Task, reason string) {
	if strings.TrimSpace(reason) == "" {
		reason = "falha sem motivo informado"
	}
	if err := m.store.RejectTask(task.ID, reason); err != nil {
		m.store.Log("ERROR", "watchdog", fmt.Sprintf("falha ao registrar watchdog task=%d: %s", task.ID, err.Error()))
		return
	}
	current, err := m.store.GetTask(task.ID)
	if err != nil {
		m.store.Log("ERROR", "watchdog", err.Error())
		return
	}
	event := watchdogEvent{
		TaskID:     current.ID,
		ContractID: current.ContractID,
		Title:      current.Title,
		Role:       current.Role,
		Attempt:    current.Attempts,
		Limit:      3,
		Reason:     reason,
		Status:     current.Status,
		Blocked:    current.Status == database.StatusBlocked,
	}
	level := "WARN"
	if event.Blocked {
		level = "ERROR"
	}
	m.store.Log(level, "watchdog", fmt.Sprintf("task=%d tentativa=%d/3 status=%s motivo=%s", event.TaskID, event.Attempt, event.Status, event.Reason))
	m.bus.Publish("watchdog", event)
}

func newWorkerQueues(size int) map[int]chan database.Task {
	queues := map[int]chan database.Task{}
	for i := 1; i <= 4; i++ {
		queues[i] = make(chan database.Task, size)
	}
	return queues
}

func workerIDForTaskRole(role string) int {
	normalized := strings.ToLower(role)
	switch {
	case strings.Contains(normalized, "auditor"):
		return 4
	case strings.Contains(normalized, "desenvolvedor") ||
		strings.Contains(normalized, "operario") ||
		strings.Contains(normalized, "automacao"):
		return 2
	case strings.Contains(normalized, "analista") ||
		strings.Contains(normalized, "redator") ||
		strings.Contains(normalized, "designer") ||
		strings.Contains(normalized, "estrategista"):
		return 3
	default:
		return 1
	}
}

func workerIdleRole(workerID int) string {
	switch workerID {
	case 1:
		return "Gerencia/Arquitetura"
	case 2:
		return "Desenvolvimento/Automacao"
	case 3:
		return "Analise/Design/Redacao"
	case 4:
		return "Auditoria Tecnica"
	default:
		return ""
	}
}

func buildAssertivenessMetrics(tasks []database.Task) assertivenessMetrics {
	metrics := assertivenessMetrics{}
	roleBuckets := map[string]*bucket{}
	domainBuckets := map[string]*bucket{}
	totalSeconds := 0.0
	timedTasks := 0
	for _, task := range tasks {
		if task.Status == database.StatusApproved {
			metrics.Approved++
		}
		if task.Attempts > 0 || task.Status == database.StatusRedoing {
			metrics.Redone++
		}
		if task.Status == database.StatusBlocked {
			metrics.Blocked++
		}
		terminal := isMetricTerminalStatus(task.Status)
		duration := taskDurationSeconds(task)
		if terminal && duration >= 0 {
			totalSeconds += duration
			timedTasks++
		}
		roleKey := strings.TrimSpace(task.Role)
		if roleKey == "" {
			roleKey = "sem papel"
		}
		domainKey := taskDomain(task)
		addMetricBucket(roleBuckets, roleKey, task, terminal, duration)
		addMetricBucket(domainBuckets, domainKey, task, terminal, duration)
	}
	if timedTasks > 0 {
		metrics.AverageSeconds = totalSeconds / float64(timedTasks)
	}
	metrics.SuccessRateByRole = bucketItems(roleBuckets)
	metrics.SuccessRateByDomain = bucketItems(domainBuckets)
	return metrics
}

func addMetricBucket(buckets map[string]*bucket, key string, task database.Task, terminal bool, duration float64) {
	item := buckets[key]
	if item == nil {
		item = &bucket{}
		buckets[key] = item
	}
	if !terminal {
		return
	}
	item.total++
	if task.Status == database.StatusApproved {
		item.approved++
	} else if isMetricFailureStatus(task.Status) {
		item.failed++
	}
	if duration >= 0 {
		item.seconds += duration
		item.timed++
	}
}

type bucket struct {
	approved int
	failed   int
	total    int
	seconds  float64
	timed    int
}

func bucketItems(buckets map[string]*bucket) []successRateItem {
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]successRateItem, 0, len(keys))
	for _, key := range keys {
		item := buckets[key]
		if item.total == 0 {
			continue
		}
		result := successRateItem{
			Key:      key,
			Approved: item.approved,
			Failed:   item.failed,
			Total:    item.total,
		}
		if item.total > 0 {
			result.SuccessRate = float64(item.approved) / float64(item.total)
		}
		if item.timed > 0 {
			result.AverageSeconds = item.seconds / float64(item.timed)
		}
		out = append(out, result)
	}
	return out
}

func isMetricTerminalStatus(status string) bool {
	return status == database.StatusApproved || isMetricFailureStatus(status)
}

func isMetricFailureStatus(status string) bool {
	return status == database.StatusBlocked || status == database.StatusRejected || status == database.StatusCancelled
}

func taskDurationSeconds(task database.Task) float64 {
	if task.CreatedAt.IsZero() || task.UpdatedAt.IsZero() || task.UpdatedAt.Before(task.CreatedAt) {
		return -1
	}
	return task.UpdatedAt.Sub(task.CreatedAt).Seconds()
}

func taskDomain(task database.Task) string {
	if value, ok := task.Payload["domain"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if value, ok := task.Payload["key"].(string); ok && strings.Contains(value, "-") {
		return strings.TrimSpace(strings.SplitN(value, "-", 2)[0])
	}
	return "general"
}

func (m *Manager) execute(task database.Task) (map[string]any, error) {
	incoming, err := m.store.HandoffsForTask(task)
	if err != nil {
		return nil, err
	}
	contract, err := m.store.GetContract(task.ContractID)
	if err != nil {
		return nil, err
	}
	jitKnowledgePacks := m.jitKnowledgePacksForTask(task, contract, incoming)
	statePayload := map[string]any{
		"task_payload":        task.Payload,
		"incoming_handoffs":   incoming,
		"contract_hash":       contract.Hash,
		"north_star":          contract.NorthStar,
		"constraints":         contract.Constraints,
		"deliverables":        contract.Deliverables,
		"jit_knowledge_packs": jitKnowledgePacks,
	}
	state, _ := json.Marshal(statePayload)
	res, err := m.engine.Infer(task.Role, task.Title, string(state))
	if err != nil {
		return nil, err
	}
	var validations []validationResult
	if isAuditorRole(task.Role) {
		validations, err = m.validateContract(contract, task)
		if err != nil {
			return nil, err
		}
	}
	fileResult, err := m.executeFileTask(task, validations)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"task_id":             task.ID,
		"role":                task.Role,
		"contract_id":         task.ContractID,
		"contract_hash":       contract.Hash,
		"report":              res.Text + "\n" + fileResult.Report,
		"engine_hash":         res.EngineHash,
		"incoming":            incoming,
		"jit_knowledge_packs": jitKnowledgePacks,
		"artifacts":           fileResult.Artifacts,
		"validations":         fileResult.Validations,
		"sealed_at":           time.Now().Format(time.RFC3339),
	}
	if len(res.Structured) > 0 {
		var structured map[string]any
		if err := json.Unmarshal(res.Structured, &structured); err == nil {
			payload["execution"] = structured
		}
	}
	if err := audit(contract, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (m *Manager) jitKnowledgePacksForTask(task database.Task, contract database.Contract, incoming []database.Handoff) []jitKnowledgePackItem {
	query := buildJITQuery(task, contract, incoming)
	results, err := m.knowledgePacks.Search(query, 5)
	if err != nil {
		return nil
	}
	out := make([]jitKnowledgePackItem, 0, len(results))
	for _, result := range results {
		ruleMap, err := m.knowledgePacks.Load(result.Topic)
		if err != nil {
			continue
		}
		out = append(out, jitKnowledgePackItem{
			Topic:    ruleMap.Topic,
			Source:   ruleMap.Source,
			Summary:  ruleMap.Summary,
			Keywords: ruleMap.Keywords,
			Rules:    ruleMap.Rules,
			Score:    result.Score,
		})
	}
	if len(out) > 0 {
		m.store.Log("INFO", "knowledge_packs", fmt.Sprintf("JIT anexou %d pack(s) para task %d", len(out), task.ID))
	}
	return out
}

func buildJITQuery(task database.Task, contract database.Contract, incoming []database.Handoff) string {
	rawPayload, _ := json.Marshal(task.Payload)
	rawIncoming, _ := json.Marshal(incoming)
	return strings.Join([]string{
		task.Title,
		task.Role,
		contract.NorthStar,
		contract.Constraints,
		contract.Deliverables,
		string(rawPayload),
		string(rawIncoming),
	}, " ")
}

func (m *Manager) createHandoffs(task database.Task, payload map[string]any) error {
	destinations, err := m.dependentTaskIDs(task)
	if err != nil {
		return err
	}
	if len(destinations) == 0 {
		_, err := m.store.AddHandoff(task.ContractID, task.ID, 0, "handoff", clonePayload(payload))
		return err
	}
	for _, toTaskID := range destinations {
		if _, err := m.store.AddHandoff(task.ContractID, task.ID, toTaskID, "handoff", clonePayload(payload)); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) dependentTaskIDs(task database.Task) ([]int64, error) {
	tasks, err := m.store.ListTasks()
	if err != nil {
		return nil, err
	}
	var out []int64
	for _, candidate := range tasks {
		if candidate.ContractID != task.ContractID || candidate.ID == task.ID {
			continue
		}
		for _, depID := range candidate.Dependencies {
			if depID == task.ID {
				out = append(out, candidate.ID)
				break
			}
		}
	}
	return out, nil
}

func clonePayload(payload map[string]any) map[string]any {
	raw, err := json.Marshal(payload)
	if err != nil {
		out := map[string]any{}
		for key, value := range payload {
			out[key] = value
		}
		return out
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		out = map[string]any{}
	}
	return out
}

func (m *Manager) createKnowledgeCandidate(task database.Task, payload map[string]any) error {
	artifacts, ok := payload["artifacts"].([]fileArtifact)
	if !ok || len(artifacts) == 0 {
		return nil
	}
	contract, err := m.store.GetContract(task.ContractID)
	if err != nil {
		return err
	}
	domain := normalizeCandidateDomain(fmt.Sprint(task.Payload["domain"]))
	pattern := candidatePattern(contract, task)
	action := "learned_" + slugCandidate(fmt.Sprintf("%s %d", domain, task.ID))
	template := fmt.Sprintf("Ao executar tarefa do dominio %s, preservar contrato %s e validar artefatos antes de selar: {{.Description}}.", domain, contract.Hash)
	evidence := candidateEvidence(contract, task, artifacts, payload)
	_, err = m.store.AddKnowledgeCandidate(contract.ID, task.ID, domain, pattern, action, template, evidence)
	if err == nil {
		m.store.Log("INFO", "knowledge_packs", fmt.Sprintf("regra candidata pendente criada para task %d", task.ID))
	}
	return err
}

func candidatePattern(contract database.Contract, task database.Task) string {
	for _, source := range []string{fmt.Sprint(task.Payload["key"]), task.Role, contract.NorthStar, contract.Deliverables} {
		for _, word := range strings.Fields(strings.ToLower(source)) {
			clean := strings.Trim(word, ".,;:!?()[]{}\"'")
			if len([]rune(clean)) >= 4 {
				return clean
			}
		}
	}
	return "contrato"
}

func candidateEvidence(contract database.Contract, task database.Task, artifacts []fileArtifact, payload map[string]any) string {
	evidence := map[string]any{
		"contract_id":   contract.ID,
		"contract_hash": contract.Hash,
		"task_id":       task.ID,
		"role":          task.Role,
		"domain":        normalizeCandidateDomain(fmt.Sprint(task.Payload["domain"])),
		"artifacts":     artifacts,
		"sealed_at":     payload["sealed_at"],
		"engine_hash":   payload["engine_hash"],
		"validations":   payload["validations"],
		"execution":     payload["execution"],
	}
	raw, err := json.Marshal(evidence)
	if err != nil {
		return fmt.Sprintf("contract=%d hash=%s task=%d role=%s artifacts=%d", contract.ID, contract.Hash, task.ID, task.Role, len(artifacts))
	}
	return string(raw)
}

func normalizeCandidateDomain(domain string) string {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" || domain == "<nil>" {
		return "general"
	}
	return slugCandidate(domain)
}

func slugCandidate(text string) string {
	var out strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(text) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			out.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(out.String(), "-")
}

func packHasAction(pack *knowledge.KnowledgePack, action string) bool {
	for _, rule := range pack.Rules {
		if rule.Action == action {
			return true
		}
	}
	return false
}

func isAuditorRole(role string) bool {
	return strings.Contains(strings.ToLower(role), "auditor")
}

func audit(contract database.Contract, payload map[string]any) error {
	report, _ := payload["report"].(string)
	if strings.TrimSpace(report) == "" {
		return errors.New("relatorio vazio")
	}
	if err := auditContractFidelity(contract, payload, report); err != nil {
		return err
	}
	if err := auditNarrativeNoise(report); err != nil {
		return err
	}
	if err := auditLooping(report); err != nil {
		return err
	}
	artifacts, ok := payload["artifacts"].([]fileArtifact)
	if !ok || len(artifacts) == 0 {
		return errors.New("nenhum artefato real produzido")
	}
	for _, artifact := range artifacts {
		if err := auditArtifact(artifact); err != nil {
			return err
		}
	}
	return nil
}

func auditContractFidelity(contract database.Contract, payload map[string]any, report string) error {
	if contract.ID == 0 {
		return errors.New("contrato ausente na auditoria")
	}
	hash, _ := payload["contract_hash"].(string)
	if hash != contract.Hash {
		return fmt.Errorf("hash do contrato divergente: payload=%q esperado=%q", hash, contract.Hash)
	}
	if !containsAnyTerm(report, contract.NorthStar, 2) && !containsAnyTerm(report, contract.Deliverables, 2) {
		return errors.New("saida rejeitada: nao referencia o objetivo ou entregaveis do contrato")
	}
	return nil
}

func auditNarrativeNoise(report string) error {
	text := strings.ToLower(report)
	banned := []string{
		"como uma ia",
		"como um modelo de linguagem",
		"espero que",
		"com certeza!",
		"claro!",
		"fico feliz",
		"espero ter ajudado",
		"posso ajudar com mais alguma coisa",
	}
	for _, token := range banned {
		if strings.Contains(text, token) {
			return errors.New("saida rejeitada por lero-lero de IA: " + token)
		}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return errors.New("saida sem termos auditaveis")
	}
	filler := 0
	for _, word := range words {
		clean := strings.Trim(word, ".,;:!?()[]{}\"'")
		switch clean {
		case "obrigado", "agradeco", "feliz", "ajudar", "certamente", "claro", "excelente", "perfeito":
			filler++
		}
	}
	if float64(filler)/float64(len(words)) > 0.05 {
		return fmt.Errorf("saida rejeitada: ruido narrativo %.1f%% excede limite de 5%%", 100*float64(filler)/float64(len(words)))
	}
	return nil
}

func auditLooping(report string) error {
	words := strings.Fields(strings.ToLower(report))
	if len(words) < 12 {
		return nil
	}
	counts := map[string]int{}
	for i := 0; i <= len(words)-4; i++ {
		gram := strings.Join(words[i:i+4], " ")
		counts[gram]++
		if counts[gram] >= 4 {
			return errors.New("saida rejeitada: loop textual detectado")
		}
	}
	for _, paragraph := range strings.Split(report, "\n") {
		line := strings.TrimSpace(paragraph)
		if len(line) > 40 && strings.Count(report, line) >= 3 {
			return errors.New("saida rejeitada: bloco repetido detectado")
		}
	}
	return nil
}

func containsAnyTerm(text, source string, minimum int) bool {
	text = strings.ToLower(text)
	seen := map[string]struct{}{}
	for _, word := range strings.Fields(strings.ToLower(source)) {
		clean := strings.Trim(word, ".,;:!?()[]{}\"'")
		if len([]rune(clean)) < 4 {
			continue
		}
		if strings.Contains(text, clean) {
			seen[clean] = struct{}{}
			if len(seen) >= minimum {
				return true
			}
		}
	}
	return false
}

func auditArtifact(artifact fileArtifact) error {
	path := filepath.Clean(artifact.Path)
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("artefato ausente: %s", path)
	}
	if info.Size() == 0 {
		return fmt.Errorf("artefato vazio: %s", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != artifact.SHA256 {
		return fmt.Errorf("hash divergente no artefato: %s", path)
	}
	if strings.EqualFold(filepath.Ext(path), ".json") && !json.Valid(raw) {
		return fmt.Errorf("json invalido: %s", path)
	}
	return nil
}

func (m *Manager) purgeExpiredKnowledgePacks() {
	if n, err := m.knowledgePacks.PurgeExpired(); err == nil && n > 0 {
		m.store.Log("INFO", "knowledge_packs", "knowledge pack temporario expirado removido")
	}
}

func interrogationQuestions(input string) []string {
	questions := []string{
		"Qual e o resultado final exato que deve ser considerado sucesso?",
		"Quais tecnologias, APIs, bancos ou arquivos sao obrigatorios?",
		"Quais restricoes sao inegociaveis?",
		"Como a entrega deve ser validada tecnicamente?",
	}
	lower := strings.ToLower(input)
	if !strings.Contains(lower, "interface") && !strings.Contains(lower, "dashboard") && !strings.Contains(lower, "api") {
		questions = append(questions, "A entrega precisa de interface visual, API, CLI ou apenas arquivos?")
	}
	if !strings.Contains(lower, "teste") && !strings.Contains(lower, "valid") {
		questions = append(questions, "Quais testes ou comandos devem ser usados como crivo de aprovacao?")
	}
	return questions
}

func buildContractFromAnswers(session database.Interrogation, answers map[string]string) (string, string, string) {
	success := strings.TrimSpace(answers["q1"])
	stack := strings.TrimSpace(answers["q2"])
	limits := strings.TrimSpace(answers["q3"])
	validation := strings.TrimSpace(answers["q4"])
	shape := strings.TrimSpace(answers["q5"])
	tests := strings.TrimSpace(answers["q6"])
	northStar := session.Input
	if success != "" {
		northStar = success
	}
	var constraints []string
	constraints = append(constraints, "Contrato imutavel por hash; SQLite como fonte de verdade; tarefas aprovadas em read-only.")
	if stack != "" {
		constraints = append(constraints, "Stack obrigatoria: "+stack)
	}
	if limits != "" {
		constraints = append(constraints, "Restricoes: "+limits)
	}
	if validation != "" {
		constraints = append(constraints, "Validacao tecnica: "+validation)
	}
	deliverables := "Checklist executavel, relatorios JSON no SQLite, logs tecnicos e artefatos selados."
	if shape != "" {
		deliverables += " Formato de entrega: " + shape + "."
	}
	if tests != "" {
		deliverables += " Crivo de aprovacao: " + tests + "."
	}
	return northStar, strings.Join(constraints, " "), deliverables
}

func completeInterrogationAnswers(session database.Interrogation, answers map[string]string) map[string]string {
	if answers == nil {
		answers = map[string]string{}
	}
	complete := map[string]string{}
	for key, value := range answers {
		complete[key] = strings.TrimSpace(value)
	}
	for i, question := range session.Questions {
		key := fmt.Sprintf("q%d", i+1)
		if strings.TrimSpace(complete[key]) == "" {
			complete[key] = "Nao informado: " + question
		}
	}
	return complete
}

func buildContractDraft(northStar, constraints, deliverables string) ContractDraft {
	northStar = strings.TrimSpace(northStar)
	constraints = strings.TrimSpace(constraints)
	deliverables = strings.TrimSpace(deliverables)
	return ContractDraft{
		NorthStar:    northStar,
		Constraints:  constraints,
		Deliverables: deliverables,
		Hash:         database.ContractHash(northStar, constraints, deliverables),
	}
}
