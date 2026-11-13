package app

import (
	"classding/server/internal/repo"
	"reflect"
	"testing"
)

// commitState persists fixture changes tests make directly in memory. A failed
// command rolls back to the last commit, exactly as production does.
func commitState(a *App) {
	a.mu.Lock()
	_ = a.Save()
	a.mu.Unlock()
}

func TestStateRowsRoundTrip(t *testing.T) {
	f := newSecurityFixture(t)
	rows, err := f.app.state.rows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != reflect.TypeOf(State{}).NumField() {
		t.Fatalf("rows cover %d tables, State has %d", len(rows), reflect.TypeOf(State{}).NumField())
	}
	var decoded State
	if err = repo.Decode(rows, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, f.app.state) {
		t.Fatal("state changed after a round trip through rows")
	}
}
