@echo off
REM ============================================================
REM  Secret Vault — Uninstall Script
REM ============================================================

setlocal EnableDelayedExpansion

set "APP_NAME=Secret Vault"
set "DATA_DIR=%USERPROFILE%\.secretvault"
set "INSTALL_DIR=%LOCALAPPDATA%\SecretVault"
set "DESKTOP_SHORTCUT=%USERPROFILE%\Desktop\Secret Vault.lnk"
set "STARTMENU_SHORTCUT=%APPDATA%\Microsoft\Windows\Start Menu\Programs\Secret Vault.lnk"


echo       Uninstall %APP_NAME%

REM ── 1. Kill running process ──
echo [1/4] Stopping running instances...
taskkill /f /im secretvault.exe >nul 2>&1
if %ERRORLEVEL% equ 0 (
    echo   √ Stopped secretvault.exe
    timeout /t 2 /nobreak >nul
) else (
    echo   - No running instance found
)

REM ── 2. Remove application files ──
echo [2/4] Removing application files...
set "removed_app=0"

if exist "%INSTALL_DIR%" (
    rmdir /s /q "%INSTALL_DIR%"
    echo   √ Removed: %INSTALL_DIR%
    set "removed_app=1"
)

REM Also check Program Files
if exist "%ProgramFiles%\SecretVault" (
    rmdir /s /q "%ProgramFiles%\SecretVault"
    echo   √ Removed: %ProgramFiles%\SecretVault
    set "removed_app=1"
)

if "!removed_app!"=="0" (
    echo   ! No application directory found in standard locations
)

REM ── 3. Remove shortcuts ──
echo [3/4] Removing shortcuts...
if exist "%DESKTOP_SHORTCUT%" (
    del /f /q "%DESKTOP_SHORTCUT%"
    echo   √ Removed desktop shortcut
) else (
    echo   - No desktop shortcut found
)

if exist "%STARTMENU_SHORTCUT%" (
    del /f /q "%STARTMENU_SHORTCUT%"
    echo   √ Removed Start Menu shortcut
) else (
    echo   - No Start Menu shortcut found
)

REM Remove registry uninstall entry (if created by NSIS installer)
reg delete "HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\SecretVault" /f >nul 2>&1
reg delete "HKLM\Software\Microsoft\Windows\CurrentVersion\Uninstall\SecretVault" /f >nul 2>&1

REM ── 4. Remove user data ──
echo.
if exist "%DATA_DIR%" (
    echo WARNING: This will permanently delete your vault!
    echo All notes, files, and keys will be lost forever.
    echo   Data directory: %DATA_DIR%

    REM Show folder size
    set "size=unknown"
    for /f "tokens=3" %%a in ('dir /s /-c "%DATA_DIR%" 2^>nul ^| findstr /i "File(s)"') do set "size=%%a bytes"
    echo   Total size:     !size!
    echo.

    set /p "confirm=  Delete all vault data? (y/N): "
    if /i "!confirm!"=="y" (
        rmdir /s /q "%DATA_DIR%"
        echo   √ Vault data deleted
    ) else (
        echo   ! Vault data kept at: %DATA_DIR%
    )
) else (
    echo [4/4] No vault data found at %DATA_DIR%
)

echo.
echo ============================================
echo   %APP_NAME% has been uninstalled.
echo ============================================
echo.

pause
endlocal
