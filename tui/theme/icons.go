package theme

import (
	"os"

	"github.com/mattsolo1/grove-core/config"
)

// Nerd Font Icons (Private Constants)
const (
	nerdIconTree                   = "" // fa-tree
	nerdIconProject                = "" // cod-project
	nerdIconRepo                   = "" // cod-repo
	nerdIconWorktree               = "" // dev-git_branch
	nerdIconEcosystem              = "" // fa-folder_tree
	nerdIconGitBranch              = "" // dev-git_branch
	nerdIconSuccess                = "✓"
	nerdIconError                  = "✗"
	nerdIconWarning                = "⚠"
	nerdIconInfo                   = "ℹ"
	nerdIconRunning                = "" // fa-refresh
	nerdIconPending                = "…"
	nerdIconSelect                 = "▶"
	nerdIconArrow                  = "→"
	nerdIconBullet                 = "•"
	nerdIconNote                   = "" // cod-note
	nerdIconPlan                   = "" // oct-project_roadmap
	nerdIconChat                   = "💬"
	nerdIconOneshot                = "🎯"
	nerdIconInteractiveAgent       = "🤖"
	nerdIconHeadlessAgent          = "◆"
	nerdIconShell                  = "▶"
	nerdIconStatusCompleted        = "●"
	nerdIconStatusRunning          = "◐"
	nerdIconStatusFailed           = "✗"
	nerdIconStatusBlocked          = "🚫"
	nerdIconStatusNeedsReview      = "👁"
	nerdIconStatusPendingUser      = "○"
	nerdIconStatusHold             = "⏸"
	nerdIconStatusTodo             = "○"
	nerdIconStatusAbandoned        = "🗑️"
	nerdIconStatusInterrupted      = "⊗"
)

// ASCII Fallback Icons (Private Constants)
const (
	asciiIconTree              = "🌲"
	asciiIconProject           = "◆"
	asciiIconRepo              = "●"
	asciiIconWorktree          = "⑂"
	asciiIconEcosystem         = "◆"
	asciiIconGitBranch         = "⎇"
	asciiIconSuccess           = "✓"
	asciiIconError             = "✗"
	asciiIconWarning           = "⚠"
	asciiIconInfo              = "ℹ"
	asciiIconRunning           = "◐"
	asciiIconPending           = "…"
	asciiIconSelect            = "▶"
	asciiIconArrow             = "→"
	asciiIconBullet            = "•"
	asciiIconNote              = "▢"
	asciiIconPlan              = "▣"
	asciiIconChat              = "★"
	asciiIconOneshot           = "●"
	asciiIconInteractiveAgent  = "⚙"
	asciiIconHeadlessAgent     = "◆"
	asciiIconShell             = "▶"
	asciiIconStatusCompleted   = "●"
	asciiIconStatusRunning     = "◐"
	asciiIconStatusFailed      = "✗"
	asciiIconStatusBlocked     = "🚫"
	asciiIconStatusNeedsReview = "👁"
	asciiIconStatusPendingUser = "○"
	asciiIconStatusHold        = "⏸"
	asciiIconStatusTodo        = "○"
	asciiIconStatusAbandoned   = "🗑️"
	asciiIconStatusInterrupted = "⊗"
)

// Public Icon Variables
var (
	IconTree              string
	IconProject           string
	IconRepo              string
	IconWorktree          string
	IconEcosystem         string
	IconGitBranch         string
	IconSuccess           string
	IconError             string
	IconWarning           string
	IconInfo              string
	IconRunning           string
	IconPending           string
	IconSelect            string
	IconArrow             string
	IconBullet            string
	IconNote              string
	IconPlan              string
	IconChat              string
	IconOneshot           string
	IconInteractiveAgent  string
	IconHeadlessAgent     string
	IconShell             string
	IconStatusCompleted   string
	IconStatusRunning     string
	IconStatusFailed      string
	IconStatusBlocked     string
	IconStatusNeedsReview string
	IconStatusPendingUser string
	IconStatusHold        string
	IconStatusTodo        string
	IconStatusAbandoned   string
	IconStatusInterrupted string
)

// init function determines which icon set to use
func init() {
	useASCII := false

	// 1. Check environment variable first
	if os.Getenv("GROVE_ICONS") == "ascii" {
		useASCII = true
	} else {
		// 2. Check config file
		cfg, err := config.LoadDefault()
		if err == nil && cfg.TUI != nil && cfg.TUI.Icons == "ascii" {
			useASCII = true
		}
	}

	if useASCII {
		// Load ASCII icons
		IconTree = asciiIconTree
		IconProject = asciiIconProject
		IconRepo = asciiIconRepo
		IconWorktree = asciiIconWorktree
		IconEcosystem = asciiIconEcosystem
		IconGitBranch = asciiIconGitBranch
		IconSuccess = asciiIconSuccess
		IconError = asciiIconError
		IconWarning = asciiIconWarning
		IconInfo = asciiIconInfo
		IconRunning = asciiIconRunning
		IconPending = asciiIconPending
		IconSelect = asciiIconSelect
		IconArrow = asciiIconArrow
		IconBullet = asciiIconBullet
		IconNote = asciiIconNote
		IconPlan = asciiIconPlan
		IconChat = asciiIconChat
		IconOneshot = asciiIconOneshot
		IconInteractiveAgent = asciiIconInteractiveAgent
		IconHeadlessAgent = asciiIconHeadlessAgent
		IconShell = asciiIconShell
		IconStatusCompleted = asciiIconStatusCompleted
		IconStatusRunning = asciiIconStatusRunning
		IconStatusFailed = asciiIconStatusFailed
		IconStatusBlocked = asciiIconStatusBlocked
		IconStatusNeedsReview = asciiIconStatusNeedsReview
		IconStatusPendingUser = asciiIconStatusPendingUser
		IconStatusHold = asciiIconStatusHold
		IconStatusTodo = asciiIconStatusTodo
		IconStatusAbandoned = asciiIconStatusAbandoned
		IconStatusInterrupted = asciiIconStatusInterrupted
	} else {
		// Load Nerd Font icons (default)
		IconTree = nerdIconTree
		IconProject = nerdIconProject
		IconRepo = nerdIconRepo
		IconWorktree = nerdIconWorktree
		IconEcosystem = nerdIconEcosystem
		IconGitBranch = nerdIconGitBranch
		IconSuccess = nerdIconSuccess
		IconError = nerdIconError
		IconWarning = nerdIconWarning
		IconInfo = nerdIconInfo
		IconRunning = nerdIconRunning
		IconPending = nerdIconPending
		IconSelect = nerdIconSelect
		IconArrow = nerdIconArrow
		IconBullet = nerdIconBullet
		IconNote = nerdIconNote
		IconPlan = nerdIconPlan
		IconChat = nerdIconChat
		IconOneshot = nerdIconOneshot
		IconInteractiveAgent = nerdIconInteractiveAgent
		IconHeadlessAgent = nerdIconHeadlessAgent
		IconShell = nerdIconShell
		IconStatusCompleted = nerdIconStatusCompleted
		IconStatusRunning = nerdIconStatusRunning
		IconStatusFailed = nerdIconStatusFailed
		IconStatusBlocked = nerdIconStatusBlocked
		IconStatusNeedsReview = nerdIconStatusNeedsReview
		IconStatusPendingUser = nerdIconStatusPendingUser
		IconStatusHold = nerdIconStatusHold
		IconStatusTodo = nerdIconStatusTodo
		IconStatusAbandoned = nerdIconStatusAbandoned
		IconStatusInterrupted = nerdIconStatusInterrupted
	}
}
