//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
)

// The script invokes the Windows Core Audio COM interfaces, identifies the
// original endpoint, and restores that same endpoint even if the default changes.
const audioScript = `
$ErrorActionPreference = 'Stop'
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
[ComImport, Guid("BCDE0395-E52F-467C-8E3D-C4579291692E")] class MMDeviceEnumerator {}
[ComImport, Guid("A95664D2-9614-4F35-A746-DE8DB63617E6"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)] interface IMMDeviceEnumerator {
 void EnumAudioEndpoints(int flow, int mask, out IntPtr devices);
 void GetDefaultAudioEndpoint(int flow, int role, out IMMDevice device);
 void GetDevice([MarshalAs(UnmanagedType.LPWStr)] string id, out IMMDevice device);
}
[ComImport, Guid("D666063F-1587-4E43-81F1-B948E807363F"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)] interface IMMDevice {
 void Activate(ref Guid id, int context, IntPtr activation, [MarshalAs(UnmanagedType.IUnknown)] out object instance);
 void OpenPropertyStore(int access, out IntPtr properties);
 void GetId([MarshalAs(UnmanagedType.LPWStr)] out string id);
}
[ComImport, Guid("5CDF2C82-841E-4546-9722-0CF74078229A"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)] interface IAudioEndpointVolume {
 void RegisterControlChangeNotify(IntPtr notify); void UnregisterControlChangeNotify(IntPtr notify);
 void GetChannelCount(out uint count); void SetMasterVolumeLevel(float level, ref Guid context);
 void SetMasterVolumeLevelScalar(float level, ref Guid context); void GetMasterVolumeLevel(out float level);
 void GetMasterVolumeLevelScalar(out float level); void SetChannelVolumeLevel(uint channel, float level, ref Guid context);
 void SetChannelVolumeLevelScalar(uint channel, float level, ref Guid context); void GetChannelVolumeLevel(uint channel, out float level);
 void GetChannelVolumeLevelScalar(uint channel, out float level); void SetMute([MarshalAs(UnmanagedType.Bool)] bool mute, ref Guid context);
 void GetMute([MarshalAs(UnmanagedType.Bool)] out bool mute);
}
public class ClassDingAudio {
 static IAudioEndpointVolume Open(string id, out string actual) {
  var enumerator = (IMMDeviceEnumerator)new MMDeviceEnumerator(); IMMDevice device;
  if (String.IsNullOrEmpty(id)) enumerator.GetDefaultAudioEndpoint(0, 1, out device); else enumerator.GetDevice(id, out device);
  device.GetId(out actual); Guid iid = typeof(IAudioEndpointVolume).GUID; object instance;
  device.Activate(ref iid, 23, IntPtr.Zero, out instance); return (IAudioEndpointVolume)instance;
 }
 public static string Get() { string id; var v = Open(null, out id); float level; bool mute; v.GetMasterVolumeLevelScalar(out level); v.GetMute(out mute); return id + "|" + level.ToString("R", System.Globalization.CultureInfo.InvariantCulture) + "|" + mute.ToString(); }
 public static void Set(string id, float level, bool mute) { string actual; var v = Open(id, out actual); Guid context = Guid.Empty; v.SetMasterVolumeLevelScalar(level, ref context); v.SetMute(mute, ref context); }
}
'@
if ($env:CLASSDING_AUDIO_ACTION -eq 'get') { [ClassDingAudio]::Get() }
else { [ClassDingAudio]::Set($env:CLASSDING_AUDIO_DEVICE, [float]::Parse($env:CLASSDING_AUDIO_VOLUME, [System.Globalization.CultureInfo]::InvariantCulture), [bool]::Parse($env:CLASSDING_AUDIO_MUTED)) }
`

func hiddenCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd
}

func powershell(script string, extra ...string) (string, error) {
	words := utf16.Encode([]rune(script))
	raw := make([]byte, len(words)*2)
	for i, word := range words {
		binary.LittleEndian.PutUint16(raw[i*2:], word)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(raw))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	cmd.Env = append(os.Environ(), extra...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("Windows 操作超时：%w", ctx.Err())
		}
		return "", fmt.Errorf("Windows 操作失败：%s", strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(output)), nil
}

func audioGet() (*volumeSnapshot, error) {
	output, err := powershell(audioScript, "CLASSDING_AUDIO_ACTION=get")
	if err != nil {
		return nil, err
	}
	parts := strings.Split(output, "|")
	if len(parts) != 3 {
		return nil, fmt.Errorf("无法读取当前音量")
	}
	volume, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return nil, err
	}
	muted, err := strconv.ParseBool(parts[2])
	if err != nil {
		return nil, err
	}
	return &volumeSnapshot{DeviceID: parts[0], Volume: volume, Muted: muted}, nil
}

func audioSet(s *volumeSnapshot) error {
	_, err := powershell(audioScript, "CLASSDING_AUDIO_ACTION=set", "CLASSDING_AUDIO_DEVICE="+s.DeviceID, "CLASSDING_AUDIO_VOLUME="+strconv.FormatFloat(s.Volume, 'f', -1, 64), "CLASSDING_AUDIO_MUTED="+strconv.FormatBool(s.Muted))
	return err
}

const runKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

func startupShortcut() (string, error) {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return "", fmt.Errorf("无法找到当前用户的启动目录")
	}
	return filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "ClassDing.lnk"), nil
}

func setAutostart(enabled bool) error {
	shortcut, err := startupShortcut()
	if err != nil {
		return err
	}
	if !enabled {
		if hiddenCommand("reg.exe", "query", runKey, "/v", "ClassDing").Run() == nil {
			if output, err := hiddenCommand("reg.exe", "delete", runKey, "/v", "ClassDing", "/f").CombinedOutput(); err != nil {
				return fmt.Errorf("关闭开机启动失败：%s", strings.TrimSpace(string(output)))
			}
		}
		if err := os.Remove(shortcut); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if output, err := hiddenCommand("reg.exe", "add", runKey, "/v", "ClassDing", "/t", "REG_SZ", "/d", `"`+exe+`" --silent`, "/f").CombinedOutput(); err != nil {
		return fmt.Errorf("注册开机启动失败：%s", strings.TrimSpace(string(output)))
	}
	_, err = powershell(`$ErrorActionPreference='Stop'; $s=New-Object -ComObject WScript.Shell; $link=$s.CreateShortcut($env:CLASSDING_SHORTCUT); $link.TargetPath=$env:CLASSDING_EXE; $link.Arguments='--silent'; $link.WorkingDirectory=[System.IO.Path]::GetDirectoryName($env:CLASSDING_EXE); $link.Save()`, "CLASSDING_SHORTCUT="+shortcut, "CLASSDING_EXE="+exe)
	if err != nil {
		_ = hiddenCommand("reg.exe", "delete", runKey, "/v", "ClassDing", "/f").Run()
		return err
	}
	return nil
}

func getAutostart() (bool, error) {
	return hiddenCommand("reg.exe", "query", runKey, "/v", "ClassDing").Run() == nil, nil
}
