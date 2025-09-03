@echo off
echo ========================================
echo CDK Server Docker 停止脚本
echo ========================================

echo.
echo 正在检查Docker状态...
docker --version >nul 2>&1
if %errorlevel% neq 0 (
    echo 错误: Docker未安装或未启动
    pause
    exit /b 1
)

echo Docker状态正常

echo.
echo 正在停止容器...
docker stop gameserver

if %errorlevel% neq 0 (
    echo 容器可能已经停止或不存在
) else (
    echo 容器已停止
)

echo.
echo 正在删除容器...
docker rm gameserver

if %errorlevel% neq 0 (
    echo 容器可能已经被删除或不存在
) else (
    echo 容器已删除
)

echo.
echo 正在清理未使用的镜像...
docker image prune -f

echo.
echo 清理完成！
echo.
echo 按任意键退出...
pause >nul
