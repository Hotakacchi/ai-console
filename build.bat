@echo off
rem Build Lumi.exe with the .NET Framework compiler that ships with Windows
cd /d %~dp0
set FW=%WINDIR%\Microsoft.NET\Framework64\v4.0.30319
"%FW%\csc.exe" /nologo /target:winexe /optimize+ /codepage:65001 /out:Lumi.exe /lib:"%FW%\WPF" /r:System.Speech.dll /r:System.Web.Extensions.dll /r:System.Windows.Forms.dll /r:System.Drawing.dll Lumi.cs Providers.cs
