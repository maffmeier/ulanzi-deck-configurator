; Per-user installer, built in the release workflow:
;   iscc /DAppVersion=0.4.0 /DArch=amd64 /DSourceExe=dist\ulanzi-deck-windows-amd64.exe packaging\windows\ulanzi-deck.iss

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef Arch
  #define Arch "amd64"
#endif
#ifndef SourceExe
  #define SourceExe "..\..\dist\ulanzi-deck-windows-" + Arch + ".exe"
#endif

[Setup]
AppId={{6C1F0B52-3D4E-4F7A-9B8E-2A5D7C9E1F43}
AppName=Ulanzi Deck
AppVersion={#AppVersion}
AppPublisher=maffmeier
AppPublisherURL=https://github.com/maffmeier/ulanzi-deck-configurator
DefaultDirName={localappdata}\Programs\ulanzi-deck
DefaultGroupName=Ulanzi Deck
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
#if Arch == "arm64"
ArchitecturesAllowed=arm64
#else
ArchitecturesAllowed=x64compatible
#endif
OutputDir=..\..\dist
OutputBaseFilename=ulanzi-deck-{#AppVersion}-windows-{#Arch}-setup
SetupIconFile=ulanzi-deck.ico
UninstallDisplayIcon={app}\ulanzi-deck.ico
LicenseFile=..\..\LICENSE
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
CloseApplications=force

[Languages]
Name: "en"; MessagesFile: "compiler:Default.isl"
Name: "de"; MessagesFile: "compiler:Languages\German.isl"

[CustomMessages]
en.AutostartTask=Start automatically when I sign in
de.AutostartTask=Beim Anmelden automatisch starten
en.OpenEditor=Open the editor
de.OpenEditor=Editor öffnen

[Tasks]
Name: "autostart"; Description: "{cm:AutostartTask}"
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; Flags: unchecked

[Files]
Source: "{#SourceExe}"; DestDir: "{app}"; DestName: "ulanzi-deck.exe"; Flags: ignoreversion
Source: "ulanzi-deck.ico"; DestDir: "{app}"
Source: "..\..\LICENSE"; DestDir: "{app}"; DestName: "LICENSE.txt"
Source: "..\..\THIRD_PARTY_NOTICES.md"; DestDir: "{app}"

[Icons]
Name: "{group}\Ulanzi Deck"; Filename: "{app}\ulanzi-deck.exe"; Parameters: "--open"; IconFilename: "{app}\ulanzi-deck.ico"
Name: "{group}\{cm:UninstallProgram,Ulanzi Deck}"; Filename: "{uninstallexe}"
Name: "{userdesktop}\Ulanzi Deck"; Filename: "{app}\ulanzi-deck.exe"; Parameters: "--open"; IconFilename: "{app}\ulanzi-deck.ico"; Tasks: desktopicon

[Run]
Filename: "{app}\ulanzi-deck.exe"; Parameters: "autostart on"; Flags: runhidden; Tasks: autostart
Filename: "{app}\ulanzi-deck.exe"; Parameters: "autostart off"; Flags: runhidden; Tasks: not autostart
Filename: "{app}\ulanzi-deck.exe"; Parameters: "--open"; Description: "{cm:OpenEditor}"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "{sys}\taskkill.exe"; Parameters: "/F /IM ulanzi-deck.exe"; Flags: runhidden; RunOnceId: "StopApp"
Filename: "{app}\ulanzi-deck.exe"; Parameters: "autostart off"; Flags: runhidden; RunOnceId: "AutostartOff"
