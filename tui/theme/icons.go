package theme

import (
	"os"

	"github.com/grovetools/core/config"
)

// Nerd Font Icons (Private Constants)
const (
	nerdIconTree              = "" // fa-tree (U+F1BB)
	nerdIconProject           = "" // cod-project (U+EB30)
	nerdIconRepo              = "" // cod-repo (U+EA62)
	nerdIconWorktree          = "" // oct-workflow (U+F52E)
	nerdIconEcosystem         = "" // fa-folder_tree (U+EF81)
	nerdIconGitBranch         = "" // dev-git_branch (U+E725)
	nerdIconSuccess           = "󰄬" // md-check (U+F012C)
	nerdIconError             = "" // cod-error (U+EA87)
	nerdIconWarning           = "" // fa-warning (U+F071)
	nerdIconInfo              = "󰋼" // md-information (U+F02FC)
	nerdIconRunning           = "" // fa-refresh (U+F021)
	nerdIconPending           = "󰦖" // md-progress_clock (U+F0996)
	nerdIconSelect            = "󰱒" // md-checkbox_outline (U+F0C52)
	nerdIconUnselect          = "󰄱" // md-checkbox_blank_outline (U+F0131)
	nerdIconArrow             = "󰁔" // md-arrow_right (U+F0054)
	nerdIconBullet            = "" // oct-dot_fill (U+F444)
	nerdIconNote              = "󰎚" // md-note (U+F039A)
	nerdIconPlan              = "󰠡" // md-floor_plan (U+F0821)
	nerdIconChat              = "󰭹" // md-chat (U+F0B79)
	nerdIconOneshot           = "" // fa-bullseye (U+F140)
	nerdIconInteractiveAgent  = "" // fa-robot (U+EE0D)
	nerdIconHeadlessAgent     = "󰭆" // md-robot_industrial (U+F0B46)
	nerdIconShell             = "" // seti-shell (U+E691)
	nerdIconStatusCompleted   = "󰄳" // md-checkbox_marked_circle (U+F0133)
	nerdIconStatusRunning     = "󰔟" // md-timer_sand (U+F051F)
	nerdIconStatusFailed      = "" // oct-x (U+F467)
	nerdIconStatusBlocked     = "" // oct-blocked (U+F479)
	nerdIconStatusNeedsReview = "" // oct-code_review (U+F4AF)
	nerdIconStatusPendingUser = "󰭻" // md-chat_processing (U+F0B7B)
	nerdIconStatusHold        = "󰏧" // md-pause_octagon (U+F03E7)
	nerdIconStatusTodo        = "󰄱" // md-checkbox_blank_outline (U+F0131)
	nerdIconStatusAbandoned   = "󰩹" // md-trash_can (U+F0A79)
	nerdIconStatusInterrupted = "" // pom-external_interruption (U+E00A)

	nerdIconArchive        = "󰀼" // md-archive (U+F003C)
	nerdIconArrowLeft      = "󰁍" // md-arrow_left (U+F004D)
	nerdIconArrowLeftBold  = "󰜱" // md-arrow_left_bold (U+F0731)
	nerdIconArrowRightBold = "󰜴" // md-arrow_right_bold (U+F0734)
	nerdIconFilter         = "󱣬" // md-filter_check (U+F18EC)
	nerdIconSave           = "󰉉" // md-floppy (U+F0249)
	nerdIconSelectAll      = "󰒆" // md-select_all (U+F0486)
	nerdIconAudited        = "󰳈" // md-shield_check_outline (U+F0CC8)

	nerdIconPullRequest           = "" // cod-git_pull_request (U+EA64)
	nerdIconPullRequestClosed     = "" // cod-git_pull_request_closed (U+EBDA)
	nerdIconPullRequestCreate     = "" // cod-git_pull_request_create (U+EBBC)
	nerdIconPullRequestDraft      = "" // cod-git_pull_request_draft (U+EBDB)
	nerdIconPullRequestNewChanges = "" // cod-git_pull_request_new_changes (U+EC0C)
	nerdIconGithubAction          = "" // cod-github_action (U+EAFF)
	nerdIconGitCompare            = "" // dev-git_compare (U+E728)
	nerdIconDiff                  = "" // cod-diff (U+EAE1)
	nerdIconGit                   = "󰊢" // md-git (U+F02A2)
	nerdIconGitStaged             = "" // cod-pass (U+EBA4)
	nerdIconGitModified           = "" // cod-diff_modified (U+EADE)
	nerdIconGitUntracked          = "" // cod-diff_added (U+EADC)
	nerdIconGitDeleted            = "" // cod-diff_removed (U+EADF)
	nerdIconGitRenamed            = "" // cod-diff_renamed (U+EAE0)
	nerdIconGitPartiallyStaged    = "󰦒" // md-plus_minus (U+F0992)
	nerdIconInbox                 = "󰚇" // md-inbox (U+F0687)
	nerdIconLightbulb             = "󰌵" // md-lightbulb (U+F0335)
	nerdIconChevron               = "󰬪" // md-chevron_right_circle (U+F0B2A)
	nerdIconMerge                 = "󰽜" // md-merge (U+F0F5C)
	nerdIconHome                  = "󰋜" // md-home (U+F02DC)
	nerdIconRobot                 = "󰚩" // md-robot (U+F06A9)
	nerdIconTrophy                = "" // fa-trophy (U+F091)
	nerdIconChart                 = "󰄨" // md-chart_bar (U+F0128)
	nerdIconSnowflake             = "" // fa-snowflake (U+F2DC)
	nerdIconClock                 = "󰥔" // md-clock (U+F0954)
	nerdIconMoney                 = "" // fa-sack_dollar (U+EF8D)
	nerdIconSync                  = "󰓦" // md-sync (U+F04E6)
	nerdIconHelp                  = "󰋗" // md-help_circle (U+F02D7)
	nerdIconBuild                 = "󰣪" // md-hammer (U+F08EA)
	nerdIconStop                  = "󰙦" // md-stop_circle (U+F0666)
	nerdIconEarth                 = "󰇧" // md-earth (U+F01E7)

	nerdIconArrowDown       = "󰁅" // md-arrow_down (U+F0045)
	nerdIconArrowDownBold   = "󰜮" // md-arrow_down_bold (U+F072E)
	nerdIconArrowUp         = "󰁝" // md-arrow_up (U+F005D)
	nerdIconArrowUpBold     = "󰜷" // md-arrow_up_bold (U+F0737)
	nerdIconArrowUpDownBold = "󰹺" // md-arrow_up_down_bold (U+F0E7A)

	nerdIconChecklist      = "" // cod-checklist (U+EAB3)
	nerdIconArchitecture   = "" // fa-building_columns (U+F19C)
	nerdIconCalendar       = "󰃭" // md-calendar (U+F00ED)
	nerdIconChatQuestion   = "󱜸" // md-chat_question (U+F1738)
	nerdIconClockFast      = "󰅒" // md-clock_fast (U+F0152)
	nerdIconLoading        = "󰝲" // md-loading (U+F0772)
	nerdIconRss            = "󰑫" // md-rss (U+F046B)
	nerdIconSchool         = "󰑴" // md-school (U+F0474)
	nerdIconIssueClosed    = "" // oct-issue_closed (U+F41D)
	nerdIconIssueOpened    = "" // oct-issue_opened (U+F41B)
	nerdIconNoteCurrent    = "󰅒" // md-clock_fast (U+F0152)
	nerdIconNoteIssues     = "" // oct-issue_opened (U+F41B)
	nerdIconNoteInbox      = "󰚇" // md-inbox (U+F0687)
	nerdIconNoteCompleted  = "󰄳" // md-checkbox_marked_circle (U+F0133)
	nerdIconNoteReview     = "" // oct-code_review (U+F4AF)
	nerdIconNoteInProgress = "󰔟" // md-timer_sand (U+F051F)

	nerdIconSparkle  = "" // cod-sparkle (U+EC10)
	nerdIconCode     = "" // fa-code (U+F121)
	nerdIconNotebook = "󰠮" // md-notebook (U+F082E)
	nerdIconDocs     = "󰡯" // md-file_question (U+F086F)
	nerdIconGear     = "󰒓" // md-cog (U+F0493)

	nerdIconFileTree      = "󰙅" // md-file_tree (U+F0645)
	nerdIconPineTreeBox   = "󰐆" // md-pine_tree_box (U+F0406)
	nerdIconViewDashboard = "󰕮" // md-view_dashboard (U+F056E)

	nerdIconFolderMinus            = "" // fa-folder_minus (U+EEC6)
	nerdIconFolderOpen             = "" // fa-folder_open (U+F07C)
	nerdIconFolderOpenO            = "" // fa-folder_open_o (U+F115)
	nerdIconFolderPlus             = "" // fa-folder_plus (U+EEC7)
	nerdIconFolderTree             = "" // fa-folder_tree (U+EF81)
	nerdIconFile                   = "󰈔" // md-file (U+F0214)
	nerdIconFileCancel             = "󰷆" // md-file_cancel (U+F0DC6)
	nerdIconFileKey                = "󱆄" // md-file_key (U+F1184)
	nerdIconFileLock               = "󰈡" // md-file_lock (U+F0221)
	nerdIconFilePlus               = "󰝒" // md-file_plus (U+F0752)
	nerdIconFish                   = "󰈺" // md-fish (U+F023A)
	nerdIconFire                   = "󰈸" // md-fire (U+F0238)
	nerdIconFolder                 = "󰉋" // md-folder (U+F024B)
	nerdIconFolderCheck            = "󱥾" // md-folder_check (U+F197E)
	nerdIconFolderEye              = "󱞊" // md-folder_eye (U+F178A)
	nerdIconFolderLock             = "󰉐" // md-folder_lock (U+F0250)
	nerdIconFolderMultiple         = "󰉓" // md-folder_multiple (U+F0253)
	nerdIconFolderMultiplePlus     = "󱑾" // md-folder_multiple_plus (U+F147E)
	nerdIconFolderOff              = "󱧸" // md-folder_off (U+F19F8)
	nerdIconFolderOpenMd           = "󰝰" // md-folder_open (U+F0770)
	nerdIconFolderPlusMd           = "󰉗" // md-folder_plus (U+F0257)
	nerdIconFolderQuestion         = "󱧊" // md-folder_question (U+F19CA)
	nerdIconFolderRemove           = "󰉘" // md-folder_remove (U+F0258)
	nerdIconFolderSearch           = "󰥨" // md-folder_search (U+F0968)
	nerdIconFolderStar             = "󰚝" // md-folder_star (U+F069D)
	nerdIconFolderSync             = "󰴋" // md-folder_sync (U+F0D0B)
	nerdIconFileDirectoryFill      = "" // oct-file_directory_fill (U+F4D3)
	nerdIconDebug                  = "" // cod-debug (U+EAD8)
	nerdIconDebugConsole           = "" // cod-debug_console (U+EB9B)
	nerdIconDebugStart             = "" // cod-debug_start (U+EAD3)
	nerdIconPass                   = "" // cod-pass (U+EBA4)
	nerdIconScriptText             = "󰯂" // md-script_text (U+F0BC2)
	nerdIconTestTube               = "󰙨" // md-test_tube (U+F0668)
	nerdIconNumeric1CircleOutline  = "󰲡" // md-numeric_1_circle_outline (U+F0CA1)
	nerdIconNumeric2CircleOutline  = "󰲣" // md-numeric_2_circle_outline (U+F0CA3)
	nerdIconNumeric3CircleOutline  = "󰲥" // md-numeric_3_circle_outline (U+F0CA5)
	nerdIconNumeric4CircleOutline  = "󰲧" // md-numeric_4_circle_outline (U+F0CA7)
	nerdIconNumeric5CircleOutline  = "󰲩" // md-numeric_5_circle_outline (U+F0CA9)
	nerdIconNumeric6CircleOutline  = "󰲫" // md-numeric_6_circle_outline (U+F0CAB)
	nerdIconNumeric7CircleOutline  = "󰲭" // md-numeric_7_circle_outline (U+F0CAD)
	nerdIconNumeric8CircleOutline  = "󰲯" // md-numeric_8_circle_outline (U+F0CAF)
	nerdIconNumeric9CircleOutline  = "󰲱" // md-numeric_9_circle_outline (U+F0CB1)
	nerdIconNumeric10CircleOutline = "󰿭" // md-numeric_10_circle_outline (U+F0FED)

	nerdIconStatusAlpha = "α" // Greek lowercase alpha (U+03B1)
	nerdIconStatusBeta  = "β" // Greek lowercase beta (U+03B2)
)

// ASCII Fallback Icons (Private Constants)
const (
	asciiIconTree              = "[T]" // Tree
	asciiIconProject           = "◆"
	asciiIconRepo              = "●"
	asciiIconWorktree          = "⑂"
	asciiIconEcosystem         = "◆"
	asciiIconGitBranch         = "⎇"
	asciiIconSuccess           = "*"
	asciiIconError             = "x"
	asciiIconWarning           = "!"
	asciiIconInfo              = "ℹ"
	asciiIconRunning           = "◐"
	asciiIconPending           = "…"
	asciiIconSelect            = "▶"
	asciiIconUnselect          = "○" // Unselect
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
	asciiIconStatusFailed      = "x"
	asciiIconStatusBlocked     = "[X]" // Blocked
	asciiIconStatusNeedsReview = "[?]"
	asciiIconStatusPendingUser = "○"
	asciiIconStatusHold        = "[H]"
	asciiIconStatusTodo        = "○"
	asciiIconStatusAbandoned   = "[D]" // Abandoned
	asciiIconStatusInterrupted = "⊗"

	asciiIconArchive        = "[A]" // Archive
	asciiIconArrowLeft      = "←"   // Arrow left
	asciiIconArrowLeftBold  = "<="  //  Arrow left bold
	asciiIconArrowRightBold = "=>"  //  Arrow right bold
	asciiIconFilter         = "⊲"   // Filter
	asciiIconSave           = "[S]" // Save
	asciiIconSelectAll      = "[*]" //  Select all
	asciiIconAudited        = "*"   // Audited

	asciiIconPullRequest           = "PR"    // Pull request
	asciiIconPullRequestClosed     = "[x]PR" // PR closed
	asciiIconPullRequestCreate     = "[+]PR" // PR create
	asciiIconPullRequestDraft      = "[~]PR" // PR draft
	asciiIconPullRequestNewChanges = "[!]PR" // PR new changes
	asciiIconGithubAction          = "[A]"   // GitHub action
	asciiIconGitCompare            = "<>"    // Git compare
	asciiIconDiff                  = "[±]"   // Diff
	asciiIconGit                   = "git"   // Git
	asciiIconGitStaged             = "*"     // Staged/ready to commit
	asciiIconGitModified           = "M"     // Modified but unstaged
	asciiIconGitUntracked          = "?"     // New untracked file
	asciiIconGitDeleted            = "D"     // Deleted file
	asciiIconGitRenamed            = "R"     // Renamed file
	asciiIconGitPartiallyStaged    = "±"     // Partially staged
	asciiIconInbox                 = "[I]"   //  Inbox
	asciiIconLightbulb             = "[*]"   //  Lightbulb
	asciiIconChevron               = ">"     //  Chevron/prompt
	asciiIconMerge                 = "><"    // Merge
	asciiIconHome                  = "[H]"   // Home
	asciiIconRobot                 = "[R]"   // Robot
	asciiIconTrophy                = "[T]"   // Trophy
	asciiIconChart                 = "[C]"   // Chart/Statistics
	asciiIconSnowflake             = "[*]"   // Snowflake
	asciiIconClock                 = "[C]"   // Clock
	asciiIconMoney                 = "[$]"   // Money
	asciiIconSync                  = "[~]"   // Sync
	asciiIconHelp                  = "[?]"   // Help
	asciiIconBuild                 = "[B]"   // Build/Hammer
	asciiIconStop                  = "[S]"   // Stop
	asciiIconEarth                 = "⨁"     // Earth

	asciiIconArrowDown       = "↓"  // Arrow down
	asciiIconArrowDownBold   = "vv" // Arrow down bold
	asciiIconArrowUp         = "↑"  // Arrow up
	asciiIconArrowUpBold     = "^^" // Arrow up bold
	asciiIconArrowUpDownBold = "<>" // Arrow up/down bold

	asciiIconChecklist      = "[]"  // Checklist/todo
	asciiIconArchitecture   = "[A]" // Architecture
	asciiIconCalendar       = "[C]" // Calendar/daily note
	asciiIconChatQuestion   = "[?]" // Chat question/prompt
	asciiIconClockFast      = "[Q]" // Quick note
	asciiIconLoading        = "..." // Loading
	asciiIconRss            = "[R]" // RSS/blog
	asciiIconSchool         = "[L]" // School/learn
	asciiIconIssueClosed    = "[x]" // Issue closed
	asciiIconIssueOpened    = "[!]" // Issue opened
	asciiIconNoteCurrent    = "[>]" // Current note
	asciiIconNoteIssues     = "[!]" // Issues notes
	asciiIconNoteInbox      = "[I]" // Inbox notes
	asciiIconNoteCompleted  = "[*]" // Completed notes
	asciiIconNoteReview     = "[R]" // Review notes
	asciiIconNoteInProgress = "[~]" // In-progress notes

	asciiIconSparkle  = "[+]" // New
	asciiIconCode     = "</>" // Code
	asciiIconNotebook = "[N]" // Notebook
	asciiIconDocs     = "[D]" // Docs
	asciiIconGear     = "[G]" // Gear/Settings

	asciiIconFileTree      = "[F]"   // File tree
	asciiIconFish          = "[><>]" // Fish
	asciiIconPineTreeBox   = "[P]"   // Pine tree box
	asciiIconViewDashboard = "[W]"   // View dashboard/workspace

	asciiIconFolderMinus            = "[-]"  // Folder minus
	asciiIconFolderOpen             = "[o]"  // Folder open
	asciiIconFolderOpenO            = "(o)"  // Folder open outline
	asciiIconFolderPlus             = "[+]"  // Folder plus
	asciiIconFolderTree             = "[T]"  // Folder tree
	asciiIconFile                   = "[ ]"  // File
	asciiIconFileCancel             = "[x]"  // File cancel
	asciiIconFileKey                = "[K]"  // File key
	asciiIconFileLock               = "[#]"  // File lock
	asciiIconFilePlus               = "[+]"  // File plus
	asciiIconFire                   = "[*]"  // Fire
	asciiIconFolder                 = "[ ]"  // Folder
	asciiIconFolderCheck            = "[*]"  // Folder check
	asciiIconFolderEye              = "[E]"  // Folder eye
	asciiIconFolderLock             = "[#]"  // Folder lock
	asciiIconFolderMultiple         = "[=]"  // Folder multiple
	asciiIconFolderMultiplePlus     = "[+]"  // Folder multiple plus
	asciiIconFolderOff              = "[-]"  // Folder off
	asciiIconFolderOpenMd           = "(o)"  // Folder open (md)
	asciiIconFolderPlusMd           = "[+]"  // Folder plus (md)
	asciiIconFolderQuestion         = "[?]"  // Folder question
	asciiIconFolderRemove           = "[-]"  // Folder remove
	asciiIconFolderSearch           = "[/]"  // Folder search
	asciiIconFolderStar             = "[*]"  // Folder star
	asciiIconFolderSync             = "[~]"  // Folder sync
	asciiIconFileDirectoryFill      = "[D]"  // File directory fill
	asciiIconDebug                  = "[D]"  // Debug
	asciiIconDebugConsole           = "[D]"  // DebugConsole
	asciiIconDebugStart             = "[D]"  // DebugStart
	asciiIconPass                   = "[P]"  // Pass
	asciiIconScriptText             = "[S]"  // ScriptText
	asciiIconTestTube               = "[T]"  // TestTube
	asciiIconNumeric1CircleOutline  = "(1)"  // Numeric1CircleOutline
	asciiIconNumeric2CircleOutline  = "(2)"  // Numeric2CircleOutline
	asciiIconNumeric3CircleOutline  = "(3)"  // Numeric3CircleOutline
	asciiIconNumeric4CircleOutline  = "(4)"  // Numeric4CircleOutline
	asciiIconNumeric5CircleOutline  = "(5)"  // Numeric5CircleOutline
	asciiIconNumeric6CircleOutline  = "(6)"  // Numeric6CircleOutline
	asciiIconNumeric7CircleOutline  = "(7)"  // Numeric7CircleOutline
	asciiIconNumeric8CircleOutline  = "(8)"  // Numeric8CircleOutline
	asciiIconNumeric9CircleOutline  = "(9)"  // Numeric9CircleOutline
	asciiIconNumeric10CircleOutline = "(10)" // Numeric10CircleOutline

	asciiIconStatusAlpha = "[A]" // Alpha status
	asciiIconStatusBeta  = "[B]" // Beta status
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
	IconUnselect          string
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

	IconArchive        string
	IconArrowLeft      string
	IconArrowLeftBold  string
	IconArrowRightBold string
	IconFilter         string
	IconSave           string
	IconSelectAll      string
	IconAudited        string

	IconPullRequest           string
	IconPullRequestClosed     string
	IconPullRequestCreate     string
	IconPullRequestDraft      string
	IconPullRequestNewChanges string
	IconGithubAction          string
	IconGitCompare            string
	IconDiff                  string
	IconGit                   string
	IconGitStaged             string
	IconGitModified           string
	IconGitUntracked          string
	IconGitDeleted            string
	IconGitRenamed            string
	IconGitPartiallyStaged    string
	IconInbox                 string
	IconLightbulb             string
	IconChevron               string
	IconMerge                 string
	IconHome                  string
	IconRobot                 string
	IconTrophy                string
	IconChart                 string
	IconSnowflake             string
	IconClock                 string
	IconMoney                 string
	IconSync                  string
	IconHelp                  string
	IconBuild                 string
	IconStop                  string
	IconEarth                 string

	IconArrowDown       string
	IconArrowDownBold   string
	IconArrowUp         string
	IconArrowUpBold     string
	IconArrowUpDownBold string

	IconChecklist      string
	IconArchitecture   string
	IconCalendar       string
	IconChatQuestion   string
	IconClockFast      string
	IconLoading        string
	IconRss            string
	IconSchool         string
	IconIssueClosed    string
	IconIssueOpened    string
	IconNoteCurrent    string
	IconNoteIssues     string
	IconNoteInbox      string
	IconNoteCompleted  string
	IconNoteReview     string
	IconNoteInProgress string

	IconSparkle  string
	IconCode     string
	IconNotebook string
	IconDocs     string
	IconGear     string

	IconFileTree      string
	IconFish          string
	IconPineTreeBox   string
	IconViewDashboard string

	IconFolderMinus            string
	IconFolderOpen             string
	IconFolderOpenO            string
	IconFolderPlus             string
	IconFolderTree             string
	IconFile                   string
	IconFileCancel             string
	IconFileKey                string
	IconFileLock               string
	IconFilePlus               string
	IconFire                   string
	IconFolder                 string
	IconFolderCheck            string
	IconFolderEye              string
	IconFolderLock             string
	IconFolderMultiple         string
	IconFolderMultiplePlus     string
	IconFolderOff              string
	IconFolderOpenMd           string
	IconFolderPlusMd           string
	IconFolderQuestion         string
	IconFolderRemove           string
	IconFolderSearch           string
	IconFolderStar             string
	IconFolderSync             string
	IconFileDirectoryFill      string
	IconDebug                  string
	IconDebugConsole           string
	IconDebugStart             string
	IconPass                   string
	IconScriptText             string
	IconTestTube               string
	IconNumeric1CircleOutline  string
	IconNumeric2CircleOutline  string
	IconNumeric3CircleOutline  string
	IconNumeric4CircleOutline  string
	IconNumeric5CircleOutline  string
	IconNumeric6CircleOutline  string
	IconNumeric7CircleOutline  string
	IconNumeric8CircleOutline  string
	IconNumeric9CircleOutline  string
	IconNumeric10CircleOutline string

	IconStatusAlpha string
	IconStatusBeta  string
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
		IconUnselect = asciiIconUnselect
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
		IconPullRequest = asciiIconPullRequest
		IconPullRequestClosed = asciiIconPullRequestClosed
		IconPullRequestCreate = asciiIconPullRequestCreate
		IconPullRequestDraft = asciiIconPullRequestDraft
		IconPullRequestNewChanges = asciiIconPullRequestNewChanges
		IconGithubAction = asciiIconGithubAction
		IconDiff = asciiIconDiff
		IconGitCompare = asciiIconGitCompare
		IconGit = asciiIconGit
		IconGitStaged = asciiIconGitStaged
		IconGitModified = asciiIconGitModified
		IconGitUntracked = asciiIconGitUntracked
		IconGitDeleted = asciiIconGitDeleted
		IconGitRenamed = asciiIconGitRenamed
		IconGitPartiallyStaged = asciiIconGitPartiallyStaged
		IconInbox = asciiIconInbox
		IconLightbulb = asciiIconLightbulb
		IconChevron = asciiIconChevron
		IconMerge = asciiIconMerge
		IconHome = asciiIconHome
		IconRobot = asciiIconRobot
		IconTrophy = asciiIconTrophy
		IconChart = asciiIconChart
		IconSnowflake = asciiIconSnowflake
		IconClock = asciiIconClock
		IconMoney = asciiIconMoney
		IconSync = asciiIconSync
		IconHelp = asciiIconHelp
		IconBuild = asciiIconBuild
		IconStop = asciiIconStop
		IconEarth = asciiIconEarth
		IconArrowDown = asciiIconArrowDown
		IconArrowDownBold = asciiIconArrowDownBold
		IconArrowUp = asciiIconArrowUp
		IconArrowUpBold = asciiIconArrowUpBold
		IconArrowUpDownBold = asciiIconArrowUpDownBold
		IconChecklist = asciiIconChecklist
		IconArchitecture = asciiIconArchitecture
		IconCalendar = asciiIconCalendar
		IconChatQuestion = asciiIconChatQuestion
		IconClockFast = asciiIconClockFast
		IconLoading = asciiIconLoading
		IconRss = asciiIconRss
		IconSchool = asciiIconSchool
		IconIssueClosed = asciiIconIssueClosed
		IconIssueOpened = asciiIconIssueOpened
		IconNoteCurrent = asciiIconNoteCurrent
		IconNoteIssues = asciiIconNoteIssues
		IconNoteInbox = asciiIconNoteInbox
		IconNoteCompleted = asciiIconNoteCompleted
		IconNoteReview = asciiIconNoteReview
		IconNoteInProgress = asciiIconNoteInProgress
		IconSparkle = asciiIconSparkle
		IconCode = asciiIconCode
		IconNotebook = asciiIconNotebook
		IconDocs = asciiIconDocs
		IconGear = asciiIconGear
		IconFileTree = asciiIconFileTree
		IconFish = asciiIconFish
		IconPineTreeBox = asciiIconPineTreeBox
		IconViewDashboard = asciiIconViewDashboard
		IconFolderMinus = asciiIconFolderMinus
		IconFolderOpen = asciiIconFolderOpen
		IconFolderOpenO = asciiIconFolderOpenO
		IconFolderPlus = asciiIconFolderPlus
		IconFolderTree = asciiIconFolderTree
		IconFile = asciiIconFile
		IconFileCancel = asciiIconFileCancel
		IconFileKey = asciiIconFileKey
		IconFileLock = asciiIconFileLock
		IconFilePlus = asciiIconFilePlus
		IconFire = asciiIconFire
		IconFolder = asciiIconFolder
		IconFolderCheck = asciiIconFolderCheck
		IconFolderEye = asciiIconFolderEye
		IconFolderLock = asciiIconFolderLock
		IconFolderMultiple = asciiIconFolderMultiple
		IconFolderMultiplePlus = asciiIconFolderMultiplePlus
		IconFolderOff = asciiIconFolderOff
		IconFolderOpenMd = asciiIconFolderOpenMd
		IconFolderPlusMd = asciiIconFolderPlusMd
		IconFolderQuestion = asciiIconFolderQuestion
		IconFolderRemove = asciiIconFolderRemove
		IconFolderSearch = asciiIconFolderSearch
		IconFolderStar = asciiIconFolderStar
		IconFolderSync = asciiIconFolderSync
		IconFileDirectoryFill = asciiIconFileDirectoryFill
		IconDebug = asciiIconDebug
		IconDebugConsole = asciiIconDebugConsole
		IconDebugStart = asciiIconDebugStart
		IconPass = asciiIconPass
		IconScriptText = asciiIconScriptText
		IconTestTube = asciiIconTestTube
		IconNumeric1CircleOutline = asciiIconNumeric1CircleOutline
		IconNumeric2CircleOutline = asciiIconNumeric2CircleOutline
		IconNumeric3CircleOutline = asciiIconNumeric3CircleOutline
		IconNumeric4CircleOutline = asciiIconNumeric4CircleOutline
		IconNumeric5CircleOutline = asciiIconNumeric5CircleOutline
		IconNumeric6CircleOutline = asciiIconNumeric6CircleOutline
		IconNumeric7CircleOutline = asciiIconNumeric7CircleOutline
		IconNumeric8CircleOutline = asciiIconNumeric8CircleOutline
		IconNumeric9CircleOutline = asciiIconNumeric9CircleOutline
		IconNumeric10CircleOutline = asciiIconNumeric10CircleOutline
		IconStatusAlpha = asciiIconStatusAlpha
		IconStatusBeta = asciiIconStatusBeta
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
		IconUnselect = nerdIconUnselect
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
		IconPullRequest = nerdIconPullRequest
		IconPullRequestClosed = nerdIconPullRequestClosed
		IconPullRequestCreate = nerdIconPullRequestCreate
		IconPullRequestDraft = nerdIconPullRequestDraft
		IconPullRequestNewChanges = nerdIconPullRequestNewChanges
		IconDiff = nerdIconDiff
		IconGithubAction = nerdIconGithubAction
		IconGitCompare = nerdIconGitCompare
		IconGit = nerdIconGit
		IconGitStaged = nerdIconGitStaged
		IconGitModified = nerdIconGitModified
		IconGitUntracked = nerdIconGitUntracked
		IconGitDeleted = nerdIconGitDeleted
		IconGitRenamed = nerdIconGitRenamed
		IconGitPartiallyStaged = nerdIconGitPartiallyStaged
		IconInbox = nerdIconInbox
		IconLightbulb = nerdIconLightbulb
		IconChevron = nerdIconChevron
		IconMerge = nerdIconMerge
		IconHome = nerdIconHome
		IconRobot = nerdIconRobot
		IconTrophy = nerdIconTrophy
		IconChart = nerdIconChart
		IconSnowflake = nerdIconSnowflake
		IconClock = nerdIconClock
		IconMoney = nerdIconMoney
		IconSync = nerdIconSync
		IconHelp = nerdIconHelp
		IconBuild = nerdIconBuild
		IconStop = nerdIconStop
		IconEarth = nerdIconEarth
		IconArrowDown = nerdIconArrowDown
		IconArrowDownBold = nerdIconArrowDownBold
		IconArrowUp = nerdIconArrowUp
		IconArrowUpBold = nerdIconArrowUpBold
		IconArrowUpDownBold = nerdIconArrowUpDownBold
		IconChecklist = nerdIconChecklist
		IconArchitecture = nerdIconArchitecture
		IconCalendar = nerdIconCalendar
		IconChatQuestion = nerdIconChatQuestion
		IconClockFast = nerdIconClockFast
		IconLoading = nerdIconLoading
		IconRss = nerdIconRss
		IconSchool = nerdIconSchool
		IconIssueClosed = nerdIconIssueClosed
		IconIssueOpened = nerdIconIssueOpened
		IconNoteCurrent = nerdIconNoteCurrent
		IconNoteIssues = nerdIconNoteIssues
		IconNoteInbox = nerdIconNoteInbox
		IconNoteCompleted = nerdIconNoteCompleted
		IconNoteReview = nerdIconNoteReview
		IconNoteInProgress = nerdIconNoteInProgress
		IconSparkle = nerdIconSparkle
		IconCode = nerdIconCode
		IconNotebook = nerdIconNotebook
		IconDocs = nerdIconDocs
		IconGear = nerdIconGear
		IconFileTree = nerdIconFileTree
		IconFish = nerdIconFish
		IconPineTreeBox = nerdIconPineTreeBox
		IconViewDashboard = nerdIconViewDashboard
		IconFolderMinus = nerdIconFolderMinus
		IconFolderOpen = nerdIconFolderOpen
		IconFolderOpenO = nerdIconFolderOpenO
		IconFolderPlus = nerdIconFolderPlus
		IconFolderTree = nerdIconFolderTree
		IconFile = nerdIconFile
		IconFileCancel = nerdIconFileCancel
		IconFileKey = nerdIconFileKey
		IconFileLock = nerdIconFileLock
		IconFilePlus = nerdIconFilePlus
		IconFire = nerdIconFire
		IconFolder = nerdIconFolder
		IconFolderCheck = nerdIconFolderCheck
		IconFolderEye = nerdIconFolderEye
		IconFolderLock = nerdIconFolderLock
		IconFolderMultiple = nerdIconFolderMultiple
		IconFolderMultiplePlus = nerdIconFolderMultiplePlus
		IconFolderOff = nerdIconFolderOff
		IconFolderOpenMd = nerdIconFolderOpenMd
		IconFolderPlusMd = nerdIconFolderPlusMd
		IconFolderQuestion = nerdIconFolderQuestion
		IconFolderRemove = nerdIconFolderRemove
		IconFolderSearch = nerdIconFolderSearch
		IconFolderStar = nerdIconFolderStar
		IconFolderSync = nerdIconFolderSync
		IconFileDirectoryFill = nerdIconFileDirectoryFill
		IconDebug = nerdIconDebug
		IconDebugConsole = nerdIconDebugConsole
		IconDebugStart = nerdIconDebugStart
		IconPass = nerdIconPass
		IconScriptText = nerdIconScriptText
		IconTestTube = nerdIconTestTube
		IconNumeric1CircleOutline = nerdIconNumeric1CircleOutline
		IconNumeric2CircleOutline = nerdIconNumeric2CircleOutline
		IconNumeric3CircleOutline = nerdIconNumeric3CircleOutline
		IconNumeric4CircleOutline = nerdIconNumeric4CircleOutline
		IconNumeric5CircleOutline = nerdIconNumeric5CircleOutline
		IconNumeric6CircleOutline = nerdIconNumeric6CircleOutline
		IconNumeric7CircleOutline = nerdIconNumeric7CircleOutline
		IconNumeric8CircleOutline = nerdIconNumeric8CircleOutline
		IconNumeric9CircleOutline = nerdIconNumeric9CircleOutline
		IconNumeric10CircleOutline = nerdIconNumeric10CircleOutline
		IconStatusAlpha = nerdIconStatusAlpha
		IconStatusBeta = nerdIconStatusBeta
	}
}
