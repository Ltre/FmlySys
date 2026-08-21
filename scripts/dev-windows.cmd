@echo off
setlocal EnableExtensions

rem FmlySys Windows development launcher.
rem All environment variables below are process-local and are restored when this script exits.

set "HTTP_PROXY=http://127.0.0.1:58591"
set "HTTPS_PROXY=http://127.0.0.1:58591"
set "ALL_PROXY=socks5://127.0.0.1:51837"

rem Some command-line tools only inspect lowercase proxy variables.
set "http_proxy=%HTTP_PROXY%"
set "https_proxy=%HTTPS_PROXY%"
set "all_proxy=%ALL_PROXY%"

rem Do not proxy local development traffic.
set "NO_PROXY=127.0.0.1,localhost"
set "no_proxy=%NO_PROXY%"

rem Step1 has no production authentication yet, so only listen on localhost.
set "FMLYSYS_ADDR=127.0.0.1:8080"

rem Resolve the repository root from this script's own location, never from the caller's current directory.
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..") do set "REPO_ROOT=%%~fI"

if not exist "%REPO_ROOT%\go.mod" (
    echo [FmlySys] Repository root could not be resolved from: %SCRIPT_DIR%
    echo [FmlySys] Expected file not found: %REPO_ROOT%\go.mod
    endlocal
    exit /b 1
)

pushd "%REPO_ROOT%" >nul
if errorlevel 1 (
    echo [FmlySys] Failed to enter repository root: %REPO_ROOT%
    endlocal
    exit /b 1
)

set "FMLYSYS_DATA_DIR=%REPO_ROOT%\data"
if not defined FMLYSYS_DEV_MEMBER set "FMLYSYS_DEV_MEMBER=Dev Admin"

where go >nul 2>&1
if errorlevel 1 (
    echo [FmlySys] Go was not found in PATH.
    echo [FmlySys] Add your Go bin directory to PATH and open a new terminal.
    popd >nul
    endlocal
    exit /b 1
)

echo [FmlySys] Repository root: %REPO_ROOT%
echo [FmlySys] HTTP_PROXY=%HTTP_PROXY%
echo [FmlySys] HTTPS_PROXY=%HTTPS_PROXY%
echo [FmlySys] ALL_PROXY=%ALL_PROXY%
echo [FmlySys] Listening on http://%FMLYSYS_ADDR%/
echo [FmlySys] Data directory: %FMLYSYS_DATA_DIR%
echo.

echo [FmlySys] Preparing Go module dependencies...
go mod download all
if errorlevel 1 (
    echo.
    echo [FmlySys] Failed to download Go module dependencies.
    echo [FmlySys] Check that the local proxy is running and the proxy ports are correct.
    popd >nul
    endlocal
    exit /b 1
)

go mod verify
if errorlevel 1 (
    echo.
    echo [FmlySys] Go module verification failed.
    popd >nul
    endlocal
    exit /b 1
)

echo [FmlySys] Go module dependencies are ready.
echo.

go run ./cmd/fmlysys
set "EXIT_CODE=%ERRORLEVEL%"

popd >nul
endlocal & exit /b %EXIT_CODE%
