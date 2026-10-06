; ルミの Windows インストーラー (Inno Setup 6)
; 管理者権限なしで %LOCALAPPDATA%\Programs\Lumi に入れる。新しい版で実行すれば上書き更新になる。
; ビルド: iscc /DAppVersion=1.0.0 /DSourceExe=..\..\lumi.exe lumi.iss

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef SourceExe
  #define SourceExe "..\..\lumi.exe"
#endif

[Setup]
AppId={{6F2C9A31-4B7E-4E0B-9C2D-1A6B3F5E8D47}
AppName=Lumi
AppVerName=Lumi {#AppVersion}
AppVersion={#AppVersion}
AppPublisher=Hotakacchi
AppPublisherURL=https://github.com/Hotakacchi/ai-console
DefaultDirName={localappdata}\Programs\Lumi
DefaultGroupName=Lumi
DisableProgramGroupPage=yes
DisableDirPage=yes
PrivilegesRequired=lowest
OutputBaseFilename=LumiSetup
OutputDir=..\..\dist
SetupIconFile=..\..\assets\icon.ico
UninstallDisplayIcon={app}\lumi.exe
UninstallDisplayName=Lumi
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
; 起動中のルミは閉じてから更新する
CloseApplications=yes
RestartApplications=no

[Languages]
Name: "ja"; MessagesFile: "compiler:Languages\Japanese.isl"
Name: "en"; MessagesFile: "compiler:Default.isl"

[CustomMessages]
ja.LocalAI=ローカルAIもダウンロードする (約2.8GB、インターネットなしで会話できる)
en.LocalAI=Also download the local AI (about 2.8 GB; chat without the internet)
ja.Launch=ルミを起動する
en.Launch=Launch Lumi
ja.DeleteData=設定とダウンロードしたAI・音声認識モデルも削除しますか？
en.DeleteData=Also delete the settings and the downloaded AI and speech models?

[Tasks]
Name: "localai"; Description: "{cm:LocalAI}"
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "{#SourceExe}"; DestDir: "{app}"; DestName: "lumi.exe"; Flags: ignoreversion

[InstallDelete]
; 旧 Windows 版 (C#) のアンインストーラー
Type: files; Name: "{app}\uninstall.exe"

[Registry]
; 旧 Windows 版 (C#) の「アプリ」一覧の登録を消す (このインストーラーの登録に置き換わる)
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\Lumi"; ValueType: none; Flags: deletekey

[Icons]
Name: "{userprograms}\Lumi"; Filename: "{app}\lumi.exe"
Name: "{userdesktop}\Lumi"; Filename: "{app}\lumi.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\lumi.exe"; Parameters: "--install-local"; Description: "{cm:Launch}"; Flags: postinstall nowait skipifsilent; Tasks: localai
Filename: "{app}\lumi.exe"; Description: "{cm:Launch}"; Flags: postinstall nowait skipifsilent; Tasks: not localai

[UninstallRun]
Filename: "{cmd}"; Parameters: "/c taskkill /f /im llama-server.exe"; Flags: runhidden; RunOnceId: "StopLocalAI"

[Code]
// アンインストールのとき、設定とダウンロードしたものも消すか聞く
procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then
    if MsgBox(CustomMessage('DeleteData'), mbConfirmation, MB_YESNO or MB_DEFBUTTON2) = IDYES then
    begin
      DelTree(ExpandConstant('{localappdata}\Lumi'), True, True, True);
      DelTree(ExpandConstant('{app}'), True, True, True);
    end;
end;
