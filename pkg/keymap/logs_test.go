package keymap

import "testing"

func TestLogKeyMapSectionsExposeLogSpecificFilters(t *testing.T) {
	sections := NewLogKeyMap(nil).Sections()

	want := map[string]string{
		"v": "cycle log level",
		"E": "toggle events only",
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
