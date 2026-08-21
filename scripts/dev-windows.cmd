@echo off
setlocal EnableExtensions

rem FmlySys Windows development launcher.
rem All environment variables below are process-local and are restored when this script exits.

set "EXIT_CODE=1"
set "PUSHD_OK=0"

set "HTTP_PROXY=http://127.0.0.1:58591"
set "HTTPS_PROXY=http://127.0.0.1:58591"
set "ALL_PROXY=socks5://127.0.0.1:51837"
set "http_proxy=%HTTP_PROXY%"
set "https_proxy=%HTTPS_PROXY%"
set "all_proxy=%ALL_PROXY%"
set "NO_PROXY=127.0.0.1,localhost"
set "no_proxy=%NO_PROXY%"

rem Development is intentionally bound to localhost.
set "FMLYSYS_ADDR=127.0.0.1:8080"
rem Local development may use the explicit dev-login button; production must leave this disabled.
if not defined FMLYSYS_DEV_AUTH_ENABLED set "FMLYSYS_DEV_AUTH_ENABLED=1"
if not defined FMLYSYS_ADMIN_USERNAME set "FMLYSYS_ADMIN_USERNAME=admin"

set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..") do set "REPO_ROOT=%%~fI"
if not exist "%REPO_ROOT%\go.mod" goto :repo_failed

pushd "%REPO_ROOT%" >nul
if errorlevel 1 goto :pushd_failed
set "PUSHD_OK=1"
set "FMLYSYS_DATA_DIR=%REPO_ROOT%\data"
if not defined FMLYSYS_DEV_MEMBER set "FMLYSYS_DEV_MEMBER=Dev Admin"

where go >nul 2>&1
if errorlevel 1 goto :go_missing

echo [FmlySys] Repository root: %REPO_ROOT%
echo [FmlySys] HTTP_PROXY=%HTTP_PROXY%
echo [FmlySys] HTTPS_PROXY=%HTTPS_PROXY%
echo [FmlySys] ALL_PROXY=%ALL_PROXY%
echo [FmlySys] Listening on http://%FMLYSYS_ADDR%/
echo [FmlySys] Data directory: %FMLYSYS_DATA_DIR%
echo [FmlySys] Local dev login: %FMLYSYS_DEV_AUTH_ENABLED%
if not defined FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD echo [FmlySys] Admin bootstrap password is not set. Set FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD before the first admin login.
if not defined FMLYSYS_WECHAT_APP_ID echo [FmlySys] WeChat OAuth is not configured. Local dev login remains available.
echo.

echo [FmlySys] Tidying Go module metadata...
go mod tidy
if errorlevel 1 goto :tidy_failed

echo [FmlySys] Downloading Go module dependencies...
go mod download all
if errorlevel 1 goto :download_failed

echo [FmlySys] Verifying Go module dependencies...
go mod verify
if errorlevel 1 goto :verify_failed

echo [FmlySys] Go module dependencies are ready.
echo.
echo [FmlySys] Starting FmlySys...
go run ./cmd/fmlysys
set "EXIT_CODE=%ERRORLEVEL%"
if "%EXIT_CODE%"=="0" goto :success

echo.
echo [FmlySys] FmlySys exited with code %EXIT_CODE%.
goto :fail

:repo_failed
echo [FmlySys] Repository root could not be resolved from: %SCRIPT_DIR%
echo [FmlySys] Expected file not found: %REPO_ROOT%\go.mod
goto :fail

:pushd_failed
echo [FmlySys] Failed to enter repository root: %REPO_ROOT%
goto :fail

:go_missing
echo [FmlySys] Go was not found in PATH.
echo [FmlySys] Add your Go bin directory to PATH and open a new terminal.
goto :fail

:tidy_failed
echo.
echo [FmlySys] go mod tidy failed.
echo [FmlySys] Check that the local proxy is running and the proxy ports are correct.
goto :fail

:download_failed
echo.
echo [FmlySys] Failed to download Go module dependencies.
echo [FmlySys] Check that the local proxy is running and the proxy ports are correct.
goto :fail

:verify_failed
echo.
echo [FmlySys] Go module verification failed.
goto :fail

:fail
if "%PUSHD_OK%"=="1" popd >nul
echo.
echo [FmlySys] Startup failed. Press any key to close this window.
pause >nul
endlocal & exit /b %EXIT_CODE%

:success
if "%PUSHD_OK%"=="1" popd >nul
endlocal & exit /b 0
