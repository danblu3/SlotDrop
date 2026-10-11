@echo off
rem Builds SlotDrop.exe. Needs Go 1.26+ from https://go.dev/dl/
cd /d "%~dp0"
go build -trimpath -tags desktop,production -ldflags "-H windowsgui -s -w" -o SlotDrop.exe .
if errorlevel 1 (pause) else (echo Built SlotDrop.exe)
