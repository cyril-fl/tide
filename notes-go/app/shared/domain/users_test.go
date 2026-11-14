package domain

import (
	"os"
	"regexp"
	"slices"
	"testing"
)

// The web app lists its themes in themes.ts: an id on one side only is refused by the API or cannot be shown.
func TestThemesMatchApp(t *testing.T) {
	file, err := os.ReadFile("../../../../notes-app/app/utils/themes.ts")
	if err != nil {
		t.Fatal(err)
	}
	var app []string
	for _, match := range regexp.MustCompile(`\{ id: '([^']+)'`).FindAllStringSubmatch(string(file), -1) {
		app = append(app, match[1])
	}
	if !slices.Equal(app, Themes) {
		t.Errorf("themes.ts has %v, domain.Themes %v", app, Themes)
	}
}

// The web app orders its plans in features.ts: a plan on one side only is refused by the API or unlocks nothing.
func TestPlansMatchApp(t *testing.T) {
	file, err := os.ReadFile("../../../../notes-app/app/utils/features.ts")
	if err != nil {
		t.Fatal(err)
	}
	var app []string
	if match := regexp.MustCompile(`plans = \[([^\]]*)\]`).FindStringSubmatch(string(file)); match != nil {
		for _, plan := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(match[1], -1) {
			app = append(app, plan[1])
		}
	}
	if !slices.Equal(app, Plans) {
		t.Errorf("features.ts has %v, domain.Plans %v", app, Plans)
	}
}

// The web app lists the sidebar sections in sidebar.ts: one on the API's side only cannot be shown, on the app's side only is refused.
func TestSidebarSectionsMatchApp(t *testing.T) {
	file, err := os.ReadFile("../../../../notes-app/app/utils/sidebar.ts")
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

// The routes of a paid feature need the plan the web app shows it from in features.ts: else one side unlocks it alone.
func TestFeaturesMatchApp(t *testing.T) {
	file, err := os.ReadFile("../../../../notes-app/app/utils/features.ts")
	if err != nil {
		t.Fatal(err)
	}
	app := map[string]string{}
	block := regexp.MustCompile(`(?s)features = \{(.*?)\n\}`).FindStringSubmatch(string(file))
	if block == nil {
		t.Fatal("no features in features.ts")
	}
	for _, match := range regexp.MustCompile(`(?m)^  (\w+): '(\w+)',$`).FindAllStringSubmatch(block[1], -1) {
		app[match[1]] = match[2]
	}
	for feature, plan := range Features {
		if app[feature] != plan {
			t.Errorf("features.ts has %s: %q, domain.Features %q", feature, app[feature], plan)
		}
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
	} {
		if got := Unlocked(c.feature, c.plan); got != c.want {
			t.Errorf("Unlocked(%q, %q) = %v, want %v", c.feature, c.plan, got, c.want)
		}
	}
}
