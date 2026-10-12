@echo off

set "SCRIPTS_DIR=%~dp0"
:: Strip trailing backslash for clean path joining
if "%SCRIPTS_DIR:~-1%"=="\" set "SCRIPTS_DIR=%SCRIPTS_DIR:~0,-1%"

set "RESTIC=%SCRIPTS_DIR%\restic.exe"
set "PW=%SCRIPTS_DIR%\password.txt"
set "REPO_DRIVE=C:"
set "REPO=%REPO_DRIVE%\restic_backup"

:: Folder to back up
set "SOURCE=C:\Users\%USERNAME%\Downloads"

:: Check if repository exists
if not exist "%REPO%\config" (
    echo [ERROR] Repository not found at %REPO%!
    pause
    exit /b 1
)

echo Backing up data...
"%RESTIC%" -r "%REPO%" --password-file "%PW%" backup "%SOURCE%" --use-fs-snapshot

if %ERRORLEVEL% EQU 0 (
    echo.
    echo [+] Backup complete successfully!
) else (
    echo.
    echo [!] Backup failed with error code %ERRORLEVEL%.
    echo Check if you ran this script as Administrator.
)

echo.
pause