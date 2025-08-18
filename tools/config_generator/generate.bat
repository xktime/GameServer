@echo off
echo 正在生成配置代码...
cd /d "%~dp0"
go run main.go
echo.
echo 按任意键退出...
pause >nul
