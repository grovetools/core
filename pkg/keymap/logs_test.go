package keymap

import "testing"

func TestLogKeyMapSectionsExposeLogSpecificFilters(t *testing.T) {
	sections := NewLogKeyMap(nil).Sections()

	// Chord canon 60 §10: the display/filter toggles moved into the t…
	// namespace (RULE T) — flat `v` and `E` are gone, chord-only (E4).
	want := map[string]string{
		"tl": "cycle log level",
		"te": "toggle events only",
	}
	for _, section := range sections {
		for _, binding := range section.Bindings {
			for _, pressed := range binding.Keys() {
				if desc, ok := want[pressed]; ok && binding.Help().Desc == desc {
					delete(want, pressed)
				}
			}
		}
	}

	for pressed, desc := range want {
		t.Errorf("log help is missing %q (%s)", pressed, desc)
	}
}

func TestLogKeyMapInfoExportsLogSectionsInsteadOfGenericBaseSections(t *testing.T) {
	info := KeymapInfo()
	for _, section := range info.Sections {
		for _, binding := range section.Bindings {
			if binding.Description == "cycle log level" {
				return
			}
		}
	}
	t.Fatal("exported log keymap is missing cycle log level")
}
