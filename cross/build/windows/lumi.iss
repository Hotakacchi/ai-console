; ルミの Windows インストーラー (Inno Setup 6)
; 最初に「自分だけ (管理者権限なし、%LOCALAPPDATA%\Programs\Lumi)」か
; 「すべてのユーザー (管理者権限が必要、Program Files\Lumi)」かを選ぶ。新しい版で実行すれば上書き更新になる。
; 設定とダウンロードしたものは、どちらでもユーザーごとの %LOCALAPPDATA%\Lumi に置かれる。
; ビルド: iscc /DAppVersion=1.4.4 /DSourceExe=..\..\lumi.exe lumi.iss

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
; {autopf} は、自分だけなら %LOCALAPPDATA%\Programs、すべてのユーザーなら Program Files になる
DefaultDirName={autopf}\Lumi
DefaultGroupName=Lumi
DisableProgramGroupPage=yes
DisableDirPage=yes
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog commandline
UsedUserAreasWarning=no
OutputBaseFilename=Lumi-Windows-Setup-{#AppVersion}
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
Name: "{autoprograms}\Lumi"; Filename: "{app}\lumi.exe"
Name: "{autodesktop}\Lumi"; Filename: "{app}\lumi.exe"; Tasks: desktopicon

[Run]
; 自動アップデート (/update) から /RELAUNCH 付きで動かされたときは、入れ終わったらルミを起動し直す
; (すべてのユーザー用に管理者として入れたときも、ルミは普段のユーザーとして起動する)
Filename: "{app}\lumi.exe"; Flags: nowait runasoriginaluser; Check: RelaunchRequested
Filename: "{app}\lumi.exe"; Parameters: "--install-local"; Description: "{cm:Launch}"; Flags: postinstall nowait skipifsilent; Tasks: localai
Filename: "{app}\lumi.exe"; Description: "{cm:Launch}"; Flags: postinstall nowait skipifsilent; Tasks: not localai

[Code]
function RelaunchRequested: Boolean;
begin
  Result := Pos('/RELAUNCH', UpperCase(GetCmdTail)) > 0;
end;

// アンインストールのとき、設定とダウンロードしたものも消すか聞く
procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then
    // サイレント実行 (/SUPPRESSMSGBOXES) のときは聞かずに「いいえ」(残す) にする
    if SuppressibleMsgBox(CustomMessage('DeleteData'), mbConfirmation, MB_YESNO or MB_DEFBUTTON2, IDNO) = IDYES then
    begin
      // (すべてのユーザーに入れた場合、消えるのはアンインストールした人の分だけ)
      DelTree(ExpandConstant('{localappdata}\Lumi'), True, True, True);
      DelTree(ExpandConstant('{app}'), True, True, True);
    end;
end;
