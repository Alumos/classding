package app

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// tool starts or stops a classroom tool on every screen of the class.
func (a *App) tool(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	name := chi.URLParam(r, "tool")
	if name != "countdown" && name != "rollcall" && name != "stop" {
		return nil, missing()
	}
	c, err := a.usableClass(u, chi.URLParam(r, "id"), false)
	if err != nil {
		return nil, err
	}
	if !a.limit("tool:"+u.ID+":"+c.ID, 20, time.Minute) {
		return nil, tooMany("工具操作过于频繁。")
	}
	params := map[string]any{}
	switch name {
	case "countdown":
		var in struct {
			Seconds float64 `json:"seconds"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if in.Seconds < 1 || in.Seconds > 7200 || in.Seconds != float64(int(in.Seconds)) {
			return nil, bad("倒计时需为 1–7200 秒整数。")
		}
		params["seconds"] = int(in.Seconds)
	case "rollcall":
		var in struct {
			Names []string `json:"names"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if len(in.Names) == 0 || len(in.Names) > 100 {
			return nil, bad("请输入 1–100 位同学姓名。")
		}
		for _, name := range in.Names {
			if strings.TrimSpace(name) == "" || !runesBetween(name, 1, maxDisplayName) {
				return nil, bad("姓名需为 1–24 字。")
			}
		}
		params["names"] = in.Names
	}
	a.audit(u, "tool."+name, c.ID)
	a.deferJob(func() { a.hub.Class(c.ID, Frame{T: "tool", D: map[string]any{"name": name, "params": params}}) })
	return ok(), nil
}
