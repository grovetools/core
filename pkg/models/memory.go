package models

import "time"

// Memory transport types for daemon ↔ client communication.
// These mirror the internal types in github.com/grovetools/memory/pkg/memory
// but live in core so that the daemon and TUI clients can share a wire format
// without either side importing the memory submodule directly.

// MemorySearchRequest is the request body for POST /api/memory/search.
type MemorySearchRequest struct {
	Query         string `json:"query"`
	Limit         int    `json:"limit,omitempty"`
	DocTypeFilter string `json:"doc_type_filter,omitempty"`
	Scope         string `json:"scope,omitempty"`          // "local" | "all" | "raw"
	WorkspacePath string `json:"workspace_path,omitempty"` // Current workspace root (for local scoping)
	UseFTS        bool   `json:"use_fts"`
	UseVector     bool   `json:"use_vector"`
}

// MemorySearchResult is a single hit returned by /api/memory/search.
type MemorySearchResult struct {
	DocumentID string  `json:"document_id"`
	ChunkID    int64   `json:"chunk_id"`
	DocType    string  `json:"doc_type"`
	Content    string  `json:"content"`
	Path       string  `json:"path"`
	Metadata   []byte  `json:"metadata,omitempty"`
	Score      float32 `json:"score"`
	FTSRank    *int    `json:"fts_rank,omitempty"`
	VectorRank *int    `json:"vector_rank,omitempty"`
}

// MemoryCoverageRequest is the request body for POST /api/memory/coverage.
type MemoryCoverageRequest struct {
	TargetPath string  `json:"target_path,omitempty"`
	Tolerance  float32 `json:"tolerance,omitempty"`
}

// MemoryCoverageReport is the response from POST /api/memory/coverage.
type MemoryCoverageReport struct {
	OverallCoveragePercentage float32              `json:"overall_coverage_percentage"`
	AdaptiveThreshold         float32              `json:"adaptive_threshold"`
	Tolerance                 float32              `json:"tolerance"`
	Files                     []MemoryFileCoverage `json:"files"`
}

// MemoryFileCoverage breaks down coverage gaps per target file.
type MemoryFileCoverage struct {
	Path                   string  `json:"path"`
	TotalChunks            int     `json:"total_chunks"`
	UncoveredChunks        int     `json:"uncovered_chunks"`
	CoveragePercentage     float32 `json:"coverage_percentage"`
	NearestConceptPath     string  `json:"nearest_concept_path"`
	NearestConceptDistance float32 `json:"nearest_concept_distance"`
}

// MemoryStatusResponse is returned by GET /api/memory/status.
type MemoryStatusResponse struct {
	DBPath      string         `json:"db_path"`
	DBSizeBytes int64          `json:"db_size_bytes"`
	Documents   int            `json:"documents"`
	ChunksVec   int            `json:"chunks_vec"`
	DocTypes    map[string]int `json:"doc_types,omitempty"`
}

// DocumentPathInfo carries path/workspace/timestamp tuples used by daemon
// handlers that combine DB results with filesystem checks.
type DocumentPathInfo struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	DocType   string    `json:"doc_type"`
	Workspace string    `json:"workspace"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MemoryReindexRequest is the request body for POST /api/memory/reindex.
type MemoryReindexRequest struct {
	Mode   string `json:"mode"`             // "stale" (default), "all", "path"
	Target string `json:"target,omitempty"` // File path (only for mode "path")
}

// MemoryReindexResponse is the response from POST /api/memory/reindex.
type MemoryReindexResponse struct {
	QueuedCount int    `json:"queued_count"`
	Mode        string `json:"mode"`
}

// EmbeddingProgressResponse is the response for GET /api/memory/analysis/progress.
type EmbeddingProgressResponse struct {
	QueueDepth            int     `json:"queue_depth"`
	InFlight              int     `json:"in_flight"`
	FilesCompletedLastMin int     `json:"files_completed_last_min"`
	ChunksEmbeddedLastMin int     `json:"chunks_embedded_last_min"`
	ErrorsLastMin         int     `json:"errors_last_min"`
	ETASeconds            float64 `json:"eta_seconds"`
	IsHealthy             bool    `json:"is_healthy"`
}

// GCAnalysisResponse is the response for /api/memory/analysis/gc.
type GCAnalysisResponse struct {
	ZombieCount   int      `json:"zombie_count"`
	MissingCount  int      `json:"missing_count"`
	StaleCount    int      `json:"stale_count"`
	PathsToRemove []string `json:"paths_to_remove,omitempty"`
	PathsRemoved  []string `json:"paths_removed,omitempty"`
}

// WorkspaceAnalysis is a per-workspace breakdown of indexed documents.
type WorkspaceAnalysis struct {
	Workspace     string         `json:"workspace"`
	TotalDocs     int            `json:"total_docs"`
	CanonicalDocs int            `json:"canonical_docs"`
	WorktreeDocs  int            `json:"worktree_docs"`
	Types         map[string]int `json:"types"`
	LastIndexed   time.Time      `json:"last_indexed"`
	StaleCount    int            `json:"stale_count"`
}

// EcosystemAnalysis describes coverage gaps at the ecosystem level.
type EcosystemAnalysis struct {
	Name             string   `json:"name"`
	Path             string   `json:"path"`
	ConfiguredGroves int      `json:"configured_groves"`
	IndexedGroves    int      `json:"indexed_groves"`
	ZeroCoverage     []string `json:"zero_coverage_workspaces"`
	LanguageGaps     []string `json:"language_gaps"`
}

// CodeAnalysis describes code-indexer health.
type CodeAnalysis struct {
	TotalCodeDocs     int            `json:"total_code_docs"`
	MissingEmbeddings int            `json:"missing_embeddings"`
	WorktreeOverrides int            `json:"worktree_overrides"`
	TopFatFiles       []FatFile      `json:"top_fat_files"`
	Languages         map[string]int `json:"languages"`
}

// FatFile is a code document with a chunk count.
type FatFile struct {
	Path       string `json:"path"`
	ChunkCount int    `json:"chunk_count"`
}

// ConceptAnalysis is a concept-coverage summary.
type ConceptAnalysis struct {
	TotalConcepts int            `json:"total_concepts"`
	ConceptCounts []ConceptChunk `json:"concept_counts"`
}

// ConceptChunk is a concept document path with its chunk count.
type ConceptChunk struct {
	Workspace  string `json:"workspace"`
	Path       string `json:"path"`
	ChunkCount int    `json:"chunk_count"`
}

// EmbeddingAnalysis describes embedder health and dedup savings.
type EmbeddingAnalysis struct {
	TotalChunks    int `json:"total_chunks"`
	UniqueChunks   int `json:"unique_chunks"`
	DedupSavings   int `json:"dedup_savings"`
	MissingVectors int `json:"missing_vectors"`
}

// FreshnessAnalysis describes time-bucket distribution and stale source files.
type FreshnessAnalysis struct {
	LastHour   int `json:"last_hour"`
	LastDay    int `json:"last_day"`
	LastWeek   int `json:"last_week"`
	Older      int `json:"older"`
	StaleFiles int `json:"stale_files"`
}

// DuplicateAnalysis is the response for /api/memory/analysis/duplicates.
type DuplicateAnalysis struct {
	TopReusedChunks []ReusedChunk `json:"top_reused_chunks"`
}

// ReusedChunk is a content hash present across multiple documents.
type ReusedChunk struct {
	ContentHash string `json:"content_hash"`
	DocCount    int    `json:"doc_count"`
	Snippet     string `json:"snippet"`
}

// NotebookAnalysis is per-workspace notebook lifecycle counts.
type NotebookAnalysis struct {
	Workspace      string `json:"workspace"`
	InboxCount     int    `json:"inbox_count"`
	IssuesCount    int    `json:"issues_count"`
	CompletedCount int    `json:"completed_count"`
	ConceptsCount  int    `json:"concepts_count"`
	SkillsCount    int    `json:"skills_count"`
}

// ContextAnalysis is the response for /api/memory/analysis/context.
type ContextAnalysis struct {
	TotalPresets int                 `json:"total_presets"`
	Presets      []ContextPresetStat `json:"presets"`
}

// ContextPresetStat summarizes a single cx context preset rules file.
type ContextPresetStat struct {
	Workspace    string `json:"workspace"`
	Path         string `json:"path"`
	FileCount    int    `json:"file_count"`
	MissingFiles int    `json:"missing_files"`
}
