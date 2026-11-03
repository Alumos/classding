package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type fakeHomeworkWindows struct {
	items     []HomeworkCard
	suspended bool
}

func (f *fakeHomeworkWindows) update(_ string, items []HomeworkCard, suspended bool) error {
	f.items = items
	f.suspended = suspended
	return nil
}

func (*fakeHomeworkWindows) close() {}
func TestHomeworkWorkAreaLayout(t *testing.T) {
	for _, area := range []struct{ x, y, w, h int }{{0, 0, 1366, 728}, {0, 0, 1920, 1032}, {0, 0, 3840, 2072}, {-1920, -1080, 1920, 1040}, {80, 0, 1840, 1080}} {
		for n := 1; n <= 12; n++ {
			cards := homeworkLayout(area.x, area.y, area.w, area.h, n)
			if len(cards) != n {
				t.Fatalf("missing cards: %d/%d", len(cards), n)
			}
			for i, a := range cards {
				if a.Height-166*a.Scale/100 < a.Font {
					t.Fatalf("card has no readable body area: %+v", a)
				}
				if a.X < area.x || a.Y < area.y || a.X+a.Width > area.x+area.w || a.Y+a.Height > area.y+area.h {
					t.Fatalf("card crosses taskbar/work area: %+v", a)
				}
				if area.w == 3840 && a.Font < 40 {
					t.Fatalf("4K classroom text too small: %d", a.Font)
				}
				for j, b := range cards {
					if j > i && a.X < b.X+b.Width && a.X+a.Width > b.X && a.Y < b.Y+b.Height && a.Y+a.Height > b.Y {
						t.Fatalf("cards overlap: %+v %+v", a, b)
					}
				}
			}
		}
	}
}

func TestHomeworkPreferencesPersistAndTeacherRevisionWins(t *testing.T) {
	ready := make(chan struct{})
	close(ready)
	windows := &fakeHomeworkWindows{}
	a := &App{dataDir: t.TempDir(), initDone: ready, homework: homeworkState{preferences: map[string]HomeworkPreference{}, windows: windows}}
	card := HomeworkCard{ID: "h1", ClassID: "c1", Subject: "数学", Text: "完成课后练习", Visible: true, AlwaysOnTop: true, UpdatedAt: "v1"}
	update := func() {
		t.Helper()
		data, _ := json.Marshal([]HomeworkCard{card})
		if err := a.UpdateHomeworks(string(data)); err != nil {
			t.Fatal(err)
		}
	}
	update()
	if err := a.SetHomeworkPreference("h1", "hide"); err != nil {
		t.Fatal(err)
	}
	if windows.items[0].Visible {
		t.Fatal("local hide was ignored")
	}
	if _, err := os.Stat(filepath.Join(a.dataDir, "homework-local.json")); err != nil {
		t.Fatal(err)
	}
	update()
	if windows.items[0].Visible {
		t.Fatal("periodic snapshot reset local setting")
	}
	card.UpdatedAt = "v2"
	update()
	if !windows.items[0].Visible {
		t.Fatal("teacher update did not replace old override")
	}
	if err := a.SetHomeworkPreference("*", "unpin"); err != nil {
		t.Fatal(err)
	}
	if windows.items[0].AlwaysOnTop {
		t.Fatal("local pin setting ignored")
	}
	if err := a.SetHomeworkPreference("h1", "default"); err != nil {
		t.Fatal(err)
	}
	if !windows.items[0].AlwaysOnTop {
		t.Fatal("follow-teacher did not reset setting")
	}
	a.suspendHomework(true)
	if !windows.suspended {
		t.Fatal("fullscreen notification must suspend homework")
	}
	if err := a.SetHomeworkPreference("h1", "hide"); err != nil {
		t.Fatal(err)
	}
	if err := a.UpdateHomeworks("[]"); err != nil {
		t.Fatal(err)
	}
	stored, _ := os.ReadFile(filepath.Join(a.dataDir, "homework-local.json"))
	var persisted map[string]HomeworkPreference
	if json.Unmarshal(stored, &persisted) != nil || len(persisted) != 0 {
		t.Fatal("unbinding must clear persisted preferences")
	}
	if len(windows.items) != 0 || len(a.GetHomeworkSettings().Preferences) != 0 {
		t.Fatal("removed homework remains locally")
	}
}

func TestHomeworkCardMetricsKeepBodyAboveControls(t *testing.T) {
	for _, area := range []struct{ w, h int }{{1366, 728}, {1920, 1032}, {3840, 2072}} {
		for n := 1; n <= 12; n++ {
			for _, g := range homeworkLayout(0, 0, area.w, area.h, n) {
				for _, scrollable := range []bool{false, true} {
					m := homeworkCardMetrics(g, scrollable)
					if m.BodyBottom-m.BodyTop < g.Font*2 {
						t.Fatalf("card body shorter than two lines: %+v %+v", g, m)
					}
					if scrollable && (m.HintTop < m.BodyBottom || m.HintTop+26*g.Scale/100 > m.HitTop) {
						t.Fatalf("scroll hint overlaps body or controls: %+v %+v", g, m)
					}
					if m.HitTop < m.BodyBottom || m.FooterTop+m.FooterHeight > g.Height || m.HitTop > g.Height-20 {
						t.Fatalf("controls are outside the bottom strip: %+v %+v", g, m)
					}
				}
			}
		}
	}
}

func TestHomeworkSubjectColor(t *testing.T) {
	if homeworkSubjectColor("初二数学") == homeworkSubjectColor("语文") {
		t.Fatal("different subjects should have different accents")
	}
	if homeworkSubjectColor("自习") != [3]uint8{25, 108, 80} {
		t.Fatal("unknown subjects should use the brand accent")
	}
}
