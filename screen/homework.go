package main

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type HomeworkCard struct {
	ID          string `json:"id"`
	ClassID     string `json:"class_id"`
	Subject     string `json:"subject"`
	Text        string `json:"text"`
	TeacherName string `json:"teacher_name"`
	Visible     bool   `json:"visible"`
	AlwaysOnTop bool   `json:"always_on_top"`
	UpdatedAt   string `json:"updated_at"`
}

// HomeworkPreference is a local override; Revision is the card's updated_at
// when the override was made.
type HomeworkPreference struct {
	Revision    string `json:"revision"`
	Visible     *bool  `json:"visible,omitempty"`
	AlwaysOnTop *bool  `json:"always_on_top,omitempty"`
}

func (p HomeworkPreference) equal(q HomeworkPreference) bool {
	same := func(x, y *bool) bool { return x == nil && y == nil || x != nil && y != nil && *x == *y }
	return p.Revision == q.Revision && same(p.Visible, q.Visible) && same(p.AlwaysOnTop, q.AlwaysOnTop)
}

func flag(value bool) *bool { return &value }

const maxHomeworkCards = 12

type HomeworkSettings struct {
	Items       []HomeworkCard                `json:"items"`
	Preferences map[string]HomeworkPreference `json:"preferences"`
}

type homeworkWindowManager interface {
	update(string, []HomeworkCard, bool) error
	close()
}

type homeworkState struct {
	mu          sync.Mutex
	items       []HomeworkCard
	preferences map[string]HomeworkPreference
	windows     homeworkWindowManager
	suspended   bool
}

// Local overrides live in homework-local.json, keyed by homework ID.
func (a *App) homeworkFile() string { return filepath.Join(a.dataDir, "homework-local.json") }

func (a *App) loadHomeworkPreferences() {
	if data, err := os.ReadFile(a.homeworkFile()); err == nil {
		_ = json.Unmarshal(data, &a.homework.preferences)
	}
	if a.homework.preferences == nil {
		a.homework.preferences = map[string]HomeworkPreference{}
	}
}

func (a *App) saveHomeworkPreferences() error {
	data, err := json.Marshal(a.homework.preferences)
	if err != nil {
		return err
	}
	return atomicWrite(a.homeworkFile(), data)
}

// UpdateHomeworks receives an authenticated server snapshot from the main WebView.
func (a *App) UpdateHomeworks(value string) error {
	if err := a.ready(); err != nil {
		return err
	}
	var cards []HomeworkCard
	if len(value) > 128*1024 {
		return errors.New("作业数据过大")
	}
	if err := json.Unmarshal([]byte(value), &cards); err != nil {
		return errors.New("作业数据无效")
	}
	if len(cards) > maxHomeworkCards {
		return errors.New("作业数量超过上限")
	}
	a.homework.mu.Lock()
	defer a.homework.mu.Unlock()
	previousItems, previousPreferences := a.homework.items, maps.Clone(a.homework.preferences)
	a.homework.items = cards
	valid := map[string]bool{}
	for _, card := range cards {
		valid[card.ID] = true
		if pref, ok := a.homework.preferences[card.ID]; ok && pref.Revision != card.UpdatedAt {
			delete(a.homework.preferences, card.ID)
		}
	}
	for id := range a.homework.preferences {
		if !valid[id] {
			delete(a.homework.preferences, id)
		}
	}
	var err error
	if !maps.EqualFunc(previousPreferences, a.homework.preferences, HomeworkPreference.equal) {
		err = a.saveHomeworkPreferences()
	}
	if err != nil {
		a.homework.items = previousItems
		a.homework.preferences = previousPreferences
		// Unbinding must still remove every desktop window if disk access fails.
		if len(cards) == 0 {
			a.homework.items = nil
			_ = a.renderHomeworkLocked()
		}
		return err
	}
	return a.renderHomeworkLocked()
}

func (a *App) renderHomeworkLocked() error {
	if a.homework.windows == nil {
		manager, err := newHomeworkWindows(func(id, action string) {
			if err := a.SetHomeworkPreference(id, action); err != nil && a.ctx != nil {
				runtime.EventsEmit(a.ctx, "homework:error", err.Error())
			}
		})
		if err != nil {
			return err
		}
		a.homework.windows = manager
	}
	cards := slices.Clone(a.homework.items)
	for i := range cards {
		if pref, ok := a.homework.preferences[cards[i].ID]; ok && pref.Revision == cards[i].UpdatedAt {
			if pref.Visible != nil {
				cards[i].Visible = *pref.Visible
			}
			if pref.AlwaysOnTop != nil {
				cards[i].AlwaysOnTop = *pref.AlwaysOnTop
			}
		}
	}
	return a.homework.windows.update(a.selectedDisplay(), cards, a.homework.suspended)
}

// rerenderHomework redraws open cards, for example on another display.
func (a *App) rerenderHomework() {
	a.homework.mu.Lock()
	defer a.homework.mu.Unlock()
	if a.homework.windows != nil {
		_ = a.renderHomeworkLocked()
	}
}

func (a *App) GetHomeworkSettings() HomeworkSettings {
	<-a.initDone
	a.homework.mu.Lock()
	defer a.homework.mu.Unlock()
	return HomeworkSettings{Items: slices.Clone(a.homework.items), Preferences: maps.Clone(a.homework.preferences)}
}

// SetHomeworkPreference overrides how this screen shows one card, or every
// card when id is "*". The override lapses when a teacher edits the card.
func (a *App) SetHomeworkPreference(id, action string) error {
	if err := a.ready(); err != nil {
		return err
	}
	a.homework.mu.Lock()
	defer a.homework.mu.Unlock()
	if a.homework.preferences == nil {
		a.homework.preferences = map[string]HomeworkPreference{}
	}
	before := maps.Clone(a.homework.preferences)
	found := false
	for _, card := range a.homework.items {
		if card.ID != id && id != "*" {
			continue
		}
		found = true
		pref := a.homework.preferences[card.ID]
		if pref.Revision != card.UpdatedAt {
			pref = HomeworkPreference{Revision: card.UpdatedAt}
		}
		switch action {
		case "show", "hide":
			pref.Visible = flag(action == "show")
		case "pin", "unpin":
			pref.AlwaysOnTop = flag(action == "pin")
		case "default":
			delete(a.homework.preferences, card.ID)
			continue
		default:
			return errors.New("未知的作业显示设置")
		}
		a.homework.preferences[card.ID] = pref
	}
	if !found && id != "*" {
		return errors.New("作业不存在或已被删除")
	}
	if err := a.saveHomeworkPreferences(); err != nil {
		a.homework.preferences = before
		return err
	}
	if err := a.renderHomeworkLocked(); err != nil {
		return err
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "homework:settings")
	}
	return nil
}

func (a *App) suspendHomework(active bool) {
	a.homework.mu.Lock()
	defer a.homework.mu.Unlock()
	a.homework.suspended = active
	if a.homework.windows != nil {
		_ = a.renderHomeworkLocked()
	}
}

func (a *App) closeHomework() {
	a.homework.mu.Lock()
	defer a.homework.mu.Unlock()
	if a.homework.windows != nil {
		a.homework.windows.close()
		a.homework.windows = nil
	}
}

// Card geometry uses the monitor's work area, which excludes the taskbar.
type homeworkGeometry struct{ X, Y, Width, Height, Font, Scale int }

func homeworkLayout(left, top, width, height, count int) []homeworkGeometry {
	if count < 1 || width < 200 || height < 200 {
		return nil
	}
	if count > 12 {
		count = 12
	}
	scale := height * 100 / 1080
	if scale < 100 {
		scale = 100
	}
	if scale > 220 {
		scale = 220
	}
	gap := 12 * scale / 100
	margin := 14 * scale / 100
	cardW := 420 * scale / 100
	if cardW > width/2 {
		cardW = width / 2
	}
	if cardW < 200 {
		cardW = 200
	}
	// Short work areas need fewer rows so the text stays above the controls.
	maxRows := height / (240 * scale / 100)
	if maxRows < 1 {
		maxRows = 1
	}
	if maxRows > 4 {
		maxRows = 4
	}
	rows := count
	if rows > maxRows {
		rows = maxRows
	}
	cardH := (height - 2*margin - (rows-1)*gap) / rows
	if capH := 350 * scale / 100; cardH > capH {
		cardH = capH
	}
	cols := (count + rows - 1) / rows
	if cols*cardW+(cols-1)*gap+2*margin > width {
		cardW = (width - 2*margin - (cols-1)*gap) / cols
	}
	result := make([]homeworkGeometry, 0, count)
	for i := 0; i < count; i++ {
		col, row := i/rows, i%rows
		result = append(result, homeworkGeometry{X: left + width - margin - cardW - col*(cardW+gap), Y: top + height - margin - cardH - row*(cardH+gap), Width: cardW, Height: cardH, Font: 22 * scale / 100, Scale: scale})
	}
	return result
}

// homeworkCardLayout keeps native card painting and hit testing in sync.
type homeworkCardLayout struct {
	Pad, Accent, ChipHeight, BodyTop, BodyBottom, HintTop, FooterTop, FooterHeight, HitTop int
}

func homeworkCardMetrics(g homeworkGeometry, scrollable bool) homeworkCardLayout {
	s := g.Scale
	l := homeworkCardLayout{Pad: 18 * s / 100, Accent: 4 * s / 100, ChipHeight: 36 * s / 100, FooterHeight: 44 * s / 100}
	l.BodyTop = l.Pad + l.ChipHeight + 14*s/100
	l.FooterTop = g.Height - l.Pad - l.FooterHeight
	// The whole bottom strip is a touch target, larger than the painted buttons.
	l.HitTop = l.FooterTop - 8*s/100
	l.BodyBottom = l.FooterTop - 12*s/100
	if scrollable {
		l.HintTop = l.BodyBottom - 26*s/100
		l.BodyBottom = l.HintTop - 4*s/100
	}
	return l
}

var homeworkSubjectPalette = []struct {
	keys  []string
	color [3]uint8
}{
	{[]string{"语文"}, [3]uint8{196, 64, 52}},
	{[]string{"数学"}, [3]uint8{37, 99, 190}},
	{[]string{"英语", "外语"}, [3]uint8{124, 77, 186}},
	{[]string{"物理"}, [3]uint8{14, 120, 140}},
	{[]string{"化学"}, [3]uint8{184, 98, 16}},
	{[]string{"生物"}, [3]uint8{46, 130, 68}},
	{[]string{"历史"}, [3]uint8{150, 96, 52}},
	{[]string{"地理"}, [3]uint8{22, 114, 166}},
	{[]string{"政治", "道德", "道法"}, [3]uint8{176, 52, 96}},
	{[]string{"科学"}, [3]uint8{16, 128, 114}},
}

// homeworkSubjectColor gives common subjects a stable accent readable on a light card.
func homeworkSubjectColor(subject string) [3]uint8 {
	for _, entry := range homeworkSubjectPalette {
		for _, key := range entry.keys {
			if strings.Contains(subject, key) {
				return entry.color
			}
		}
	}
	return [3]uint8{25, 108, 80}
}
