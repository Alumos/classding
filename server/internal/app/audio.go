package app

import (
	"crypto/hmac"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// audioURLTTL bounds how long a signed recording link stays valid.
const audioURLTTL = 20 * time.Minute

// audioExtensions lists the recording formats accepted, by media type.
var audioExtensions = map[string]string{
	"audio/webm": ".webm",
	"video/webm": ".webm",
	"audio/mp4":  ".m4a",
	"audio/mpeg": ".mp3",
	"audio/ogg":  ".ogg",
	"audio/wav":  ".wav",
}

func (a *App) audioSignature(audioID, expires string) string {
	return hex.EncodeToString(sign(a.config.JWTSecret, audioID+":"+expires))
}

// audioURL is a signed link classroom screens can fetch without credentials.
func (a *App) audioURL(audioID string) string {
	expires := strconv.FormatInt(time.Now().Add(audioURLTTL).Unix(), 10)
	return "/api/v1/audio/" + audioID + "?expires=" + expires + "&signature=" + a.audioSignature(audioID, expires)
}

func (a *App) uploadAudio(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if !a.limit("upload:"+u.ID, 20, time.Hour) {
		return nil, tooMany("录音上传过于频繁。")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAudioBytes)
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		return nil, bad("录音不能超过 8 MB。")
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, bad("请选择录音文件。")
	}
	defer file.Close()
	mime, _, _ := strings.Cut(header.Header.Get("Content-Type"), ";")
	ext, accepted := audioExtensions[mime]
	if !accepted {
		return nil, bad("仅支持 webm、mp4、mp3、ogg 或 wav 录音。")
	}

	audio := Audio{ID: id("audio"), UserID: u.ID, MIME: mime, CreatedAt: now()}
	audio.Path = filepath.Join(a.config.DataDir, "audio", audio.ID+ext)
	out, err := os.OpenFile(audio.Path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	size, err := io.Copy(out, io.LimitReader(file, maxAudioBytes+1))
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil || size == 0 || size > maxAudioBytes {
		os.Remove(audio.Path)
		return nil, bad("录音为空或超过 8 MB。")
	}
	a.deferRollback(func() {
		if err := os.Remove(audio.Path); err != nil {
			slog.Error("remove rolled-back audio", "error", err)
		}
	})
	a.state.Audio[audio.ID] = audio
	return map[string]string{"audio_id": audio.ID, "url": a.audioURL(audio.ID)}, nil
}

func (a *App) downloadAudio(w http.ResponseWriter, r *http.Request) {
	audioID := chi.URLParam(r, "id")
	expiry := r.URL.Query().Get("expires")
	expires, err := strconv.ParseInt(expiry, 10, 64)
	valid := err == nil &&
		time.Now().Unix() <= expires &&
		expires <= time.Now().Add(audioURLTTL+time.Minute).Unix() &&
		hmac.Equal([]byte(r.URL.Query().Get("signature")), []byte(a.audioSignature(audioID, expiry)))
	if !valid {
		writeError(w, forbidden())
		return
	}
	a.mu.Lock()
	audio, exists := a.state.Audio[audioID]
	a.mu.Unlock()
	if !exists {
		writeError(w, missing())
		return
	}
	w.Header().Set("Content-Type", audio.MIME)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filepath.Base(audio.Path)))
	w.Header().Set("Cache-Control", "private, max-age=60")
	http.ServeFile(w, r, audio.Path)
}
