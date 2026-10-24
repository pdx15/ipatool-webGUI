@echo off
title ipatool GUI — App Store Downloader for Windows
cd /d "%~dp0"

echo =======================================================
echo          ipatool GUI for Windows
echo =======================================================
echo.

:: Check if port 54321 is already in use
netstat -ano | findstr :54321 | findstr LISTENING >nul 2>nul
if %ERRORLEVEL% equ 0 (
    echo [ERROR] Port 54321 is already in use!
    echo.
    echo Another instance of ipatool GUI or another application
    echo is already using port 54321.
    echo.
    echo Please close the other application and try again,
    echo or wait a moment for it to fully shut down.
    echo.
    pause
    exit /b 1
)

echo Checking for ipatool binary...

if exist "ipatool.exe" (
    echo Found ipatool.exe. Starting GUI server...
    ipatool.exe gui --port 54321
    goto :exit
)

if exist "bin\ipatool.exe" (
    echo Found bin\ipatool.exe. Starting GUI server...
    bin\ipatool.exe gui --port 54321
    goto :exit
)

where ipatool >nul 2>nul
if %ERRORLEVEL% equ 0 (
    echo Found ipatool in PATH. Starting GUI server...
    ipatool gui --port 54321
    goto :exit
)

echo [ERROR] ipatool.exe was not found.
echo Please place ipatool.exe in this folder or add ipatool to PATH.
pause
exit /b 1

:exit
echo.
if %ERRORLEVEL% neq 0 (
    echo.
    echo =======================================================
    echo  An error occurred (exit code: %ERRORLEVEL%)
    echo  Read the output above for details.
    echo =======================================================
)
echo.
pause
