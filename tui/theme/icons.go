package theme

import (
	"os"

	"github.com/mattsolo1/grove-core/config"
)

// Nerd Font Icons (Private Constants)
const (
	nerdIconTree                   = "" // fa-tree (U+F1BB)
	nerdIconProject                = "" // cod-project (U+EB30)
	nerdIconRepo                   = "" // cod-repo (U+EA62)
	nerdIconWorktree               = "" // oct-workflow (U+F52E)
	nerdIconEcosystem              = "" // fa-folder_tree (U+EF81)
	nerdIconGitBranch              = "" // dev-git_branch (U+E725)
	nerdIconSuccess                = "󰄬" // md-check (U+F012C)
	nerdIconError                  = "" // cod-error (U+EA87)
	nerdIconWarning                = "" // fa-warning (U+F071)
	nerdIconInfo                   = "󰋼" // md-information (U+F02FC)
	nerdIconRunning                = "" // fa-refresh (U+F021)
	nerdIconPending                = "󰦖" // md-progress_clock (U+F0996)
	nerdIconSelect                 = "󰱒" // md-checkbox_outline (U+F0C52)
	nerdIconArrow                  = "󰁔" // md-arrow_right (U+F0054)
	nerdIconBullet                 = "" // oct-dot_fill (U+F444)
	nerdIconNote                   = "󰎚" // md-note (U+F039A)
	nerdIconPlan                   = "󰚸" // md-note_multiple (U+F06B8)
	nerdIconChat                   = "󰭹" // md-chat (U+F0B79)
	nerdIconOneshot                = "" // fa-bullseye (U+F140)
	nerdIconInteractiveAgent       = "" // fa-robot (U+EE0D)
	nerdIconHeadlessAgent          = "󰭆" // md-robot_industrial (U+F0B46)
	nerdIconShell                  = "" // seti-shell (U+E691)
	nerdIconStatusCompleted        = "󰄳" // md-checkbox_marked_circle (U+F0133)
	nerdIconStatusRunning          = "󰔟" // md-timer_sand (U+F051F)
	nerdIconStatusFailed           = "" // oct-x (U+F467)
	nerdIconStatusBlocked          = "" // oct-blocked (U+F479)
	nerdIconStatusNeedsReview      = "" // oct-code_review (U+F4AF)
	nerdIconStatusPendingUser      = "󰭻" // md-chat_processing (U+F0B7B)
	nerdIconStatusHold             = "󰏧" // md-pause_octagon (U+F03E7)
	nerdIconStatusTodo             = "󰄱" // md-checkbox_blank_outline (U+F0131)
	nerdIconStatusAbandoned        = "󰩹" // md-trash_can (U+F0A79)
	nerdIconStatusInterrupted      = "" // pom-external_interruption (U+E00A)

	nerdIconArchive                = "󰀼" // md-archive (U+F003C)
	nerdIconArrowLeft              = "󰁍" // md-arrow_left (U+F004D)
	nerdIconArrowLeftBold          = "󰜱" // md-arrow_left_bold (U+F0731)
	nerdIconArrowRightBold         = "󰜴" // md-arrow_right_bold (U+F0734)
	nerdIconFilter                 = "󱣬" // md-filter_check (U+F18EC)
	nerdIconSave                   = "󰉉" // md-floppy (U+F0249)
	nerdIconSelectAll              = "󰒆" // md-select_all (U+F0486)
	nerdIconAudited                = "󰳈" // md-shield_check_outline (U+F0CC8)
)

// ASCII Fallback Icons (Private Constants)
const (
	asciiIconTree                  = "[T]" // Tree
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
	asciiIconStatusBlocked         = "[X]" // Blocked
	asciiIconStatusNeedsReview     = "[?]"
	asciiIconStatusPendingUser = "○"
	asciiIconStatusHold            = "[H]"
	asciiIconStatusTodo        = "○"
	asciiIconStatusAbandoned       = "[D]" // Abandoned
	asciiIconStatusInterrupted = "⊗"

	asciiIconArchive               = "[A]" // Archive
	asciiIconArrowLeft             = "←" // Arrow left
	asciiIconArrowLeftBold         = "<=" //  Arrow left bold
	asciiIconArrowRightBold        = "=>" //  Arrow right bold
	asciiIconFilter                = "⊲" // Filter
	asciiIconSave                  = "[S]" // Save
	asciiIconSelectAll             = "[*]" //  Select all
	asciiIconAudited               = "✓" // Audited
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

	IconArchive                    string
	IconArrowLeft                  string
	IconArrowLeftBold              string
	IconArrowRightBold             string
	IconFilter                     string
	IconSave                       string
	IconSelectAll                  string
	IconAudited                    string
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
		IconArchive = asciiIconArchive
		IconArrowLeft = asciiIconArrowLeft
		IconArrowLeftBold = asciiIconArrowLeftBold
		IconArrowRightBold = asciiIconArrowRightBold
		IconFilter = asciiIconFilter
		IconSave = asciiIconSave
		IconSelectAll = asciiIconSelectAll
		IconAudited = asciiIconAudited
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
		IconArchive = nerdIconArchive
		IconArrowLeft = nerdIconArrowLeft
		IconArrowLeftBold = nerdIconArrowLeftBold
		IconArrowRightBold = nerdIconArrowRightBold
		IconFilter = nerdIconFilter
		IconSave = nerdIconSave
		IconSelectAll = nerdIconSelectAll
		IconAudited = nerdIconAudited
	}
}
