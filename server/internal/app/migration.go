package app

import (
	"net/http"
	"os"
	"strings"
)

func migrationNotReady() error {
	return fail(409, "MIGRATION_NOT_READY", "目标站数据尚不完整，请停止发送后重新备份并恢复。")
}

// deviceMigrationCheck verifies the source checkpoint and each locally queued
// notification before issuing URLs signed by the destination server.
func (a *App) deviceMigrationCheck(_ http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	d, err := a.deviceFromToken(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if err != nil {
		return nil, err
	}
	c, exists := a.state.Classes[d.ClassID]
	if !exists {
		return nil, fail(401, "DEVICE_REVOKED", "班级已被删除。")
	}
	var in struct {
		MinSequence *int64    `json:"min_sequence"`
		PendingIDs  *[]string `json:"pending_ids"`
	}
	if err := decodeStrict(r, &in); err != nil {
		return nil, err
	}
	if in.MinSequence == nil || *in.MinSequence < 0 || in.PendingIDs == nil || len(*in.PendingIDs) > 4096 {
		return nil, bad("迁移检查的序号或待播消息列表不正确。")
	}
	if d.Sequence < *in.MinSequence {
		return nil, migrationNotReady()
	}
	notifications := []Frame{}
	recalled := []string{}
	seen := map[string]bool{}
	for _, messageID := range *in.PendingIDs {
		if messageID == "" || len(messageID) > 128 {
			return nil, bad("待播消息编号不正确。")
		}
		if seen[messageID] {
			continue
		}
		seen[messageID] = true
		message, target, status := a.pendingForDevice(d, c, messageID)
		switch status {
		case pendingMissing:
			return nil, migrationNotReady()
		case pendingRecalled:
			recalled = append(recalled, messageID)
		case pendingAvailable:
			if message.Kind == "voice" {
				audio, exists := a.state.Audio[message.AudioID]
				if !exists || audio.UserID != message.SenderID {
					return nil, migrationNotReady()
				}
				info, err := os.Lstat(audio.Path)
				if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
					return nil, migrationNotReady()
				}
				file, err := os.Open(audio.Path)
				if err != nil {
					return nil, migrationNotReady()
				}
				if err := file.Close(); err != nil {
					return nil, migrationNotReady()
				}
			}
			notifications = append(notifications, a.notify(message, target))
		}
	}
	return map[string]any{
		"device": publicDevice(d), "class": map[string]string{"id": c.ID, "name": c.Name},
		"volume": c.Volume, "sequence": d.Sequence, "notifications": notifications, "recalled_ids": recalled,
	}, nil
}
