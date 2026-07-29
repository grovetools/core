package pager

// The three host messages the pager itself acts on.
//
// These used to be read from core/tui/embed, which aliases tuimux/embed —
// three message cases that cost the pager the entire workspace, plan and git
// package graph and made a drawer of pages unusable outside a grove binary.
// The definitions live here now and tuimux/embed aliases them, so a host
// sending embed.FocusMsg and a page matching pager.FocusMsg are still matching
// the same type: `type FocusMsg = pager.FocusMsg` is an alias, not a copy.
//
// Everything else on the embed wire stays in tuimux/embed. Only what the pager
// switches on is here.

// FocusMsg informs a pager that it has gained focus in the host layout. The
// pager forwards it to the active page and then calls that page's Focus.
type FocusMsg struct{}

// BlurMsg informs a pager that it has lost focus in the host layout. The pager
// blurs the active page and forwards the message to it.
type BlurMsg struct{}

// SwitchTabMsg requests that the pager activate a different tab. TabID wins
// when set — it is the stable identifier, resilient to tabs being added,
// removed or reordered — and TabIndex is the positional fallback. A request
// naming an unknown TabID, or an index that is out of range or disabled, is
// ignored.
type SwitchTabMsg struct {
	TabID    string
	TabIndex int
}
