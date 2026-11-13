package app

import (
	"classding/server/internal/repo"
	"encoding/json"
	"fmt"
)

// rows encodes each record on its own, so a save compares row by row without
// first marshalling and re-parsing the whole state.
func (s State) rows() (repo.Rows, error) {
	out := repo.Rows{}
	var err error
	add := func(table string, encode func() (map[string]string, error)) {
		if err == nil {
			out[table], err = encode()
			if err != nil {
				err = fmt.Errorf("encode %s: %w", table, err)
			}
		}
	}
	add("student_calls", func() (map[string]string, error) { return tableRows(s.StudentCalls) })
	add("users", func() (map[string]string, error) { return tableRows(s.Users) })
	add("classes", func() (map[string]string, error) { return tableRows(s.Classes) })
	add("class_members", func() (map[string]string, error) { return tableRows(s.Members) })
	add("devices", func() (map[string]string, error) { return tableRows(s.Devices) })
	add("messages", func() (map[string]string, error) { return tableRows(s.Messages) })
	add("homeworks", func() (map[string]string, error) { return tableRows(s.Homeworks) })
	add("activation_codes", func() (map[string]string, error) { return tableRows(s.Codes) })
	add("collab_invites", func() (map[string]string, error) { return tableRows(s.Invites) })
	add("sessions", func() (map[string]string, error) { return tableRows(s.Sessions) })
	add("phrases", func() (map[string]string, error) { return tableRows(s.Phrases) })
	add("audio", func() (map[string]string, error) { return tableRows(s.Audio) })
	add("audit_logs", func() (map[string]string, error) { return tableRows(s.Audit) })
	add("settings", func() (map[string]string, error) { return tableRows(s.Settings) })
	add("schools", func() (map[string]string, error) { return tableRows(s.Schools) })
	return out, err
}

func tableRows[T any](items map[string]T) (map[string]string, error) {
	out := make(map[string]string, len(items))
	for id, item := range items {
		data, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		out[id] = string(data)
	}
	return out, nil
}
