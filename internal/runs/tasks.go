package runs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
	"gopkg.in/yaml.v3"
)

const (
	TaskFileName     = "task.md"
	TaskRunsFileName = "runs.ndjson"
)

// TaskFrontmatter is the machine-readable task snapshot stored in task.md frontmatter.
type TaskFrontmatter struct {
	TraceID   string              `yaml:"traceId"`
	TaskID    string              `yaml:"taskId"`
	Title     string              `yaml:"title,omitempty"`
	Bee       string              `yaml:"bee,omitempty"`
	Intent    string              `yaml:"intent,omitempty"`
	Status    protocol.TaskStatus `yaml:"status"`
	DependsOn []string            `yaml:"dependsOn,omitempty"`
	Summary   string              `yaml:"summary,omitempty"`
	Commit    string              `yaml:"commit,omitempty"`
	UpdatedAt string              `yaml:"updatedAt,omitempty"`
}

// TaskRunEntry is one line in runs.ndjson linking a task to an agent run directory.
type TaskRunEntry struct {
	AgentID    string    `json:"agentId"`
	Bee        string    `json:"bee,omitempty"`
	RunDir     string    `json:"runDir"`
	StartedAt  time.Time `json:"startedAt,omitempty"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
	RunStatus  string    `json:"runStatus,omitempty"`
}

// TaskDir holds paths for one task projection under a trace.
type TaskDir struct {
	ColonyRoot string
	TraceID    string
	TaskID     string
}

// Root returns .paseka/runs/<traceId>/tasks/<taskId>/.
func (d TaskDir) Root() string {
	return filepath.Join(d.ColonyRoot, ".paseka", "runs", d.TraceID, "tasks", d.TaskID)
}

func (d TaskDir) TaskPath() string {
	return filepath.Join(d.Root(), TaskFileName)
}

func (d TaskDir) RunsPath() string {
	return filepath.Join(d.Root(), TaskRunsFileName)
}

// TraceTasksRoot returns .paseka/runs/<traceId>/tasks/.
func TraceTasksRoot(colonyRoot, traceID string) string {
	return filepath.Join(colonyRoot, ".paseka", "runs", traceID, "tasks")
}

// NewTaskDir validates identifiers and returns a task directory handle.
func NewTaskDir(colonyRoot, traceID, taskID string) (TaskDir, error) {
	if colonyRoot == "" || traceID == "" || taskID == "" {
		return TaskDir{}, fmt.Errorf("runs: colony root, traceId, and taskId are required")
	}
	return TaskDir{ColonyRoot: colonyRoot, TraceID: traceID, TaskID: taskID}, nil
}

// WriteTaskSnapshot writes task.md from a ledger task snapshot.
func WriteTaskSnapshot(colonyRoot string, traceID string, task taskledger.TaskSnapshot) error {
	d, err := NewTaskDir(colonyRoot, traceID, task.TaskID)
	if err != nil {
		return err
	}
	fm := TaskFrontmatter{
		TraceID:   traceID,
		TaskID:    task.TaskID,
		Title:     task.Title,
		Bee:       task.Bee,
		Intent:    task.Intent,
		Status:    task.Status,
		DependsOn: append([]string(nil), task.DependsOn...),
		Summary:   task.Summary,
		Commit:    task.Commit,
	}
	if !task.UpdatedAt.IsZero() {
		fm.UpdatedAt = task.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return d.WriteTask(fm, task.Body)
}

// SyncTraceTasks writes task.md for every task in a trace snapshot.
func SyncTraceTasks(colonyRoot string, trace taskledger.TraceSnapshot) error {
	if trace.TraceID == "" {
		return fmt.Errorf("runs: traceId is required")
	}
	for _, task := range trace.Tasks {
		if err := WriteTaskSnapshot(colonyRoot, trace.TraceID, task); err != nil {
			return err
		}
	}
	return nil
}

// WriteTask writes task.md with YAML frontmatter and a markdown body.
func (d TaskDir) WriteTask(fm TaskFrontmatter, body string) error {
	if err := os.MkdirAll(d.Root(), 0o755); err != nil {
		return fmt.Errorf("runs: mkdir task dir: %w", err)
	}
	content := MarshalTaskMarkdown(fm, body)
	return os.WriteFile(d.TaskPath(), []byte(content), 0o644)
}

// ReadTask parses task.md into frontmatter and body.
func (d TaskDir) ReadTask() (TaskFrontmatter, string, error) {
	data, err := os.ReadFile(d.TaskPath())
	if err != nil {
		return TaskFrontmatter{}, "", err
	}
	return ParseTaskMarkdown(string(data))
}

// AppendTaskRun appends one run linkage entry for this task.
func (d TaskDir) AppendTaskRun(entry TaskRunEntry) error {
	if err := os.MkdirAll(d.Root(), 0o755); err != nil {
		return fmt.Errorf("runs: mkdir task dir: %w", err)
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(d.RunsPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

// ReadTaskRuns reads all run linkage entries for this task.
func (d TaskDir) ReadTaskRuns() ([]TaskRunEntry, error) {
	f, err := os.Open(d.RunsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []TaskRunEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry TaskRunEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("runs: parse task run: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// UpdateTaskRunStatus updates the matching agent run entry in runs.ndjson.
func (d TaskDir) UpdateTaskRunStatus(agentID, runStatus string, finishedAt time.Time) error {
	entries, err := d.ReadTaskRuns()
	if err != nil {
		return err
	}
	updated := false
	for i := range entries {
		if entries[i].AgentID != agentID {
			continue
		}
		entries[i].RunStatus = runStatus
		if !finishedAt.IsZero() {
			entries[i].FinishedAt = finishedAt
		}
		updated = true
	}
	if !updated {
		return nil
	}
	return d.writeTaskRuns(entries)
}

func (d TaskDir) writeTaskRuns(entries []TaskRunEntry) error {
	if err := os.MkdirAll(d.Root(), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return os.WriteFile(d.RunsPath(), []byte(b.String()), 0o644)
}

// AppendTaskRun is a convenience wrapper for recording a new task run linkage.
func AppendTaskRun(colonyRoot, traceID, taskID string, entry TaskRunEntry) error {
	d, err := NewTaskDir(colonyRoot, traceID, taskID)
	if err != nil {
		return err
	}
	return d.AppendTaskRun(entry)
}

// UpdateTaskRunStatus is a convenience wrapper for finishing a task run linkage entry.
func UpdateTaskRunStatus(colonyRoot, traceID, taskID, agentID, runStatus string, finishedAt time.Time) error {
	d, err := NewTaskDir(colonyRoot, traceID, taskID)
	if err != nil {
		return err
	}
	return d.UpdateTaskRunStatus(agentID, runStatus, finishedAt)
}

// LoadTraceTasksFromFS loads task snapshots from the filesystem projection for one trace.
func LoadTraceTasksFromFS(colonyRoot, traceID string) (taskledger.TraceSnapshot, error) {
	root := TraceTasksRoot(colonyRoot, traceID)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return taskledger.TraceSnapshot{TraceID: traceID, Tasks: map[string]taskledger.TaskSnapshot{}}, nil
		}
		return taskledger.TraceSnapshot{}, err
	}

	tasks := make(map[string]taskledger.TaskSnapshot)
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		d := TaskDir{ColonyRoot: colonyRoot, TraceID: traceID, TaskID: ent.Name()}
		fm, body, err := d.ReadTask()
		if err != nil {
			return taskledger.TraceSnapshot{}, fmt.Errorf("runs: read task %s: %w", ent.Name(), err)
		}
		task := taskledger.TaskSnapshot{
			TaskID:    fm.TaskID,
			Title:     fm.Title,
			Body:      body,
			Bee:       fm.Bee,
			Intent:    fm.Intent,
			Status:    fm.Status,
			DependsOn: append([]string(nil), fm.DependsOn...),
			Summary:   fm.Summary,
			Commit:    fm.Commit,
		}
		if fm.UpdatedAt != "" {
			if ts, err := time.Parse(time.RFC3339, fm.UpdatedAt); err == nil {
				task.UpdatedAt = ts
			}
		}
		if task.TaskID == "" {
			task.TaskID = ent.Name()
		}
		tasks[task.TaskID] = task
	}
	return taskledger.TraceSnapshot{TraceID: traceID, Tasks: tasks}, nil
}

// ListTraceTaskIDs returns sorted task ids from the filesystem projection.
func ListTraceTaskIDs(colonyRoot, traceID string) ([]string, error) {
	root := TraceTasksRoot(colonyRoot, traceID)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, ent := range entries {
		if ent.IsDir() {
			ids = append(ids, ent.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// MarshalTaskMarkdown renders task.md content from frontmatter and body.
func MarshalTaskMarkdown(fm TaskFrontmatter, body string) string {
	yamlData, err := yaml.Marshal(fm)
	if err != nil {
		yamlData = []byte("taskId: " + fm.TaskID + "\n")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return "---\n" + string(yamlData) + "---\n"
	}
	return "---\n" + string(yamlData) + "---\n\n" + body + "\n"
}

// ParseTaskMarkdown splits YAML frontmatter and markdown body.
func ParseTaskMarkdown(content string) (TaskFrontmatter, string, error) {
	content = strings.TrimPrefix(content, "\ufeff")
	if !strings.HasPrefix(content, "---") {
		return TaskFrontmatter{}, strings.TrimSpace(content), nil
	}
	rest := content[len("---"):]
	rest = strings.TrimPrefix(rest, "\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return TaskFrontmatter{}, strings.TrimSpace(content), nil
	}
	yamlPart := rest[:end]
	body := strings.TrimSpace(rest[end+len("\n---"):])
	var fm TaskFrontmatter
	if err := yaml.Unmarshal([]byte(yamlPart), &fm); err != nil {
		return TaskFrontmatter{}, body, fmt.Errorf("runs: parse task frontmatter: %w", err)
	}
	return fm, body, nil
}
