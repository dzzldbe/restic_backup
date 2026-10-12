@echo off
setlocal EnableDelayedExpansion

set "SCRIPTS_DIR=%~dp0"
:: Strip trailing backslash for clean path joining
if "%SCRIPTS_DIR:~-1%"=="\" set "SCRIPTS_DIR=%SCRIPTS_DIR:~0,-1%"

set "RESTIC=%SCRIPTS_DIR%\restic.exe"
set "PW=%SCRIPTS_DIR%\password.txt"
set "REPO_DRIVE=C:"
set "REPO=%REPO_DRIVE%\restic_backup"

:: Destination folder inside the script's directory
set "TARGET_DIR=%SCRIPTS_DIR%\restored_data"

:: 1. Check if repository exists
if not exist "%REPO%\config" (
    echo [ERROR] Repository not found at %REPO%!
    pause
    exit /b 1
)

:: 2. Check if password file exists
if not exist "%PW%" (
    echo [ERROR] Password file not found at %PW%!
    pause
    exit /b 1
)

echo ===================================================
echo              Restic Restore Utility
echo ===================================================
echo.
echo Checking repository snapshots...
echo.

:: 3. List available snapshots
"%RESTIC%" -r "%REPO%" --password-file "%PW%" snapshots
echo.

:: 4. Ask for Snapshot ID (defaults to 'latest')
set "SNAP_ID=latest"
set /p "USER_INPUT=Enter Snapshot ID to restore (leave blank and press Enter for 'latest'): "
if not "!USER_INPUT!"=="" set "SNAP_ID=!USER_INPUT!"

:: 5. Create restore directory if it doesn't exist
if not exist "%TARGET_DIR%" mkdir "%TARGET_DIR%"

echo.
echo Restoring snapshot '!SNAP_ID!' to:
echo %TARGET_DIR%
echo.

:: 6. Perform the restore
"%RESTIC%" -r "%REPO%" --password-file "%PW%" restore "!SNAP_ID!" --target "%TARGET_DIR%"

if %ERRORLEVEL% EQU 0 (
    echo.
    echo [+] Restore completed successfully!
    echo Files are located in: %TARGET_DIR%
) else (
    echo.
    echo [!] Restore failed with error code %ERRORLEVEL%.
)

echo.
pause