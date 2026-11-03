package app

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	backupVersion     = 1
	maxBackupBytes    = 256 << 20
	maxBackupUnpacked = 512 << 20
	maxBackupManifest = 32 << 20
	maxBackupFiles    = 10000
	maxBackupAudio    = 8 << 20
)

type backupFile struct {
	AudioID string `json:"audio_id"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
}

type backupManifest struct {
	Format    string       `json:"format"`
	Version   int          `json:"version"`
	CreatedAt string       `json:"created_at"`
	State     State        `json:"state"`
	Files     []backupFile `json:"files"`
}

var safeAudioID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func archiveAudioPath(audio Audio) (string, error) {
	extensions := map[string]string{"audio/webm": ".webm", "video/webm": ".webm", "audio/mp4": ".m4a", "audio/mpeg": ".mp3", "audio/ogg": ".ogg", "audio/wav": ".wav"}
	ext, exists := extensions[audio.MIME]
	if !exists || !safeAudioID.MatchString(audio.ID) {
		return "", errors.New("invalid audio metadata")
	}
	return "audio/" + audio.ID + ext, nil
}

// Callers hold a.mu to obtain a consistent state and file snapshot.
func (a *App) createBackup(w io.Writer) error {
	if len(a.state.Audio)+1 > maxBackupFiles {
		return errors.New("too many files")
	}
	stateData, err := json.Marshal(a.state)
	if err != nil {
		return err
	}
	var state State
	if err = json.Unmarshal(stateData, &state); err != nil {
		return err
	}
	manifest := backupManifest{Format: "classding-backup", Version: backupVersion, CreatedAt: now(), State: state, Files: []backupFile{}}
	archive := zip.NewWriter(w)
	var unpacked int64
	for key, audio := range state.Audio {
		name, err := archiveAudioPath(audio)
		if err != nil || key != audio.ID {
			return errors.New("invalid audio identifier")
		}
		base, err := filepath.Abs(filepath.Join(a.config.DataDir, "audio"))
		if err != nil {
			return err
		}
		actual, err := filepath.Abs(audio.Path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(base, actual)
		parts := strings.Split(rel, string(filepath.Separator))
		if err != nil || rel == "." || rel == ".." || len(parts) > 2 || (len(parts) == 2 && (!strings.HasPrefix(parts[0], "restore_") || !safeAudioID.MatchString(parts[0]))) {
			return errors.New("audio outside storage directory")
		}
		if len(parts) == 2 {
			parent, err := os.Lstat(filepath.Join(base, parts[0]))
			if err != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 {
				return errors.New("invalid restored audio directory")
			}
		}
		info, err := os.Lstat(actual)
		if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > maxBackupAudio {
			return errors.New("audio file missing or invalid")
		}
		unpacked += info.Size()
		if unpacked > maxBackupUnpacked {
			return errors.New("backup too large")
		}
		file, err := os.Open(actual)
		if err != nil {
			return err
		}
		entry, err := archive.Create(name)
		if err != nil {
			file.Close()
			return err
		}
		digest := sha256.New()
		size, copyErr := io.Copy(io.MultiWriter(entry, digest), io.LimitReader(file, maxBackupAudio+1))
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil || size != info.Size() || size > maxBackupAudio {
			return errors.New("could not read audio file")
		}
		audio.Path = name
		manifest.State.Audio[key] = audio
		manifest.Files = append(manifest.Files, backupFile{AudioID: key, Path: name, Size: size, SHA256: hex.EncodeToString(digest.Sum(nil))})
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil || len(manifestData) > maxBackupManifest || int64(len(manifestData))+unpacked > maxBackupUnpacked {
		return errors.New("backup manifest too large")
	}
	entry, err := archive.Create("manifest.json")
	if err != nil {
		return err
	}
	if _, err := entry.Write(manifestData); err != nil {
		return err
	}
	return archive.Close()
}

func (a *App) backup(w http.ResponseWriter, r *http.Request) {
	if !validOrigin(r) {
		writeError(w, forbidden())
		return
	}
	a.mu.Lock()
	u, err := a.authenticate(r)
	if err == nil && u.Role != "super_admin" {
		err = forbidden()
	}
	if err != nil {
		a.mu.Unlock()
		writeError(w, err)
		return
	}
	file, err := os.CreateTemp(a.config.DataDir, ".classding-backup-*.zip")
	if err == nil {
		err = a.createBackup(file)
	}
	a.mu.Unlock()
	if file == nil {
		writeError(w, err)
		return
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err != nil {
		slog.Error("create backup", "error", err)
		writeError(w, fail(500, "BACKUP_FAILED", "无法生成完整备份，请检查录音文件和存储空间。"))
		return
	}
	info, err := file.Stat()
	if err != nil || info.Size() > maxBackupBytes {
		writeError(w, fail(500, "BACKUP_FAILED", "备份文件过大或无法读取。"))
		return
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="classding-backup.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", fmt.Sprint(info.Size()))
	_, _ = io.Copy(w, file)
}

// Validate every relationship that is required for live entities. Historical
// messages and audio can legitimately refer to since-deleted users/classes/devices.
func validateBackupState(s State) error {
	if s.Users == nil || s.Classes == nil || s.Members == nil || s.Devices == nil || s.Messages == nil || s.Homeworks == nil || s.Codes == nil || s.Invites == nil || s.Sessions == nil || s.Phrases == nil || s.Audio == nil || s.Audit == nil || s.Settings == nil {
		return errors.New("missing state collection")
	}
	if _, ok := s.Settings["site_name"]; !ok {
		return errors.New("missing site name")
	}
	if _, ok := s.Settings["icp_number"]; !ok {
		return errors.New("missing ICP number")
	}
	if _, ok := s.Settings["theme"]; !ok {
		return errors.New("missing theme")
	}
	if !validSettings(siteSettings{SiteName: s.Settings["site_name"], ICPNumber: s.Settings["icp_number"], Theme: s.Settings["theme"]}) {
		return errors.New("invalid settings")
	}
	usernames := map[string]bool{}
	admin := false
	for key, u := range s.Users {
		if key == "" || u.ID != key || !validUsername(u.Username) || usernames[u.Username] || u.PasswordHash == "" || (u.Role != "user" && u.Role != "school_admin" && u.Role != "super_admin") || (u.Status != "active" && u.Status != "disabled") || u.TokenVersion < 0 {
			return errors.New("invalid user")
		}
		usernames[u.Username] = true
		admin = admin || u.Role == "super_admin" && u.Status == "active"
	}
	if !admin {
		return errors.New("no active administrator")
	}
	for key, c := range s.Classes {
		if key == "" || c.ID != key || c.Name == "" || c.HomeworkRevision < 0 || c.HomeworkRevision > maxHomeworkRevision {
			return errors.New("invalid class")
		}
		if _, exists := s.Users[c.OwnerID]; !exists {
			return errors.New("class owner missing")
		}
		if m, exists := s.Members[c.ID+"|"+c.OwnerID]; !exists || m.Role != "owner" {
			return errors.New("class owner membership missing")
		}
		if code, exists := s.Codes[c.ActivationCode]; !exists || code.ClassID == nil || *code.ClassID != c.ID {
			return errors.New("class license missing")
		}
	}
	for key, m := range s.Members {
		if key != m.ClassID+"|"+m.UserID {
			return errors.New("invalid membership")
		}
		c, classExists := s.Classes[m.ClassID]
		_, userExists := s.Users[m.UserID]
		if !classExists || !userExists || (m.Role != "owner" && m.Role != "collaborator") || (m.Role == "owner" && c.OwnerID != m.UserID) {
			return errors.New("invalid membership relationship")
		}
	}
	classHomeworkCounts := map[string]int{}
	for key, item := range s.Homeworks {
		c, classExists := s.Classes[item.ClassID]
		_, teacherExists := s.Users[item.TeacherID]
		created, createdErr := time.Parse(time.RFC3339Nano, item.CreatedAt)
		updated, updatedErr := time.Parse(time.RFC3339Nano, item.UpdatedAt)
		if key == "" || item.ID != key || !classExists || !teacherExists || c.HomeworkRevision < 1 || strings.TrimSpace(item.TeacherName) == "" || !validHomeworkText(item.Subject, item.Text) || createdErr != nil || updatedErr != nil || updated.Before(created) {
			return errors.New("invalid homework")
		}
		classHomeworkCounts[item.ClassID]++
		if classHomeworkCounts[item.ClassID] > maxClassHomeworks {
			return errors.New("too many homeworks")
		}
	}
	tokens := map[string]bool{}
	for key, d := range s.Devices {
		_, exists := s.Classes[d.ClassID]
		digest, digestErr := hex.DecodeString(d.TokenHash)
		if key == "" || d.ID != key || !exists || digestErr != nil || len(digest) != sha256.Size || tokens[d.TokenHash] || d.Sequence < 0 {
			return errors.New("invalid device")
		}
		if d.RemoteConfig != nil && (!validServerURL(d.RemoteConfig.ServerURL) || d.RemoteConfig.Revision == "" || len(d.RemoteConfig.Revision) > 128) {
			return errors.New("invalid remote configuration")
		}
		tokens[d.TokenHash] = true
	}
	for key, code := range s.Codes {
		if key == "" || code.Code != key {
			return errors.New("invalid license")
		}
		if code.ClassID != nil {
			if c, exists := s.Classes[*code.ClassID]; !exists || c.ActivationCode != key {
				return errors.New("invalid license relationship")
			}
		}
	}
	for key, invite := range s.Invites {
		if key == "" || invite.Code != key {
			return errors.New("invalid invitation")
		}
		if _, exists := s.Classes[invite.ClassID]; !exists {
			return errors.New("invitation class missing")
		}
	}
	for key, session := range s.Sessions {
		if key == "" || session.ID != key {
			return errors.New("invalid session")
		}
		if _, exists := s.Users[session.UserID]; !exists {
			return errors.New("session user missing")
		}
	}
	for key, phrase := range s.Phrases {
		if key == "" || phrase.ID != key {
			return errors.New("invalid phrase")
		}
		if _, exists := s.Users[phrase.UserID]; !exists {
			return errors.New("phrase user missing")
		}
	}
	for key, audio := range s.Audio {
		if key != audio.ID {
			return errors.New("invalid audio")
		}
		if _, err := archiveAudioPath(audio); err != nil {
			return err
		}
	}
	for key, message := range s.Messages {
		if key == "" || message.ID != key || message.Targets == nil {
			return errors.New("invalid message")
		}
		for _, target := range message.Targets {
			if target.ClassID == "" {
				return errors.New("message target class missing")
			}
			if device, exists := s.Devices[target.DeviceID]; exists && device.ClassID != target.ClassID {
				return errors.New("message target device mismatch")
			}
		}
	}
	for key, audit := range s.Audit {
		if key == "" || audit.ID != key {
			return errors.New("invalid audit entry")
		}
	}
	return nil
}

func prepareRestore(zipPath, stage string) (State, error) {
	var empty State
	archive, err := zip.OpenReader(zipPath)
	if err != nil {
		return empty, err
	}
	defer archive.Close()
	if len(archive.File) == 0 || len(archive.File) > maxBackupFiles {
		return empty, errors.New("invalid file count")
	}
	entries := map[string]*zip.File{}
	var total uint64
	for _, file := range archive.File {
		name := file.Name
		if name == "" || strings.ContainsAny(name, "\\\x00:") || path.IsAbs(name) || path.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") || !file.Mode().IsRegular() {
			return empty, errors.New("unsafe archive path")
		}
		if _, exists := entries[name]; exists {
			return empty, errors.New("duplicate archive path")
		}
		if file.UncompressedSize64 > maxBackupUnpacked || total > uint64(maxBackupUnpacked)-file.UncompressedSize64 {
			return empty, errors.New("expanded backup too large")
		}
		total += file.UncompressedSize64
		entries[name] = file
	}
	manifestFile, exists := entries["manifest.json"]
	if !exists || manifestFile.UncompressedSize64 > maxBackupManifest {
		return empty, errors.New("manifest missing or too large")
	}
	manifestReader, err := manifestFile.Open()
	if err != nil {
		return empty, err
	}
	data, err := io.ReadAll(io.LimitReader(manifestReader, maxBackupManifest+1))
	closeErr := manifestReader.Close()
	if err != nil || closeErr != nil || len(data) > maxBackupManifest {
		return empty, errors.New("cannot read manifest")
	}
	var manifest backupManifest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&manifest) != nil {
		return empty, errors.New("invalid manifest")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || manifest.Format != "classding-backup" || manifest.Version != backupVersion || manifest.CreatedAt == "" || manifest.Files == nil {
		return empty, errors.New("unsupported or incomplete backup")
	}
	if _, err := time.Parse(time.RFC3339Nano, manifest.CreatedAt); err != nil {
		return empty, errors.New("invalid backup time")
	}
	var envelope struct {
		State map[string]json.RawMessage `json:"state"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return empty, errors.New("invalid state")
	}
	// Version 1 archives created before homework support do not have this
	// collection. A null collection or a revision without a collection cannot
	// be an old archive and must not silently discard homework.
	if manifest.State.Homeworks == nil {
		if _, exists := envelope.State["homeworks"]; exists {
			return empty, errors.New("invalid homework collection")
		}
		for _, c := range manifest.State.Classes {
			if c.HomeworkRevision != 0 {
				return empty, errors.New("homework collection missing for revision")
			}
		}
		manifest.State.Homeworks = map[string]Homework{}
	} else {
		// Boolean zero values alone cannot distinguish false from missing/null.
		var rawItems map[string]map[string]json.RawMessage
		if json.Unmarshal(envelope.State["homeworks"], &rawItems) != nil {
			return empty, errors.New("invalid homework collection")
		}
		for _, fields := range rawItems {
			for _, key := range []string{"visible", "always_on_top"} {
				value, exists := fields[key]
				if !exists || string(value) == "null" {
					return empty, errors.New("missing homework display option")
				}
			}
		}
	}
	if err := validateBackupState(manifest.State); err != nil {
		return empty, err
	}
	if len(manifest.Files) != len(manifest.State.Audio) || len(entries) != len(manifest.Files)+1 {
		return empty, errors.New("missing or extra audio files")
	}
	seen := map[string]bool{}
	for _, item := range manifest.Files {
		audio, exists := manifest.State.Audio[item.AudioID]
		expected, err := archiveAudioPath(audio)
		if !exists || err != nil || seen[item.AudioID] || item.Path != expected || audio.Path != expected || item.Size < 1 || item.Size > maxBackupAudio {
			return empty, errors.New("invalid audio manifest")
		}
		seen[item.AudioID] = true
		file, exists := entries[item.Path]
		if !exists || file.UncompressedSize64 != uint64(item.Size) {
			return empty, errors.New("audio file missing")
		}
		reader, err := file.Open()
		if err != nil {
			return empty, err
		}
		dst, err := os.OpenFile(filepath.Join(stage, filepath.Base(item.Path)), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			reader.Close()
			return empty, err
		}
		digest := sha256.New()
		size, copyErr := io.Copy(io.MultiWriter(dst, digest), io.LimitReader(reader, maxBackupAudio+1))
		readCloseErr, writeCloseErr := reader.Close(), dst.Close()
		if copyErr != nil || readCloseErr != nil || writeCloseErr != nil || size != item.Size || hex.EncodeToString(digest.Sum(nil)) != item.SHA256 {
			return empty, errors.New("audio checksum or write failed")
		}
	}
	return manifest.State, nil
}

func (a *App) restore(w http.ResponseWriter, r *http.Request) {
	if !validOrigin(r) {
		writeError(w, forbidden())
		return
	}
	// Authenticate before accepting the potentially large request body.
	a.mu.Lock()
	u, err := a.authenticate(r)
	if err == nil && u.Role != "super_admin" {
		err = forbidden()
	}
	a.mu.Unlock()
	if err != nil {
		writeError(w, err)
		return
	}
	mediaType, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaErr != nil || mediaType != "application/zip" || r.Header.Get("X-ClassDing-Restore") != "replace" {
		writeError(w, bad("请选择 ZIP 备份并明确确认替换。"))
		return
	}
	if r.ContentLength > maxBackupBytes {
		writeError(w, fail(413, "BODY_TOO_LARGE", "备份文件过大。"))
		return
	}
	file, err := os.CreateTemp(a.config.DataDir, ".classding-upload-*.zip")
	if err != nil {
		writeError(w, err)
		return
	}
	defer os.Remove(file.Name())
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(5 * time.Minute))
	size, copyErr := io.Copy(file, io.LimitReader(r.Body, maxBackupBytes+1))
	_ = r.Body.Close()
	_ = controller.SetReadDeadline(time.Time{})
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		writeError(w, bad("读取备份失败。"))
		return
	}
	if size > maxBackupBytes {
		writeError(w, fail(413, "BODY_TOO_LARGE", "备份文件过大。"))
		return
	}
	stage, err := os.MkdirTemp(a.config.DataDir, ".classding-restore-")
	if err != nil {
		writeError(w, err)
		return
	}
	defer os.RemoveAll(stage)
	state, err := prepareRestore(file.Name(), stage)
	if err != nil {
		slog.Warn("reject backup restore", "error", err)
		writeError(w, bad("备份不完整、已损坏或格式不受支持。"))
		return
	}
	current := filepath.Join(a.config.DataDir, "audio")
	generation := "restore_" + random(12)
	next := filepath.Join(current, generation)
	for key, audio := range state.Audio {
		audio.Path = filepath.Join(next, filepath.Base(audio.Path))
		state.Audio[key] = audio
	}
	state.Sessions = map[string]Session{}
	state.Settings["_auth_epoch"] = random(32)
	for key, device := range state.Devices {
		device.Online = false
		state.Devices[key] = device
	}

	a.mu.Lock()
	// A logout, account change or another restore may have happened while uploading.
	u, err = a.authenticate(r)
	if err == nil && u.Role != "super_admin" {
		err = forbidden()
	}
	if err != nil {
		a.mu.Unlock()
		writeError(w, err)
		return
	}
	// Keep the old generation untouched until the database transaction commits.
	// A process crash therefore leaves either the old files or the new files
	// matching the database, with at most an unreferenced directory to clean up.
	if err = os.Rename(stage, next); err != nil {
		a.mu.Unlock()
		writeError(w, fail(500, "RESTORE_FAILED", "无法恢复录音。"))
		return
	}
	if err = a.store.Replace(context.Background(), state); err != nil {
		if removeErr := os.RemoveAll(next); removeErr != nil {
			slog.Error("remove failed restore audio", "error", removeErr)
		}
		a.mu.Unlock()
		writeError(w, fail(500, "RESTORE_FAILED", "恢复未完成，原数据已保留。"))
		return
	}
	a.state = state
	a.bindings = map[string]binding{}
	a.lastSent = map[string]time.Time{}
	a.limits = map[string]bucket{}
	a.hub.Close()
	entries, err := os.ReadDir(current)
	if err != nil {
		slog.Error("list old audio after restore", "error", err)
	} else {
		for _, entry := range entries {
			if entry.Name() != generation {
				if err := os.RemoveAll(filepath.Join(current, entry.Name())); err != nil {
					slog.Error("remove old audio after restore", "error", err, "name", entry.Name())
				}
			}
		}
	}
	a.mu.Unlock()
	writeJSON(w, 200, ok())
}
