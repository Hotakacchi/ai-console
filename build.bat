@echo off
rem Build Lumi.exe and LumiSetup.exe with the .NET Framework compiler that ships with Windows
cd /d %~dp0
set FW=%WINDIR%\Microsoft.NET\Framework64\v4.0.30319
set REFS=/r:System.Web.Extensions.dll /r:System.IO.Compression.dll /r:System.IO.Compression.FileSystem.dll /r:System.Windows.Forms.dll /r:System.Drawing.dll

"%FW%\csc.exe" /nologo /target:winexe /optimize+ /codepage:65001 /out:Lumi.exe /win32icon:lumi.ico /lib:"%FW%\WPF" /r:System.Speech.dll %REFS% Lumi.cs Peek.cs Listen.cs PcControl.cs Providers.cs LocalAI.cs || exit /b 1

if exist Setup.cs "%FW%\csc.exe" /nologo /target:winexe /optimize+ /codepage:65001 /out:LumiSetup.exe /win32icon:lumi.ico /resource:Lumi.exe,Lumi.exe %REFS% Setup.cs LocalAI.cs Providers.cs || exit /b 1
