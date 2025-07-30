@echo off
echo 正在构建游戏服务器...

REM 清理缓存
go clean -cache

REM 下载依赖
go mod download

REM 构建项目
go build -o gameserver.exe main.go

if %ERRORLEVEL% EQU 0 (
    echo 构建成功！
    echo 正在启动服务器...
    ./gameserver.exe
) else (
    echo 构建失败！
    pause
) 