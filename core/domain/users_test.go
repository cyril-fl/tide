package domain

import (
	"encoding/json"
	"maps"
	"os"
	"regexp"
	"slices"
	"testing"
)

// The web app lists its themes in themes.ts: an id on one side only is refused by the API or cannot be shown.
func TestThemesMatchApp(t *testing.T) {
	file, err := os.ReadFile("../../web/app/utils/themes.ts")
	if err != nil {
		t.Fatal(err)
	}
	var app []string
	for _, match := range regexp.MustCompile(`(?:\{ id: |own\()'([^']+)'`).FindAllStringSubmatch(string(file), -1) {
		app = append(app, match[1])
	}
	if !slices.Equal(app, Themes) {
		t.Errorf("themes.ts has %v, domain.Themes %v", app, Themes)
	}
}

// The web app names each plan in fr.json and en.json: a plan without its label shows its id.
func TestPlansHaveLabels(t *testing.T) {
	for _, locale := range Locales {
		file, err := os.ReadFile("../../web/i18n/locales/" + locale + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var labels struct {
			Settings struct {
				Plans map[string]string `json:"plans"`
			} `json:"settings"`
		}
		if err := json.Unmarshal(file, &labels); err != nil {
			t.Fatal(err)
		}
		for _, plan := range Plans {
			if labels.Settings.Plans[plan] == "" {
				t.Errorf("%s.json has no settings.plans.%s", locale, plan)
			}
		}
	}
}

// The web app lists the sidebar sections in sidebar.ts: one on the API's side only cannot be shown, on the app's side only is refused.
func TestSidebarSectionsMatchApp(t *testing.T) {
	file, err := os.ReadFile("../../web/app/utils/sidebar.ts")
	if err != nil {
		t.Fatal(err)
	}
	var app []string
	if match := regexp.MustCompile(`sidebarSectionIds = \[([^\]]*)\]`).FindStringSubmatch(string(file)); match != nil {
		for _, id := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(match[1], -1) {
			app = append(app, id[1])
		}
	}
	if !slices.Equal(app, SidebarSectionIDs) {
		t.Errorf("sidebar.ts has %v, domain.SidebarSectionIDs %v", app, SidebarSectionIDs)
	}
}

// The web app puts each feature in a subsection of the settings in features.ts: a feature on one side only is
// either never shown or never unlocked.
func TestFeaturesMatchApp(t *testing.T) {
	file, err := os.ReadFile("../../web/app/utils/features.ts")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)groups = \{(.*?)\n\}`).FindStringSubmatch(string(file))
	if block == nil {
		t.Fatal("no groups in features.ts")
	}
	var app []string
	for _, match := range regexp.MustCompile(`(?m)^  (\w+): '\w+',$`).FindAllStringSubmatch(block[1], -1) {
		app = append(app, match[1])
	}
	slices.Sort(app)
	if names := slices.Sorted(maps.Keys(Features)); !slices.Equal(app, names) {
		t.Errorf("features.ts has %v, domain.Features %v", app, names)
	}
}

func TestUnlocked(t *testing.T) {
	for _, c := range []struct {
		feature, plan string
		want          bool
	}{
		{"noteHistory", "base", false},
		{"noteHistory", "pro", true},
		{"replaceEverywhere", "pro", false},
		{"replaceEverywhere", "ultra", true},
		{"noteHistory", "unknown", false},
		{"graph", "unknown", true},
		{"unknown", "ultra", false},
	} {
		if got := Unlocked(c.feature, c.plan); got != c.want {
			t.Errorf("Unlocked(%q, %q) = %v, want %v", c.feature, c.plan, got, c.want)
		}
	}
}
