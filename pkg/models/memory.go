package models

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
