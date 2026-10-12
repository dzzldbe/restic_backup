@echo off
setlocal EnableDelayedExpansion

:: --- Configuration ---
:: Automatically uses the folder where this script is located
set "SCRIPTS_DIR=%~dp0"
:: Strip trailing backslash for clean path joining
if "%SCRIPTS_DIR:~-1%"=="\" set "SCRIPTS_DIR=%SCRIPTS_DIR:~0,-1%"

set "RESTIC=%SCRIPTS_DIR%\restic.exe"
set "PW=%SCRIPTS_DIR%\password.txt"
set "REPO_DRIVE=C:"
set "REPO=%REPO_DRIVE%\restic_backup"

echo ===================================================
echo           Restic Initial Setup Wizard
echo ===================================================
echo.

:: 1. Check if restic.exe is present in this folder
if not exist "%RESTIC%" (
    echo [ERROR] restic.exe not found at %RESTIC%!
    echo Please make sure restic.exe is in the same folder as this script.
    echo.
    pause
    exit /b 1
)

:: 2. Check drive availability
if not exist "%REPO_DRIVE%\" (
    echo [ERROR] Drive %REPO_DRIVE% was not found!
    echo Please verify the drive letter in the script configuration.
    echo.
    pause
    exit /b 1
)

:: 3. Check if repository is already initialized
if exist "%REPO%\config" (
    echo [WARNING] A Restic repository already exists at %REPO%.
    echo Initial setup has already been run. No changes made.
    echo.
    pause
    exit /b 0
)

:: 4. Set up password.txt
if exist "%PW%" (
    echo [+] Existing password file detected: %PW%
) else (
    echo Enter a password for your encrypted backup.
    echo NOTE: Write this down! If lost, your backups cannot be recovered.
    echo.
    set /p "USER_PW=Enter backup password: "
    if "!USER_PW!"=="" (
        echo [ERROR] Password cannot be blank!
        pause
        exit /b 1
    )
    echo !USER_PW!> "%PW%"
    echo [+] Saved password to %PW%
)

:: 5. Initialize the repository
echo.
echo Initializing Restic repository at %REPO%...
"%RESTIC%" init --repo "%REPO%" --password-file "%PW%"

if %ERRORLEVEL% EQU 0 (
    echo.
    echo ===================================================
    echo  SUCCESS: Repository initialized!
    echo ===================================================
) else (
    echo.
    echo [ERROR] Initialization failed with code %ERRORLEVEL%.
)

echo.
pause
exit /b %ERRORLEVEL%