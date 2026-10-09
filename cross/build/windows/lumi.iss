; ルミの Windows インストーラー (Inno Setup 6)
; 最初に「自分だけ (管理者権限なし、%LOCALAPPDATA%\Programs\Lumi)」か
; 「すべてのユーザー (管理者権限が必要、Program Files\Lumi)」かを選ぶ。新しい版で実行すれば上書き更新になる。
; 設定とダウンロードしたものは、どちらでもユーザーごとの %LOCALAPPDATA%\Lumi に置かれる。
; ビルド: iscc /DAppVersion=1.6.0 /DSourceExe=..\..\lumi.exe /DSourceCliExe=..\..\lumi-cli.exe lumi.iss
; ターミナル版 (lumi-cli.exe) も入れて、Windows ターミナルの新しいタブのメニューに「Lumi」を出す (プロファイルの断片)。

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef SourceExe
  #define SourceExe "..\..\lumi.exe"
#endif
#ifndef SourceCliExe
  #define SourceCliExe "..\..\lumi-cli.exe"
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
; アプリのアイコンにダウンロードのバッジを付けたもの (setupicon.py で作る)
SetupIconFile=setup.ico
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
ja.LocalAI=ローカルAIもダウンロードする (この PC の性能に合ったものを選びます。約0.6〜6GB、インターネットなしで会話できる)
en.LocalAI=Also download the local AI (picks one that suits this PC; about 0.6-6 GB; chat without the internet)
ja.Whisper=高精度な音声認識 (Whisper、約70MB) もダウンロードする (話しかけた内容を正確に書き起こす)
en.Whisper=Also download accurate speech recognition (Whisper, about 70 MB; transcribes what you say more accurately)
ja.Launch=ルミを起動する
en.Launch=Launch Lumi
ja.DeleteData=設定とダウンロードしたAI・音声認識モデルも削除しますか？
en.DeleteData=Also delete the settings and the downloaded AI and speech models?

[Tasks]
Name: "localai"; Description: "{cm:LocalAI}"
Name: "whisper"; Description: "{cm:Whisper}"
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "{#SourceExe}"; DestDir: "{app}"; DestName: "lumi.exe"; Flags: ignoreversion
Source: "{#SourceCliExe}"; DestDir: "{app}"; DestName: "lumi-cli.exe"; Flags: ignoreversion
; Windows ターミナルのタブのアイコン
Source: "..\..\assets\icon.ico"; DestDir: "{app}"; DestName: "lumi.ico"; Flags: ignoreversion

[InstallDelete]
; 旧 Windows 版 (C#) のアンインストーラー
Type: files; Name: "{app}\uninstall.exe"

[UninstallDelete]
; Windows ターミナルのプロファイル
Type: filesandordirs; Name: "{localappdata}\Microsoft\Windows Terminal\Fragments\Lumi"
Type: filesandordirs; Name: "{commonappdata}\Microsoft\Windows Terminal\Fragments\Lumi"

[Registry]
; 旧 Windows 版 (C#) の「アプリ」一覧の登録を消す (このインストーラーの登録に置き換わる)
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Uninstall\Lumi"; ValueType: none; Flags: deletekey

[Icons]
Name: "{autoprograms}\Lumi"; Filename: "{app}\lumi.exe"
; デスクトップのショートカットは、もうあれば作り直さない (作り直すと、置いた位置が毎回リセットされる)
Name: "{autodesktop}\Lumi"; Filename: "{app}\lumi.exe"; Tasks: desktopicon; Check: DesktopIconMissing

[Run]
; 自動アップデート (/update) から /RELAUNCH 付きで動かされたときは、入れ終わったらルミを起動し直す
; (すべてのユーザー用に管理者として入れたときも、ルミは普段のユーザーとして起動する)
Filename: "{app}\lumi.exe"; Flags: nowait runasoriginaluser; Check: RelaunchRequested
; 選んだもの (ローカルAI・高精度な音声認識) を、初回の起動でダウンロードする
Filename: "{app}\lumi.exe"; Parameters: "{code:FirstRunArgs}"; Description: "{cm:Launch}"; Flags: postinstall nowait skipifsilent

[Code]
function RelaunchRequested: Boolean;
begin
  Result := Pos('/RELAUNCH', UpperCase(GetCmdTail)) > 0;
end;

// 初回の起動に渡す引数 (インストーラーで選んだものをダウンロードさせる)
function FirstRunArgs(Param: String): String;
begin
  Result := '';
  if WizardIsTaskSelected('localai') then
    Result := Result + ' --install-local';
  if WizardIsTaskSelected('whisper') then
    Result := Result + ' --install-whisper';
  Result := Trim(Result);
end;

function DesktopIconMissing: Boolean;
begin
  Result := not FileExists(ExpandConstant('{autodesktop}\Lumi.lnk'));
end;

// JSON の文字列にする (日本語のユーザー名なども \uXXXX にして、ファイルは ASCII だけにする)
function JsonString(S: String): String;
var
  I: Integer;
  C: Char;
begin
  Result := '"';
  for I := 1 to Length(S) do
  begin
    C := S[I];
    if C = '\' then
      Result := Result + '\\'
    else if C = '"' then
      Result := Result + '\"'
    else if Ord(C) > 126 then
      Result := Result + '\u' + Format('%.4x', [Ord(C)])
    else
      Result := Result + C;
  end;
  Result := Result + '"';
end;

// Windows ターミナルの新しいタブのメニューに「Lumi」を出す (ターミナルのプロファイルの断片を置く)
procedure AddTerminalProfile;
var
  Dir, Json: String;
begin
  if IsAdminInstallMode then
    Dir := ExpandConstant('{commonappdata}\Microsoft\Windows Terminal\Fragments\Lumi')
  else
    Dir := ExpandConstant('{localappdata}\Microsoft\Windows Terminal\Fragments\Lumi');
  if not ForceDirectories(Dir) then
    Exit;
  Json := '{"profiles": [{' +
    '"name": "Lumi", ' +
    '"commandline": ' + JsonString('"' + ExpandConstant('{app}\lumi-cli.exe') + '"') + ', ' +
    '"icon": ' + JsonString(ExpandConstant('{app}\lumi.ico')) + ', ' +
    '"startingDirectory": "%USERPROFILE%", ' +
    '"tabTitle": "Lumi", "suppressApplicationTitle": true' +
    '}]}';
  SaveStringToFile(Dir + '\lumi.json', Json, False);
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
    AddTerminalProfile;
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
