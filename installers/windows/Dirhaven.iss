; Dirhaven per-user Windows installer.
; Compile with Inno Setup 6 after building dist/dirhaven-windows-amd64.exe.
; Explicitly does not install or modify OpenAI tunnels, shared plugins,
; existing MCP services, or user configuration.
#define AppName "Dirhaven"
#define AppVersion "0.2.0-dev"
#define AppExe "dirhaven.exe"

[Setup]
AppId={{E5CBEA13-DA27-43EF-B067-24E20A8AF5CC}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion} (setup preview)
AppPublisher=Diego Linan
AppPublisherURL=https://github.com/diegolinan/dirhaven
AppSupportURL=https://github.com/diegolinan/dirhaven/issues
AppUpdatesURL=https://github.com/diegolinan/dirhaven/releases
DefaultDirName={localappdata}\Programs\Dirhaven
DefaultGroupName=Dirhaven
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
ChangesAssociations=no
ChangesEnvironment=no
DisableProgramGroupPage=yes
DisableDirPage=no
DisableWelcomePage=no
OutputDir=..\..\dist\installer
OutputBaseFilename=Dirhaven-Setup-0.2.0-dev-windows-x64
SetupIconFile=..\..\assets\dirhaven-installer.ico
UninstallDisplayIcon={app}\dirhaven.ico
LicenseFile=..\..\LICENSE
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
CloseApplications=no
RestartApplications=no
UsePreviousAppDir=yes
VersionInfoVersion=0.2.0.0

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "Create desktop shortcut for Dirhaven Setup"; GroupDescription: "Additional shortcuts:"; Flags: unchecked

[Files]
Source: "..\..\dist\dirhaven-windows-amd64.exe"; DestDir: "{app}"; DestName: "{#AppExe}"; Flags: ignoreversion
Source: "..\..\assets\dirhaven-installer.ico"; DestDir: "{app}"; DestName: "dirhaven.ico"; Flags: ignoreversion
Source: "..\..\LICENSE"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\NOTICE"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\README.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\SECURITY.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\CONTRIBUTING.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\third_party\README.md"; DestDir: "{app}\third_party"; Flags: ignoreversion
Source: "..\..\third_party\licenses\*"; DestDir: "{app}\third_party\licenses"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{autoprograms}\Dirhaven Setup"; Filename: "{app}\{#AppExe}"; Parameters: "setup"; WorkingDir: "{app}"; IconFilename: "{app}\dirhaven.ico"
Name: "{autodesktop}\Dirhaven Setup"; Filename: "{app}\{#AppExe}"; Parameters: "setup"; WorkingDir: "{app}"; IconFilename: "{app}\dirhaven.ico"; Tasks: desktopicon

[Run]
Filename: "{app}\{#AppExe}"; Parameters: "setup"; Description: "Open Dirhaven Setup Wizard"; Flags: postinstall nowait skipifsilent
