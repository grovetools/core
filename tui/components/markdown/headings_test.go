package markdown

import (
	"reflect"
	"strings"
	"testing"
)

func TestExtractHeadingsV1Syntax(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []Heading
	}{
		{
			name:  "ATX levels indentation and trailing hashes",
			input: "# One\n ## Two ##\n  ### Three###\n   #### Four ####\n##### Five\n###### Six\n# ###\n",
			want: []Heading{
				{Level: 1, Text: "One", Line: 1},
				{Level: 2, Text: "Two", Line: 2},
				{Level: 3, Text: "Three###", Line: 3},
				{Level: 4, Text: "Four", Line: 4},
				{Level: 5, Text: "Five", Line: 5},
				{Level: 6, Text: "Six", Line: 6},
				{Level: 1, Text: "", Line: 7},
			},
		},
		{
			name:  "rejects unsupported heading forms",
			input: "#hashtag\n####### seven\n    # indented code\n> # quoted\nSetext\n===\n",
		},
		{
			name: "skips YAML frontmatter",
			input: strings.Join([]string{
				"---", "title: '# not a heading'", "nested:", "  # still metadata", "---", "# Body",
			}, "\n"),
			want: []Heading{{Level: 1, Text: "Body", Line: 6}},
		},
		{
			name:  "unclosed frontmatter is ordinary content",
			input: "---\n# Visible\n",
			want:  []Heading{{Level: 1, Text: "Visible", Line: 2}},
		},
		{
			name:  "backtick fences suppress headings",
			input: "# Before\n```go\n# fake\n````\n## After\n",
			want: []Heading{
				{Level: 1, Text: "Before", Line: 1},
				{Level: 2, Text: "After", Line: 5},
			},
		},
		{
			name:  "tilde fence ignores backticks inside",
			input: "~~~markdown\n# fake\n```\n## also fake\n~~~\n### Real\n",
			want:  []Heading{{Level: 3, Text: "Real", Line: 6}},
		},
		{
			name:  "fences accept zero through three spaces",
			input: "```\n# fake zero\n```\n ~~~\n# fake one\n ~~~\n  ```\n# fake two\n  ```\n   ~~~\n# fake three\n   ~~~\n# Real\n",
			want:  []Heading{{Level: 1, Text: "Real", Line: 13}},
		},
		{
			name:  "four-space pseudo fences are outside v1",
			input: "    ```\n# Real\n    ```\n",
			want:  []Heading{{Level: 1, Text: "Real", Line: 2}},
		},
		{
			name:  "tab-indented pseudo fences are outside v1",
			input: "\t~~~\n# Real\n\t~~~\n",
			want:  []Heading{{Level: 1, Text: "Real", Line: 2}},
		},
		{
			name:  "CRLF tolerance",
			input: "---\r\ntitle: x\r\n---\r\n  ## Windows ##\r\n",
			want:  []Heading{{Level: 2, Text: "Windows", Line: 4}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractHeadings(strings.Split(tt.input, "\n"))
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ExtractHeadings() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
