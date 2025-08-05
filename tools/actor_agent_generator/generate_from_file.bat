@echo off
setlocal enabledelayedexpansion

echo 正在从配置文件读取目录并生成Actor方法...

cd /d "%~dp0"

:: 检查配置文件是否存在
if not exist "directories.txt" (
    echo 错误: 找不到 directories.txt 配置文件
    pause
    exit /b 1
)

:: 遍历配置文件中的每一行
for /f "usebackq delims=" %%i in ("directories.txt") do (
    echo.
    echo ========================================
    echo 正在生成: %%i
    echo ========================================
    go run . -source %%i
    if !errorlevel! neq 0 (
        echo 警告: 生成 %%i 时出现错误
    )
)

echo.
echo 所有生成完成！
pause 