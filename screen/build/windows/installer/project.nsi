Unicode true
!ifndef REQUEST_EXECUTION_LEVEL
  !define REQUEST_EXECUTION_LEVEL "user"
!endif
!ifndef WAILS_INSTALL_SCOPE
  !define WAILS_INSTALL_SCOPE "user"
!endif
!define PRODUCT_EXECUTABLE "ClassDingScreen.exe"
!include "wails_tools.nsh"
!include "MUI2.nsh"

Name "课堂叮咚大屏端"
OutFile "..\..\bin\ClassDingScreen-${ARCH}-installer.exe"
InstallDir "$LOCALAPPDATA\Programs\ClassDing"
ManifestDPIAware true
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion "${INFO_PRODUCTVERSION}.0"
VIAddVersionKey "ProductName" "${INFO_PRODUCTNAME}"
VIAddVersionKey "FileVersion" "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright" "${INFO_COPYRIGHT}"
!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"

Function .onInit
  !insertmacro wails.checkArchitecture
  ClearErrors
  GetTempFileName $0 "D:\"
  IfErrors try_e
  Delete "$0"
  StrCpy $INSTDIR "D:\ClassDing"
  Goto done
  try_e:
  ClearErrors
  GetTempFileName $0 "E:\"
  IfErrors done
  Delete "$0"
  StrCpy $INSTDIR "E:\ClassDing"
  done:
FunctionEnd

Section
  !insertmacro wails.setShellContext
  !insertmacro wails.webview2runtime
  SetOutPath "$INSTDIR"
  !insertmacro wails.files
  CreateShortcut "$SMPROGRAMS\课堂叮咚大屏端.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
  CreateShortcut "$DESKTOP\课堂叮咚大屏端.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
  !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
  !insertmacro wails.setShellContext
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "ClassDing"
  Delete "$SMSTARTUP\ClassDing.lnk"
  Delete "$SMPROGRAMS\课堂叮咚大屏端.lnk"
  Delete "$DESKTOP\课堂叮咚大屏端.lnk"
  Delete "$INSTDIR\${PRODUCT_EXECUTABLE}"
  !insertmacro wails.deleteUninstaller
  # 保留 data，避免卸载时不可逆地删除绑定和待处理消息。
  RMDir "$INSTDIR"
SectionEnd
