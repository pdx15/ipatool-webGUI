@echo off
setlocal enabledelayedexpansion
chcp 65001 >nul
title ipatool GUI - Windows Builder
cd /d "%~dp0"

set "NON_INTERACTIVE=0"
set "AUTO_CONFIRM=0"
set "CUSTOM_VER="

:parse_args
if "%~1"=="" goto after_args
if "%~1"=="--non-interactive" (
    set "NON_INTERACTIVE=1"
    shift
    goto parse_args
)
if "%~1"=="-y" (
    set "AUTO_CONFIRM=1"
    shift
    goto parse_args
)
if "%CUSTOM_VER%"=="" (
    set "CUSTOM_VER=%~1"
    shift
    goto parse_args
)
shift
goto parse_args

:after_args

echo ======================================================================
echo             ipatool WebGUI - Windows Executable Builder
echo ======================================================================
echo.

:: 1. Check prerequisites
echo [1/6] Checking required build tools...

where go >nul 2>nul
if %ERRORLEVEL% neq 0 (
    echo [ERROR] Go compiler is not installed or not found in PATH!
    echo.
    echo Please install Go for Windows from: https://go.dev/dl/
    echo After installing, restart your command prompt and run build.bat again.
    echo.
    goto :handle_exit_error
)

where git >nul 2>nul
if %ERRORLEVEL% neq 0 (
    echo [ERROR] Git is not installed or not found in PATH!
    echo.
    echo Please install Git for Windows from: https://git-scm.com/downloads/win
    echo After installing, restart your command prompt and run build.bat again.
    echo.
    goto :handle_exit_error
)

for /f "tokens=*" %%v in ('go version') do set "GO_VER=%%v"
for /f "tokens=*" %%v in ('git --version') do set "GIT_VER=%%v"
echo  - %GO_VER%
echo  - %GIT_VER%
echo.

:: 2. Determine target version
set "TARGET_VER=%CUSTOM_VER%"
if "%TARGET_VER%"=="" (
    for /f "tokens=*" %%t in ('git describe --tags --abbrev=0 2^>nul') do set "TARGET_VER=%%t"
)
if "%TARGET_VER%"=="" set "TARGET_VER=2.7.0k-webGUI"

set "RAW_VER=!TARGET_VER!"
if "!RAW_VER:~0,1!"=="v" set "RAW_VER=!RAW_VER:~1!"
if "!RAW_VER:~-7!"=="-webGUI" set "RAW_VER=!RAW_VER:~0,-7!"
if "!RAW_VER:~-7!"=="-webgui" set "RAW_VER=!RAW_VER:~0,-7!"
set "RELEASE_VER=!RAW_VER!-webGUI"

for /f "tokens=1,2,3 delims=." %%a in ("!RAW_VER!") do (
    set "MAJ=%%a"
    set "MIN=%%b"
    set "PAT=%%c"
)
set "CLEAN_PAT=!PAT!"
for %%L in (a b c d e f g h i j k l m n o p q r s t u v w x y z A B C D E F G H I J K L M N O P Q R S T U V W X Y Z) do (
    if "!CLEAN_PAT:~-1!"=="%%L" set "CLEAN_PAT=!CLEAN_PAT:~0,-1!"
)
set "UPSTREAM_VER=!MAJ!.!MIN!.!CLEAN_PAT!"
set "UPSTREAM_TAG=v!UPSTREAM_VER!"

echo [2/6] Version: !RAW_VER! [WebGUI build: !RELEASE_VER! ^| Upstream base: !UPSTREAM_TAG!]
echo.

:: 3. Prepare working tree
set "BUILD_DIR=build_temp\ipatool"
echo [3/6] Setting up build directory in %BUILD_DIR%...

if not exist "build_temp" mkdir "build_temp"

if not exist "%BUILD_DIR%\.git" (
    echo  - Cloning majd/ipatool repository [!UPSTREAM_TAG!]...
    git clone --depth 1 --branch !UPSTREAM_TAG! https://github.com/majd/ipatool.git "%BUILD_DIR%"
    if !ERRORLEVEL! neq 0 (
        echo  - Fallback: cloning default branch...
        git clone --depth 1 https://github.com/majd/ipatool.git "%BUILD_DIR%"
    )
    if !ERRORLEVEL! neq 0 (
        echo [ERROR] Failed to clone majd/ipatool repository. Check your internet connection.
        goto :handle_exit_error
    )
) else (
    echo  - Reusing existing repository, resetting tracked files...
    pushd "%BUILD_DIR%"
    git reset --hard HEAD >nul 2>nul
    git clean -fd >nul 2>nul
    popd
)
echo.

:: 4. Apply WebGUI patches and copy custom sources
echo [4/6] Integrating WebGUI sources and patches...

pushd "%BUILD_DIR%"

set "PATCH_GUI=..\..\patches\register-gui.patch"
set "PATCH_META=..\..\patches\minimum-os-version.patch"
set "PATCH_PRIO=..\..\patches\download-product-priority.patch"

if not exist "!PATCH_GUI!" (
    echo [ERROR] Missing patch: !PATCH_GUI!
    popd
    goto :handle_exit_error
)
if not exist "!PATCH_META!" (
    echo [ERROR] Missing patch: !PATCH_META!
    popd
    goto :handle_exit_error
)
if not exist "!PATCH_PRIO!" (
    echo [ERROR] Missing patch: !PATCH_PRIO!
    popd
    goto :handle_exit_error
)

echo  - Applying patches...
git apply "!PATCH_GUI!"
if !ERRORLEVEL! neq 0 (
    echo [ERROR] Failed to apply register-gui.patch
    popd
    goto :handle_exit_error
)
git apply "!PATCH_META!"
if !ERRORLEVEL! neq 0 (
    echo [ERROR] Failed to apply minimum-os-version.patch
    popd
    goto :handle_exit_error
)
git apply "!PATCH_PRIO!"
if !ERRORLEVEL! neq 0 (
    echo [ERROR] Failed to apply download-product-priority.patch
    popd
    goto :handle_exit_error
)

echo  - Copying WebGUI cmd sources and embedded web assets...
xcopy /E /I /Y "..\..\cmd\gui*" "cmd\" >nul
xcopy /E /I /Y "..\..\cmd\gui_assets" "cmd\gui_assets\" >nul

echo  - Copying resources...
if not exist "resources" mkdir "resources"
xcopy /E /I /Y "..\..\resources\*" "resources\" >nul 2>nul

popd
echo.

:: 5. Compile binary with Go
echo [5/6] Compiling ipatool.exe [GOOS=windows GOARCH=amd64]...
pushd "%BUILD_DIR%"

set "GOOS=windows"
set "GOARCH=amd64"
set "CGO_ENABLED=0"

go build -trimpath -ldflags="-s -w -X github.com/majd/ipatool/v2/cmd.version=!RELEASE_VER!" -o "..\..\ipatool.exe" .
set "BUILD_STATUS=!ERRORLEVEL!"

popd

if !BUILD_STATUS! neq 0 (
    echo.
    echo ======================================================================
    echo [ERROR] Compilation failed with exit code !BUILD_STATUS!
    echo ======================================================================
    goto :handle_exit_error
)

if not exist "ipatool.exe" (
    echo [ERROR] ipatool.exe was not created!
    goto :handle_exit_error
)

echo.
for %%A in ("ipatool.exe") do set "EXE_SIZE=%%~zA"
echo  - Successfully built ipatool.exe [!EXE_SIZE! bytes]
echo.

:: 6. Check for libimobiledevice suite (tools/ folder)
echo [6/6] Checking device installation tools [libimobiledevice]...
if exist "tools\ideviceinstaller.exe" (
    echo  - Found libimobiledevice tools in tools\
    goto :build_finished
)

echo  - Note: 'tools\ideviceinstaller.exe' not found.
echo    The 'Install to device' tab requires libimobiledevice tools.

if "!AUTO_CONFIRM!"=="1" goto :do_download_tools
if "!NON_INTERACTIVE!"=="1" (
    echo  - Non-interactive mode: skipping optional tools download.
    goto :build_finished
)

set "DL_CHOICE=n"
set /p "DL_CHOICE=   Download libimobiledevice tools automatically? (y/N): "
if /i "!DL_CHOICE!"=="y" goto :do_download_tools

echo  - Skipped tools download. You can extract them manually into tools\ later.
goto :build_finished

:do_download_tools
call :download_libimobiledevice
goto :build_finished

:build_finished
echo.
echo ======================================================================
echo                    BUILD COMPLETED SUCCESSFULLY!
echo ======================================================================
echo.
echo  ipatool.exe [version: !RELEASE_VER!] is ready in: %CD%\ipatool.exe
echo  You can now launch the application by running:
echo.
echo       ipatool-gui.bat
echo.
if "!NON_INTERACTIVE!"=="0" pause
exit /b 0

:handle_exit_error
if "!NON_INTERACTIVE!"=="0" pause
exit /b 1

:: Subroutine to download and extract libimobiledevice tools
:download_libimobiledevice
echo  - Downloading and extracting libimobiledevice suite...
powershell -NoProfile -Command ^
    "$ProgressPreference = 'SilentlyContinue';" ^
    "New-Item -ItemType Directory -Force -Path 'tools' | Out-Null;" ^
    "$url = 'https://github.com/L1ghtmann/libimobiledevice/releases/download/suite-exe-21d57a9/libimobile-suite-latest_x86_64-mingw64.tar.xz';" ^
    "Write-Host '   Downloading from GitHub...';" ^
    "Invoke-WebRequest -Uri $url -OutFile 'libimobile-suite.tar.xz';" ^
    "if (Test-Path 'libimobile-suite.tar.xz') {" ^
    "    Write-Host '   Extracting archive...';" ^
    "    tar -xf 'libimobile-suite.tar.xz' -C 'tools/' --strip-components=1 2>$null;" ^
    "    if (-not (Test-Path 'tools\ideviceinstaller.exe')) { tar -xf 'libimobile-suite.tar.xz' -C 'tools/' 2>$null; };" ^
    "    Remove-Item 'libimobile-suite.tar.xz' -Force -ErrorAction SilentlyContinue;" ^
    "    Write-Host '   Tools ready in tools/ folder.';" ^
    "}"
exit /b 0
