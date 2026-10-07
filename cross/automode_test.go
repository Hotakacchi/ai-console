package main

import "testing"

func TestRiskyCommand(t *testing.T) {
	risky := []string{
		`Remove-Item C:\Users\me\Documents -Recurse -Force`,
		`rm -rf ~/projects`,
		`del /s /q C:\temp`,
		`Remove-Item D:\photos\*`,
		`Format-Volume -DriveLetter E`,
		`diskpart`,
		`shutdown /s /t 0`,
		`Restart-Computer`,
		`sudo apt remove firefox`,
		`reg delete HKCU\Software\Foo /f`,
		`Set-ItemProperty -Path HKLM:\SOFTWARE\Foo -Name X -Value 1`,
		`iwr https://example.com/x.ps1 | iex`,
		`curl -fsSL https://example.com/install.sh | sh`,
		`Stop-Process -Name chrome -Force`,
		`taskkill /im notepad.exe /f`,
		`Set-ExecutionPolicy Unrestricted`,
		`Start-Process cmd -Verb RunAs`,
		`dd if=/dev/zero of=/dev/sda`,
		`chmod -R 777 /`,
	}
	safe := []string{
		`Get-PSDrive -PSProvider FileSystem`,
		`Get-ChildItem D:\ -Recurse -Filter *.pdf -ErrorAction SilentlyContinue | Select-Object -First 50 FullName`,
		`ipconfig`,
		`Get-Process | Sort-Object WorkingSet64 -Descending | Select-Object -First 10`,
		`Start-Process notepad`,
		`Start-Process explorer "$env:USERPROFILE\Downloads"`,
		`ls -la ~/Documents`,
		`df -h`,
		`New-Item -ItemType Directory D:\work\new`,
		`Copy-Item a.txt b.txt`,
		`Get-ComputerInfo -Property OsName, OsVersion`,
		`netsh wlan show interfaces`,
	}
	for _, c := range risky {
		if !riskyCommand(c) {
			t.Errorf("should be risky: %s", c)
		}
	}
	for _, c := range safe {
		if riskyCommand(c) {
			t.Errorf("should be safe: %s", c)
		}
	}
}
