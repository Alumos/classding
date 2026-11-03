package app

import (
	"net/http"
	"os"
)

// Screens before 0.3.0 move servers by asking the destination to vouch for
// everything they hold before switching. Newer screens switch directly and
// fall back on their own, but the check stays for the old ones.

func migrationNotReady() error {
	return fail(409, "MIGRATION_NOT_READY", "目标站数据尚不完整，请停止发送后重新备份并恢复。")
}

// deviceMigrationCheck verifies the source checkpoint and each locally queued
// notification before issuing URLs signed by the destination server.
func (a *App) deviceMigrationCheck(_ http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	d, c, err := a.boundDevice(r)
	if err != nil {
		return nil, err
	}
	var in struct {
		MinSequence         *int64    `json:"min_sequence"`
		MinHomeworkRevision *int64    `json:"min_homework_revision"`
		PendingIDs          *[]string `json:"pending_ids"`
	}
	if err := decodeStrict(r, &in); err != nil {
		return nil, err
	}
	validRevision := in.MinHomeworkRevision == nil || *in.MinHomeworkRevision >= 0 && *in.MinHomeworkRevision <= maxHomeworkRevision
	if in.MinSequence == nil || *in.MinSequence < 0 || !validRevision || in.PendingIDs == nil || len(*in.PendingIDs) > maxPending {
		return nil, bad("迁移检查的序号或待播消息列表不正确。")
	}
	if d.Sequence < *in.MinSequence || in.MinHomeworkRevision != nil && c.HomeworkRevision < *in.MinHomeworkRevision {
		return nil, migrationNotReady()
	}
	notifications := []Frame{}
	recalled := []string{}
	for _, messageID := range unique(*in.PendingIDs) {
		if messageID == "" || len(messageID) > 128 {
			return nil, bad("待播消息编号不正确。")
		}
		message, target, status := a.pendingForDevice(d, c, messageID)
		switch status {
		case pendingMissing:
			return nil, migrationNotReady()
		case pendingRecalled:
			recalled = append(recalled, messageID)
		case pendingAvailable:
			if message.Kind == "voice" && !a.recordingAvailable(message) {
				return nil, migrationNotReady()
			}
			notifications = append(notifications, a.notify(message, target))
		}
	}
	snapshot := a.deviceSnapshot(d, c)
	snapshot["sequence"] = d.Sequence
	snapshot["notifications"] = notifications
	snapshot["recalled_ids"] = recalled
	return snapshot, nil
}

// recordingAvailable reports whether the voice message's file was restored too.
func (a *App) recordingAvailable(m Message) bool {
	audio, exists := a.state.Audio[m.AudioID]
	if !exists || audio.UserID != m.SenderID {
		return false
	}
	info, err := os.Lstat(audio.Path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return false
	}
	file, err := os.Open(audio.Path)
	if err != nil {
		return false
	}
	return file.Close() == nil
}
